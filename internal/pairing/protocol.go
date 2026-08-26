package pairing

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	pairingPacketVersion     = 1
	pairingPacketTypeSpake2  = 0
	pairingPacketTypePeerInfo = 1

	// maxPeerInfoSize matches AOSP's kMaxPeerInfoSize: the PeerInfo struct is
	// always exactly this many bytes, zero-padded.
	maxPeerInfoSize = 8192
	maxPayloadSize  = maxPeerInfoSize * 2
)

type peerInfoType uint8

const (
	peerInfoADBRSAPubKey peerInfoType = 0
	peerInfoADBDeviceGUID peerInfoType = 1
)

// writePairingHeader writes a 6-byte PairingPacketHeader: version(1) +
// type(1) + big-endian payload length(4).
func writePairingHeader(w io.Writer, packetType uint8, payloadLen int) error {
	hdr := make([]byte, 6)
	hdr[0] = pairingPacketVersion
	hdr[1] = packetType
	binary.BigEndian.PutUint32(hdr[2:], uint32(payloadLen))
	_, err := w.Write(hdr)
	return err
}

func readPairingHeader(r io.Reader) (packetType uint8, payloadLen uint32, err error) {
	hdr := make([]byte, 6)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return 0, 0, err
	}
	if hdr[0] != pairingPacketVersion {
		return 0, 0, fmt.Errorf("unsupported pairing packet version %d (want %d)", hdr[0], pairingPacketVersion)
	}
	packetType = hdr[1]
	if packetType != pairingPacketTypeSpake2 && packetType != pairingPacketTypePeerInfo {
		return 0, 0, fmt.Errorf("unknown pairing packet type %d", packetType)
	}
	payloadLen = binary.BigEndian.Uint32(hdr[2:])
	if payloadLen == 0 || payloadLen > maxPayloadSize {
		return 0, 0, fmt.Errorf("pairing packet payload size %d out of range", payloadLen)
	}
	return packetType, payloadLen, nil
}

func buildPeerInfo(typ peerInfoType, data string) []byte {
	buf := make([]byte, maxPeerInfoSize)
	buf[0] = byte(typ)
	n := copy(buf[1:], data)
	_ = n
	return buf
}

func parsePeerInfo(buf []byte) (typ peerInfoType, data string, err error) {
	if len(buf) != maxPeerInfoSize {
		return 0, "", fmt.Errorf("unexpected PeerInfo size %d (want %d)", len(buf), maxPeerInfoSize)
	}
	typ = peerInfoType(buf[0])
	rest := buf[1:]
	if i := bytes.IndexByte(rest, 0); i >= 0 {
		rest = rest[:i]
	}
	return typ, string(rest), nil
}

// PeerResult carries what the peer sent us once pairing succeeds.
type PeerResult struct {
	Type peerInfoType
	Data string
}

// RunServerExchange drives the post-TLS-handshake SPAKE2 + encrypted
// PeerInfo exchange, playing the server role. conn must already be a
// completed TLS connection. password is the SPAKE2 password (already
// combined with the TLS exported keying material by the caller, matching
// AOSP's SetupTlsConnection). myPeerInfo is what we send the peer (our adb
// public key); the peer's equivalent is returned on success.
func RunServerExchange(conn io.ReadWriter, password []byte, myPeerInfoData string) (*PeerResult, error) {
	ctx, err := NewServerCtx(password)
	if err != nil {
		return nil, err
	}

	msg := ctx.Msg()
	if err := writePairingHeader(conn, pairingPacketTypeSpake2, len(msg)); err != nil {
		return nil, fmt.Errorf("writing SPAKE2 header: %w", err)
	}
	if _, err := conn.Write(msg); err != nil {
		return nil, fmt.Errorf("writing SPAKE2 message: %w", err)
	}

	pt, plen, err := readPairingHeader(conn)
	if err != nil {
		return nil, fmt.Errorf("reading peer SPAKE2 header: %w", err)
	}
	if pt != pairingPacketTypeSpake2 {
		return nil, fmt.Errorf("expected SPAKE2_MSG packet, got type %d", pt)
	}
	theirMsg := make([]byte, plen)
	if _, err := io.ReadFull(conn, theirMsg); err != nil {
		return nil, fmt.Errorf("reading peer SPAKE2 message: %w", err)
	}

	sharedKey, err := ctx.Finish(theirMsg)
	if err != nil {
		return nil, fmt.Errorf("SPAKE2 key agreement failed: %w", err)
	}
	cipher, err := newAeadCipher(sharedKey)
	if err != nil {
		return nil, fmt.Errorf("initializing pairing cipher: %w", err)
	}

	myInfo := buildPeerInfo(peerInfoADBRSAPubKey, myPeerInfoData)
	encrypted := cipher.Encrypt(myInfo)
	if err := writePairingHeader(conn, pairingPacketTypePeerInfo, len(encrypted)); err != nil {
		return nil, fmt.Errorf("writing PeerInfo header: %w", err)
	}
	if _, err := conn.Write(encrypted); err != nil {
		return nil, fmt.Errorf("writing encrypted PeerInfo: %w", err)
	}

	pt, plen, err = readPairingHeader(conn)
	if err != nil {
		return nil, fmt.Errorf("reading peer PeerInfo header: %w", err)
	}
	if pt != pairingPacketTypePeerInfo {
		return nil, fmt.Errorf("expected PEER_INFO packet, got type %d", pt)
	}
	encTheirs := make([]byte, plen)
	if _, err := io.ReadFull(conn, encTheirs); err != nil {
		return nil, fmt.Errorf("reading peer encrypted PeerInfo: %w", err)
	}
	decrypted, err := cipher.Decrypt(encTheirs)
	if err != nil {
		return nil, fmt.Errorf("decrypting peer PeerInfo failed, wrong pairing code?: %w", err)
	}

	typ, data, err := parsePeerInfo(decrypted)
	if err != nil {
		return nil, err
	}
	return &PeerResult{Type: typ, Data: data}, nil
}
