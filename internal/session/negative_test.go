package session

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/crypto"
)

// T4.4: Модификация ephemeral key в helloReply приводит к отказу
// верификации подписи сервера.
func TestT4_4_TamperedServerEphemeral(t *testing.T) {
	alice := mkID(t)
	bob := mkID(t)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// Вручную симулируем сервер, чтобы иметь возможность испортить
	// ephemeral key в ответе.
	go func() {
		var hello helloMsg
		if err := readMsg(serverConn, &hello); err != nil {
			return
		}
		ephPub, _, _ := crypto.GenerateX25519Keypair()
		nonce := make([]byte, 32)
		rand.Read(nonce)

		tx := transcript{
			Version:            protocolVersion,
			SessionID:          hello.SessionID,
			ClientNodeID:       hello.NodeID,
			ClientIdentityPub:  hello.IdentityPub,
			ClientEphemeralPub: hello.EphemeralPub,
			ClientNonce:        hello.Nonce,
			ServerNodeID:       hex.EncodeToString(bob.NodeID[:]),
			ServerIdentityPub:  base64.StdEncoding.EncodeToString(bob.Public),
			ServerEphemeralPub: base64.StdEncoding.EncodeToString(ephPub),
			ServerNonce:        base64.StdEncoding.EncodeToString(nonce),
		}
		txBytes, _ := json.Marshal(tx)
		sig := ed25519.Sign(bob.Private, txBytes)

		// ВАЖНО: подменяем ephemeral key после подписи
		badEph := make([]byte, 32)
		rand.Read(badEph)
		reply := helloReplyMsg{
			helloMsg: helloMsg{
				Version:      protocolVersion,
				SessionID:    hello.SessionID,
				NodeID:       tx.ServerNodeID,
				IdentityPub:  tx.ServerIdentityPub,
				EphemeralPub: base64.StdEncoding.EncodeToString(badEph), // ← подмена
				Nonce:        tx.ServerNonce,
			},
			Signature: base64.StdEncoding.EncodeToString(sig),
		}
		_ = writeMsg(serverConn, reply)
	}()

	_, err := ClientHandshake(clientConn, alice)
	if err == nil {
		t.Fatal("client accepted tampered ephemeral key")
	}
	t.Logf("client correctly rejected tampered ephemeral: %v", err)
}

// T4.5: Модификация ciphertext SecureConn ломает расшифровку.
func TestT4_5_TamperedCiphertext(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, crypto.KeySize)
	nonce := bytes.Repeat([]byte{0x01}, crypto.NonceSize)
	plaintext := []byte("secure payload")
	ciphertext, err := crypto.Encrypt(key, nonce, plaintext, nil)
	if err != nil {
		t.Fatal(err)
	}

	ciphertext[5] ^= 0xFF
	if _, err := crypto.Decrypt(key, nonce, ciphertext, nil); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}

