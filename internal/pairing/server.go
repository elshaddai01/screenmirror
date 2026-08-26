package pairing

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base32"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"screenmirror/internal/logging"

	"github.com/grandcat/zeroconf"
)

const mdnsServiceType = "_adb-tls-pairing._tcp"

// Result is delivered once a phone successfully completes pairing.
type Result struct {
	// DeviceGUID/Model are best-effort; adb wireless pairing only actually
	// requires the RSA public key to register trust, everything else is for
	// display purposes.
	PeerPublicKey string
}

// Server advertises one AirPlay-style QR-pairing target on the LAN: an
// mDNS-advertised _adb-tls-pairing._tcp service plus a TLS+SPAKE2 listener
// on the same port. Scanning the QR code it prints causes a phone's
// Wireless debugging screen to connect, authenticate with the printed
// password, and (on success) hand us its adb public key, which the caller
// is expected to add to the local adb server's trusted keys so future plain
// `adb connect`/mDNS auto-discovery needs no further prompts.
type Server struct {
	name     string
	password string
	adbPubKey string
	logger   *logging.Logger

	listener  net.Listener
	mdns      *zeroconf.Server
	port      int
}

// NewServer builds (but does not start) a pairing server advertised under
// serviceName, authenticating with a freshly generated random password.
// adbPubKey should be the exact contents of the local adbkey.pub file, so
// that a phone which trusts it will also trust adb.exe's later plain
// connections without an extra prompt.
func NewServer(serviceName, adbPubKey string, logger *logging.Logger) (*Server, error) {
	password, err := randomPassword()
	if err != nil {
		return nil, err
	}
	return &Server{
		name:      serviceName,
		password:  password,
		adbPubKey: strings.TrimSpace(adbPubKey),
		logger:    logger,
	}, nil
}

// QRText returns the string to encode as a QR code: scanning it with
// Android's "Pair device with QR code" screen is equivalent to typing the
// name/password into "Pair device with pairing code".
func (s *Server) QRText() string {
	return fmt.Sprintf("WIFI:T:ADB;S:%s;P:%s;;", s.name, s.password)
}

func randomPassword() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf), nil
}

// Start opens the TLS listener and advertises it over mDNS. It returns
// immediately; pairing results are delivered to onResult (called once per
// successful pairing, from a background goroutine) until ctx is cancelled.
func (s *Server) Start(ctx context.Context, onResult func(Result)) error {
	cert, err := generateEphemeralCert()
	if err != nil {
		return fmt.Errorf("generating pairing TLS certificate: %w", err)
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAnyClientCert,
		MinVersion:   tls.VersionTLS12,
	}

	ln, err := tls.Listen("tcp", ":0", tlsConfig)
	if err != nil {
		return fmt.Errorf("starting pairing TLS listener: %w", err)
	}
	s.listener = ln
	s.port = ln.Addr().(*net.TCPAddr).Port

	host, _ := os.Hostname()
	mdnsServer, err := zeroconf.Register(s.name, mdnsServiceType, "local.", s.port, []string{"txtvers=1", "host=" + host}, nil)
	if err != nil {
		ln.Close()
		return fmt.Errorf("advertising pairing service over mDNS: %w", err)
	}
	s.mdns = mdnsServer

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	go s.acceptLoop(ctx, onResult)

	return nil
}

func (s *Server) acceptLoop(ctx context.Context, onResult func(Result)) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.logger.Debug("pairing listener accept error: %v", err)
			continue
		}
		go s.handleConn(conn, onResult)
	}
}

func (s *Server) handleConn(conn net.Conn, onResult func(Result)) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		s.logger.Debug("pairing connection was not TLS (unexpected)")
		return
	}
	if err := tlsConn.Handshake(); err != nil {
		s.logger.Debug("pairing TLS handshake failed: %v", err)
		return
	}

	// The SPAKE2 password is the printed password concatenated with 64
	// bytes of TLS exported keying material, so that stealing/replaying the
	// printed code alone (without being the actual TLS peer) isn't enough.
	connState := tlsConn.ConnectionState()
	keyMaterial, err := connState.ExportKeyingMaterial("adb-label\x00", nil, 64)
	if err != nil {
		s.logger.Debug("failed to export TLS keying material: %v", err)
		return
	}
	fullPassword := append([]byte(s.password), keyMaterial...)

	result, err := RunServerExchange(tlsConn, fullPassword, s.adbPubKey)
	if err != nil {
		s.logger.Debug("pairing exchange failed: %v", err)
		return
	}

	onResult(Result{PeerPublicKey: result.Data})
}

// Stop tears down the mDNS advertisement and TLS listener.
func (s *Server) Stop() {
	if s.mdns != nil {
		s.mdns.Shutdown()
	}
	if s.listener != nil {
		s.listener.Close()
	}
}

// AdbKeyPath returns the default location of adb's own persistent keypair,
// so callers can read adbkey.pub to hand to NewServer.
func AdbKeyPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".android", "adbkey.pub")
}
