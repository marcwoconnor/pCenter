package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// TestCopySSHKey_EndToEnd exercises the full native-Go SSH bootstrap path
// against an in-process SSH server. Regression coverage for the removal of the
// external sshpass/ssh-copy-id dependency.
func TestCopySSHKey_EndToEnd(t *testing.T) {
	// --- Fake HOME with an ed25519 keypair (what ensureSSHKeypair would produce)
	homeDir := t.TempDir()
	sshDir := filepath.Join(homeDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("mkdir sshDir: %v", err)
	}
	pubBytes, privPEM := newClientEd25519(t)
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519"), privPEM, 0o600); err != nil {
		t.Fatalf("write privkey: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519.pub"), pubBytes, 0o644); err != nil {
		t.Fatalf("write pubkey: %v", err)
	}
	t.Setenv("HOME", homeDir)

	// --- In-process SSH server
	srv := newTestSSHServer(t, "testpw", pubBytes)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp := copySSHKey(ctx, srv.Addr, "testpw")
	if !resp.Success {
		t.Fatalf("copySSHKey failed: %s", resp.Message)
	}

	// authorized_keys bytes the server captured should contain our pubkey
	got := strings.TrimSpace(string(srv.AuthorizedKeys()))
	want := strings.TrimSpace(string(pubBytes))
	if got != want {
		t.Errorf("authorized_keys mismatch\nwant: %q\ngot:  %q", want, got)
	}

	// known_hosts should now contain an entry keyed by srv.Addr
	kh, err := os.ReadFile(filepath.Join(sshDir, "known_hosts"))
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}
	if !bytes.Contains(kh, []byte(srv.Addr+" ")) {
		t.Errorf("known_hosts missing entry for %s: %q", srv.Addr, kh)
	}
}

// TestCopySSHKey_WrongPassword ensures bad credentials surface a clear error
// rather than a false-positive success.
func TestCopySSHKey_WrongPassword(t *testing.T) {
	homeDir := t.TempDir()
	sshDir := filepath.Join(homeDir, ".ssh")
	_ = os.MkdirAll(sshDir, 0o700)
	pubBytes, privPEM := newClientEd25519(t)
	_ = os.WriteFile(filepath.Join(sshDir, "id_ed25519"), privPEM, 0o600)
	_ = os.WriteFile(filepath.Join(sshDir, "id_ed25519.pub"), pubBytes, 0o644)
	t.Setenv("HOME", homeDir)

	srv := newTestSSHServer(t, "correct", pubBytes)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp := copySSHKey(ctx, srv.Addr, "wrong")
	if resp.Success {
		t.Fatal("expected failure with wrong password, got success")
	}
	if !strings.Contains(resp.Message, "ssh connect") {
		t.Errorf("expected 'ssh connect' in error, got: %s", resp.Message)
	}
}

// --- test helpers ---

func newClientEd25519(t *testing.T) (pubAuthorizedKey, privPEM []byte) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	pubAuthorizedKey = bytes.TrimRight(ssh.MarshalAuthorizedKey(signer.PublicKey()), "\n")

	// Encode ed25519 privkey as OpenSSH-parseable PKCS8 PEM so ssh.ParsePrivateKey accepts it.
	_ = pub
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal privkey: %v", err)
	}
	privPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return pubAuthorizedKey, privPEM
}

type testSSHServer struct {
	Addr string

	ln       net.Listener
	cfg      *ssh.ServerConfig
	mu       sync.Mutex
	authKeys []byte
	wg       sync.WaitGroup
}

func (s *testSSHServer) AuthorizedKeys() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.authKeys...)
}

func (s *testSSHServer) Close() {
	_ = s.ln.Close()
	s.wg.Wait()
}

func newTestSSHServer(t *testing.T, validPassword string, expectedPubKey []byte) *testSSHServer {
	t.Helper()

	// Host key (ed25519) for the server
	_, hostPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("host key gen: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatalf("host signer: %v", err)
	}

	expectedParsed, _, _, _, err := ssh.ParseAuthorizedKey(expectedPubKey)
	if err != nil {
		t.Fatalf("parse expected pubkey: %v", err)
	}

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if conn.User() == "root" && string(pw) == validPassword {
				return &ssh.Permissions{}, nil
			}
			return nil, &ssh.PartialSuccessError{}
		},
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if conn.User() == "root" && bytes.Equal(key.Marshal(), expectedParsed.Marshal()) {
				return &ssh.Permissions{}, nil
			}
			return nil, &ssh.PartialSuccessError{}
		},
	}
	cfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	s := &testSSHServer{
		Addr: ln.Addr().String(),
		ln:   ln,
		cfg:  cfg,
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.handleConn(c)
			}()
		}
	}()
	return s
}

func (s *testSSHServer) handleConn(nc net.Conn) {
	defer nc.Close()
	_, chans, reqs, err := ssh.NewServerConn(nc, s.cfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			_ = newCh.Reject(ssh.UnknownChannelType, "only session")
			continue
		}
		ch, reqs, err := newCh.Accept()
		if err != nil {
			return
		}
		go s.handleSession(ch, reqs)
	}
}

func (s *testSSHServer) handleSession(ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()
	for req := range reqs {
		switch req.Type {
		case "exec":
			// Reply first so the client starts pumping stdin; only then read it.
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
			buf, _ := io.ReadAll(ch)
			s.mu.Lock()
			s.authKeys = append(s.authKeys, bytes.TrimRight(buf, "\n")...)
			s.mu.Unlock()
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
			return
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}