// T4.6: Клиент с валидным hello, но неверной подписью confirm — отклоняется.
func TestT4_6_BadClientConfirmSignature(t *testing.T) {
	alice := mkID(t)
	bob := mkID(t)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	errCh := make(chan error, 1)
	go func() {
		_, _, err := ServerHandshake(serverConn, bob)
		errCh <- err
	}()

	// Клиент отправляет hello с валидными ключами
	ephPub, _, _ := crypto.GenerateX25519Keypair()
	nonce := make([]byte, 32)
	rand.Read(nonce)
	hello := helloMsg{
		Version:      protocolVersion,
		SessionID:    "test-session",
		NodeID:       hex.EncodeToString(alice.NodeID[:]),
		IdentityPub:  base64.StdEncoding.EncodeToString(alice.Public),
		EphemeralPub: base64.StdEncoding.EncodeToString(ephPub),
		Nonce:        base64.StdEncoding.EncodeToString(nonce),
	}
	if err := writeMsg(clientConn, hello); err != nil {
		t.Fatal(err)
	}

	// Читаем ответ сервера
	var reply helloReplyMsg
	if err := readMsg(clientConn, &reply); err != nil {
		t.Fatal(err)
	}

	// Отправляем НЕВЕРНУЮ подпись confirm
	badSig := make([]byte, ed25519.SignatureSize)
	rand.Read(badSig)
	_ = writeMsg(clientConn, confirmMsg{Signature: base64.StdEncoding.EncodeToString(badSig)})

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("server accepted bad client signature")
		}
		t.Logf("server correctly rejected bad signature: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

// T4.7: Повтор защищённого кадра (replay) отклоняется монотонным
// счётчиком nonce в SecureConn.readNextFrame.
//
// Проверяем на уровне SecureConn: если удалённая сторона присылает
// кадр с nonce <= recvCounter, чтение должно вернуть ошибку.
func TestT4_7_ReplayRejected(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, crypto.KeySize)
	recvKey := bytes.Repeat([]byte{0x42}, crypto.KeySize)
	plaintext := []byte("replay-target")

	// Создаём поддельную "сессию" сервера с уже обработанным recvCounter = 10
	sess := &Session{
		ID:          "test-replay",
		sendKey:     key,
		recvKey:     recvKey,
		sendCounter: 0,
		recvCounter: 10,
	}

	// Атакующий шифрует кадр со старым nonce (5 < 10) и присылает его.
	oldNonce := make([]byte, crypto.NonceSize)
	binary.BigEndian.PutUint64(oldNonce[4:], 5) // старый счётчик

	ciphertext, err := crypto.Encrypt(recvKey, oldNonce, plaintext, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Формируем сырой фрейм в том же формате, что SecureConn ожидает.
	var buf bytes.Buffer
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(ciphertext)))
	buf.Write(lenBuf[:])
	buf.Write(oldNonce)
	buf.Write(ciphertext)

	// Подсовываем этот фрейм как входящие данные.
	sc := &SecureConn{
		Conn:    &bufConn{Reader: &buf, Writer: io.Discard},
		session: sess,
	}

	readBuf := make([]byte, 128)
	_, readErr := sc.Read(readBuf)
	if readErr == nil {
		t.Fatal("replay accepted: old nonce (5 < recvCounter 10) was not rejected")
	}
	if !strings.Contains(readErr.Error(), "replay") {
		t.Fatalf("want replay error, got: %v", readErr)
	}
	t.Logf("replay correctly rejected: %v", readErr)

	// Контрольная проверка: свежий nonce (11 > 10) принимается.
	freshNonce := make([]byte, crypto.NonceSize)
	binary.BigEndian.PutUint64(freshNonce[4:], 11)
	freshCT, _ := crypto.Encrypt(recvKey, freshNonce, plaintext, nil)

	var buf2 bytes.Buffer
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(freshCT)))
	buf2.Write(lenBuf[:])
	buf2.Write(freshNonce)
	buf2.Write(freshCT)

	sc.Conn = &bufConn{Reader: &buf2, Writer: io.Discard}
	_, err = sc.Read(readBuf)
	if err != nil {
		t.Fatalf("fresh nonce rejected: %v", err)
	}
	if sess.recvCounter != 11 {
		t.Fatalf("recvCounter = %d, want 11", sess.recvCounter)
	}
}

// bufConn — минимальная реализация net.Conn для теста.
type bufConn struct {
	io.Reader
	io.Writer
}

func (c *bufConn) Read(b []byte) (int, error)         { return c.Reader.Read(b) }
func (c *bufConn) Write(b []byte) (int, error)        { return c.Writer.Write(b) }
func (c *bufConn) Close() error                       { return nil }
func (c *bufConn) LocalAddr() net.Addr                { return dummyAddr{} }
func (c *bufConn) RemoteAddr() net.Addr               { return dummyAddr{} }
func (c *bufConn) SetDeadline(t time.Time) error      { return nil }
func (c *bufConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *bufConn) SetWriteDeadline(t time.Time) error { return nil }

type dummyAddr struct{}

func (dummyAddr) Network() string { return "test" }
func (dummyAddr) String() string  { return "dummy" }
