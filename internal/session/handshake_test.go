package session

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"net"
	"testing"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/crypto"
	"github.com/LAMPOVIYREX/p2p-node/internal/identity"
)

func mkID(t *testing.T) *identity.Identity {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &identity.Identity{
		Private: priv,
		Public:  pub,
		NodeID:  identity.DeriveNodeID(pub),
	}
}

// T4.1: успешное рукопожатие, стороны видят правильные NodeID.
func TestHandshakeSuccess(t *testing.T) {
	alice := mkID(t)
	bob := mkID(t)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	type cres struct {
		sess *Session
		err  error
	}
	type sres struct {
		sess   *Session
		peerID *identity.NodeID
		err    error
	}
	clientCh := make(chan cres, 1)
	serverCh := make(chan sres, 1)

	go func() {
		s, err := ClientHandshake(clientConn, alice)
		clientCh <- cres{s, err}
	}()
	go func() {
		s, p, err := ServerHandshake(serverConn, bob)
		serverCh <- sres{s, p, err}
	}()

	var cr cres
	var sr sres
	select {
	case cr = <-clientCh:
	case <-time.After(5 * time.Second):
		t.Fatal("client timeout")
	}
	select {
	case sr = <-serverCh:
	case <-time.After(5 * time.Second):
		t.Fatal("server timeout")
	}

	if cr.err != nil {
		t.Fatalf("client: %v", cr.err)
	}
	if sr.err != nil {
		t.Fatalf("server: %v", sr.err)
	}
	if cr.sess.PeerID != bob.NodeID {
		t.Fatal("client sees wrong peer")
	}
	if sr.sess.PeerID != alice.NodeID {
		t.Fatal("server sees wrong peer")
	}
	if cr.sess.ID != sr.sess.ID {
		t.Fatal("session IDs differ")
	}
}

// T4.2: SecureConn передаёт данные в обе стороны.
func TestHandshakeSecureConn(t *testing.T) {
	alice := mkID(t)
	bob := mkID(t)

	clientConn, serverConn := net.Pipe()

	clientCh := make(chan *SecureConn, 1)
	serverCh := make(chan *SecureConn, 1)

	go func() {
		s, err := ClientHandshake(clientConn, alice)
		if err != nil {
			t.Errorf("client: %v", err)
			clientCh <- nil
			return
		}
		clientCh <- NewSecureConn(clientConn, s)
	}()
	go func() {
		s, _, err := ServerHandshake(serverConn, bob)
		if err != nil {
			t.Errorf("server: %v", err)
			serverCh <- nil
			return
		}
		serverCh <- NewSecureConn(serverConn, s)
	}()

	var clientSC, serverSC *SecureConn
	select {
	case clientSC = <-clientCh:
	case <-time.After(5 * time.Second):
		t.Fatal("client timeout")
	}
	select {
	case serverSC = <-serverCh:
	case <-time.After(5 * time.Second):
		t.Fatal("server timeout")
	}
	if clientSC == nil || serverSC == nil {
		t.Fatal("handshake failed")
	}
	defer clientSC.Close()
	defer serverSC.Close()

	// Client → Server
	msg1 := []byte("ping from client to server")
	done1 := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 200)
		n, err := serverSC.Read(buf)
		if err != nil {
			done1 <- nil
			return
		}
		done1 <- buf[:n]
	}()
	if _, err := clientSC.Write(msg1); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case got := <-done1:
		if string(got) != string(msg1) {
			t.Fatalf("got %q, want %q", got, msg1)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("read timeout")
	}

	// Server → Client
	msg2 := []byte("pong from server to client")
	done2 := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 200)
		n, err := clientSC.Read(buf)
		if err != nil {
			done2 <- nil
			return
		}
		done2 <- buf[:n]
	}()
	if _, err := serverSC.Write(msg2); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case got := <-done2:
		if string(got) != string(msg2) {
			t.Fatalf("got %q, want %q", got, msg2)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("read timeout")
	}
}

// T4.3: сервер отклоняет попытку подмены identity.
func TestHandshakeRejectsBadSignature(t *testing.T) {
	alice := mkID(t)
	bob := mkID(t)
	mallory := mkID(t)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	errCh := make(chan error, 1)
	go func() {
		_, _, err := ServerHandshake(serverConn, bob)
		errCh <- err
	}()

	// Mallory отправляет hello с NodeID Алисы и ключом Mallory.
	ephPub, _, err := crypto.GenerateX25519Keypair()
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 32)
	rand.Read(nonce)

	hello := helloMsg{
		Version:      protocolVersion,
		SessionID:    "forged-session",
		NodeID:       hex.EncodeToString(alice.NodeID[:]),
		IdentityPub:  base64.StdEncoding.EncodeToString(mallory.Public),
		EphemeralPub: base64.StdEncoding.EncodeToString(ephPub),
		Nonce:        base64.StdEncoding.EncodeToString(nonce),
	}
	if err := writeMsg(clientConn, hello); err != nil {
		t.Fatalf("write hello: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("server accepted forged identity")
		}
		t.Logf("server correctly rejected forged identity: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}