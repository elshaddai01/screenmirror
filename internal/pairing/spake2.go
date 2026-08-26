// Package pairing implements the server side of Android's ADB TLS wireless
// pairing protocol (the same one used by "Pair device with QR code" in
// Developer Options / Wireless debugging, and by Android Studio's "Pair
// device over Wi-Fi" dialog).
//
// This is a from-scratch reimplementation ported directly from the AOSP
// source (packages/modules/adb/pairing_auth, pairing_connection) and
// BoringSSL's crypto/curve25519/spake25519.cc, since neither adb.exe nor any
// public Go module exposes this. See spake2.go, aesgcm.go and protocol.go
// for the three layers (SPAKE2 key agreement, AES-128-GCM message
// encryption, packet framing) and server.go for how they're wired together
// with TLS and mDNS.
package pairing

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"math/big"

	"filippo.io/edwards25519"
)

// groupOrderL is the order of the prime-order subgroup of edwards25519 (the
// constant BoringSSL calls kOrder in spake25519.cc).
var groupOrderL, _ = new(big.Int).SetString(
	"1000000000000000000000000000000014def9dea2f79cd65812631a5cf5d3ed", 16)

// spakeNPoint and spakeMPoint are BoringSSL's fixed SPAKE2 generator points
// for edwards25519, found by hashing fixed seed strings onto the curve. Their
// compressed encodings are reproduced verbatim from spake25519.cc's comment
// header (and were cross-checked against the y-coordinate bytes embedded in
// that file's kSpakeNSmallPrecomp/kSpakeMSmallPrecomp tables).
var (
	spakeNPoint = mustDecodePoint("10e3df0ae37d8e7a99b5fe74b44672103dbddcbd06af680d71329a11693bc778")
	spakeMPoint = mustDecodePoint("5ada7e4bf6ddd9adb6626d32131c6b5c51a1e347a3478f53cfcf441b88eed12e")
)

func mustDecodePoint(hexStr string) *edwards25519.Point {
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}
	p, err := edwards25519.NewIdentityPoint().SetBytes(b)
	if err != nil {
		panic(err)
	}
	return p
}

// role names, matching AOSP's kClientName/kServerName exactly, including
// their NUL terminator (BoringSSL's sizeof() on the C string literal
// includes it).
var (
	clientName = []byte("adb pair client\x00")
	serverName = []byte("adb pair server\x00")
)

// Ctx runs one side of a single SPAKE2 exchange. Only the server role is
// implemented, since that's the role our tool plays (the phone, after
// scanning the QR code, connects to us as the SPAKE2 client).
type Ctx struct {
	myName, theirName []byte
	isClient          bool

	myPrivate      *big.Int // ephemeral scalar, kept as a raw (non-canonical) integer
	passwordScalar *big.Int // password-derived scalar, forced to be a genuine multiple of 8
	passwordHash   []byte   // SHA-512(password), kept for the transcript hash

	myMsg []byte // our 32-byte P* to send to the peer
}

// NewServerCtx builds a SPAKE2 context playing the server role (AOSP's
// "bob"), matching pairing_auth_server_new.
func NewServerCtx(password []byte) (*Ctx, error) {
	return newCtx(false, password)
}

func newCtx(isClient bool, password []byte) (*Ctx, error) {
	if len(password) == 0 {
		return nil, fmt.Errorf("pairing password must not be empty")
	}

	ctx := &Ctx{isClient: isClient}
	if isClient {
		ctx.myName, ctx.theirName = clientName, serverName
	} else {
		ctx.myName, ctx.theirName = serverName, clientName
	}

	// Ephemeral private scalar. AOSP derives this from 64 random bytes,
	// reduced mod L, then left-shifted by 3 bits (multiplied by 8, as a raw
	// integer) to clear the cofactor. Since this is our own ephemeral
	// secret never revealed to the peer, we only need it to be a genuine
	// multiple of 8 and unpredictable -- we don't need to match AOSP's RNG
	// byte-for-byte.
	randBuf := make([]byte, 64)
	if _, err := rand.Read(randBuf); err != nil {
		return nil, err
	}
	priv := reduceModL(randBuf)
	priv.Lsh(priv, 3)
	ctx.myPrivate = priv

	// Password scalar: this one DOES need to exactly match what a real
	// Android device computes for the same password, since both sides use
	// it to mask/unmask each other's points.
	h := sha512.Sum512(password)
	ctx.passwordHash = h[:]
	ctx.passwordScalar = makeDivisibleBy8(reduceModL(h[:]))

	// P = myPrivate * basePoint
	P := scalarMultBigInt(ctx.myPrivate, edwards25519.NewGeneratorPoint())

	// mask = passwordScalar * (M if client else N)
	mask := scalarMultBigInt(ctx.passwordScalar, ctx.myMaskPoint())

	// P* = P + mask
	pstar := edwards25519.NewIdentityPoint().Add(P, mask)
	ctx.myMsg = pstar.Bytes()

	return ctx, nil
}

