package session

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/LAMPOVIYREX/p2p-node/internal/crypto"
)

const maxEncryptedFrame = 2 * 1024 * 1024 // 2 MB

// SecureConn оборачивает net.Conn и обеспечивает шифрование
// ChaCha20-Poly1305 с монотонным счётчиком nonce.
// Каждый Write — один зашифрованный фрейм [4B len][12B nonce][ciphertext+tag].
// Реализует интерфейс net.Conn, поэтому прозрачен для слоёв выше.
type SecureConn struct {
	net.Conn
	session *Session

	readMu  sync.Mutex
	writeMu sync.Mutex
	readBuf []byte
}

func NewSecureConn(conn net.Conn, s *Session) *SecureConn {
	return &SecureConn{Conn: conn, session: s}
}

// Session возвращает сессию (для логов и метрик).
func (c *SecureConn) Session() *Session { return c.session }

func (c *SecureConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.session.sendCounter++
	nonce := make([]byte, crypto.NonceSize)
	binary.BigEndian.PutUint64(nonce[4:], c.session.sendCounter)

	ciphertext, err := crypto.Encrypt(c.session.sendKey, nonce, p, nil)
	if err != nil {
		return 0, err
	}

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(ciphertext)))

	if _, err := c.Conn.Write(lenBuf[:]); err != nil {
		return 0, err
	}
	if _, err := c.Conn.Write(nonce); err != nil {
		return 0, err
	}
	if _, err := c.Conn.Write(ciphertext); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *SecureConn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	if len(c.readBuf) == 0 {
		if err := c.readNextFrame(); err != nil {
			return 0, err
		}
	}

	n := copy(p, c.readBuf)
	c.readBuf = c.readBuf[n:]
	return n, nil
}

func (c *SecureConn) readNextFrame() error {
	var lenBuf [4]byte
	if _, err := io.ReadFull(c.Conn, lenBuf[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(lenBuf[:])
	if n > maxEncryptedFrame {
		return errors.New("encrypted frame too large")
	}

	nonce := make([]byte, crypto.NonceSize)
	if _, err := io.ReadFull(c.Conn, nonce); err != nil {
		return err
	}

	// Проверка монотонности: nonce должен быть строго больше recvCounter.
	// Защита от replay и переупорядочивания кадров.
	recvNonce := binary.BigEndian.Uint64(nonce[4:])
	if recvNonce <= c.session.recvCounter {
		return fmt.Errorf("replay detected: nonce %d <= recvCounter %d",
			recvNonce, c.session.recvCounter)
	}
	c.session.recvCounter = recvNonce

	ciphertext := make([]byte, n)
	if _, err := io.ReadFull(c.Conn, ciphertext); err != nil {
		return err
	}

	plaintext, err := crypto.Decrypt(c.session.recvKey, nonce, ciphertext, nil)
	if err != nil {
		return err
	}

	c.readBuf = plaintext
	return nil
}