// myMaskPoint mirrors AOSP: alice (client) masks with M, bob (server) masks
// with N.
func (c *Ctx) myMaskPoint() *edwards25519.Point {
	if c.isClient {
		return spakeMPoint
	}
	return spakeNPoint
}

// theirMaskPoint is the point used to unmask the peer's message: the
// opposite of myMaskPoint.
func (c *Ctx) theirMaskPoint() *edwards25519.Point {
	if c.isClient {
		return spakeNPoint
	}
	return spakeMPoint
}

// Msg returns the 32-byte message to send to the peer.
func (c *Ctx) Msg() []byte { return c.myMsg }

// Finish processes the peer's message and derives the 64-byte shared key
// material used to set up the AES-128-GCM cipher, matching
// SPAKE2_process_msg.
func (c *Ctx) Finish(theirMsg []byte) ([]byte, error) {
	if len(theirMsg) != 32 {
		return nil, fmt.Errorf("peer SPAKE2 message must be 32 bytes, got %d", len(theirMsg))
	}
	qstar, err := edwards25519.NewIdentityPoint().SetBytes(theirMsg)
	if err != nil {
		return nil, fmt.Errorf("peer SPAKE2 point is not on the curve: %w", err)
	}

	peersMask := scalarMultBigInt(c.passwordScalar, c.theirMaskPoint())
	qext := edwards25519.NewIdentityPoint().Subtract(qstar, peersMask)
	dhShared := scalarMultBigInt(c.myPrivate, qext)
	dhSharedEncoded := dhShared.Bytes()

	sha := sha512.New()
	if c.isClient {
		writeLenPrefixed(sha, c.myName)
		writeLenPrefixed(sha, c.theirName)
		writeLenPrefixed(sha, c.myMsg)
		writeLenPrefixed(sha, theirMsg)
	} else {
		writeLenPrefixed(sha, c.theirName)
		writeLenPrefixed(sha, c.myName)
		writeLenPrefixed(sha, theirMsg)
		writeLenPrefixed(sha, c.myMsg)
	}
	writeLenPrefixed(sha, dhSharedEncoded)
	writeLenPrefixed(sha, c.passwordHash)

	return sha.Sum(nil), nil
}

func writeLenPrefixed(h hash.Hash, data []byte) {
	var lenLE [8]byte
	binary.LittleEndian.PutUint64(lenLE[:], uint64(len(data)))
	h.Write(lenLE[:])
	h.Write(data)
}

// reduceModL reduces a little-endian byte buffer of arbitrary length modulo
// the group order L, matching x25519_sc_reduce's wide reduction.
func reduceModL(leBuf []byte) *big.Int {
	be := make([]byte, len(leBuf))
	for i, b := range leBuf {
		be[len(leBuf)-1-i] = b
	}
	n := new(big.Int).SetBytes(be)
	n.Mod(n, groupOrderL)
	return n
}

// makeDivisibleBy8 replicates BoringSSL's bit-fixing hack: it conditionally
// adds L, 2L and 4L (raw integer addition, not reduced) so the bottom 3 bits
// become zero, while remaining congruent to ps modulo L. This is required
// because the password scalar is used to scale fixed points (M/N) that are
// NOT guaranteed to be in the prime-order subgroup -- clearing the low 3
// bits cancels their order-8-dividing torsion component.
func makeDivisibleBy8(ps *big.Int) *big.Int {
	result := new(big.Int).Set(ps)
	order := new(big.Int).Set(groupOrderL)

	if result.Bit(0) == 1 {
		result.Add(result, order)
	}
	order.Lsh(order, 1) // 2L
	if result.Bit(1) == 1 {
		result.Add(result, order)
	}
	order.Lsh(order, 1) // 4L
	if result.Bit(2) == 1 {
		result.Add(result, order)
	}
	return result
}

// scalarMultBigInt computes k*p via plain double-and-add, treating k as an
// exact (possibly non-canonical, i.e. >= L) non-negative integer rather than
// its residue mod L. This is deliberate: unlike edwards25519.Scalar (which
// always represents a value mod L), some of the scalars in this protocol
// must be multiplied against points that aren't in the prime-order subgroup,
// where the exact integer value -- not just its residue mod L -- determines
// the result. This is not constant-time, which is an acceptable trade-off
// here (pairing your own phone on your own LAN), unlike in a general-purpose
// TLS/crypto library.
func scalarMultBigInt(k *big.Int, p *edwards25519.Point) *edwards25519.Point {
	result := edwards25519.NewIdentityPoint()
	if k.Sign() == 0 {
		return result
	}
	addend := edwards25519.NewIdentityPoint().Set(p)
	bitLen := k.BitLen()
	for i := 0; i < bitLen; i++ {
		if k.Bit(i) == 1 {
			result.Add(result, addend)
		}
		if i != bitLen-1 {
			addend.Add(addend, addend)
		}
	}
	return result
}
