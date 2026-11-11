// Package session реализует AKE (Authenticated Key Exchange) и защищённый транспорт.
package session

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/google/uuid"

	"github.com/LAMPOVIYREX/p2p-node/internal/crypto"
	"github.com/LAMPOVIYREX/p2p-node/internal/identity"
)

const (
	protocolVersion  = 1
	maxHandshakeSize = 8192
	handshakeTimeout = 5 * time.Second
	hkdfInfo         = "p2p-ake-v1"
)

// Session содержит состояние защищённой сессии.
type Session struct {
	ID          string
	PeerID      identity.NodeID
	sendKey     []byte
	recvKey     []byte
	sendCounter uint64
	recvCounter uint64
}

// transcript — канонический транскрипт рукопожатия для подписи.
// Порядок и состав полей фиксированы.
type transcript struct {
	Version            uint8  `json:"version"`
	SessionID          string `json:"session_id"`
	ClientNodeID       string `json:"client_node_id"`
	ClientIdentityPub  string `json:"client_identity_pub"`
	ClientEphemeralPub string `json:"client_ephemeral_pub"`
	ClientNonce        string `json:"client_nonce"`
	ServerNodeID       string `json:"server_node_id"`
	ServerIdentityPub  string `json:"server_identity_pub"`
	ServerEphemeralPub string `json:"server_ephemeral_pub"`
	ServerNonce        string `json:"server_nonce"`
}

type helloMsg struct {
	Version      uint8  `json:"version"`
	SessionID    string `json:"session_id"`
	NodeID       string `json:"node_id"`
	IdentityPub  string `json:"identity_pub"`
	EphemeralPub string `json:"ephemeral_pub"`
	Nonce        string `json:"nonce"`
}

type helloReplyMsg struct {
	helloMsg
	Signature string `json:"signature"`
}

type confirmMsg struct {
	Signature string `json:"signature"`
}

// ClientHandshake выполняет рукопожатие со стороны клиента (инициатора).
// Возвращает установленную сессию с ключами.
func ClientHandshake(conn net.Conn, id *identity.Identity) (*Session, error) {
	_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))

	ephPub, ephPriv, err := crypto.GenerateX25519Keypair()
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	sessionID := uuid.New().String()

	hello := helloMsg{
		Version:      protocolVersion,
		SessionID:    sessionID,
		NodeID:       hex.EncodeToString(id.NodeID[:]),
		IdentityPub:  base64.StdEncoding.EncodeToString(id.Public),
		EphemeralPub: base64.StdEncoding.EncodeToString(ephPub),
		Nonce:        base64.StdEncoding.EncodeToString(nonce),
	}
	if err := writeMsg(conn, hello); err != nil {
		return nil, fmt.Errorf("send hello: %w", err)
	}

	var reply helloReplyMsg
	if err := readMsg(conn, &reply); err != nil {
		return nil, fmt.Errorf("read hello reply: %w", err)
	}

	serverID, _, serverEphPub, txBytes, err := verifyServerReply(hello, reply)
	if err != nil {
		return nil, err
	}

	// Вычисляем общий секрет
	sharedSecret, err := crypto.ComputeSharedSecret(ephPriv, serverEphPub)
	if err != nil {
		return nil, err
	}

	// Подписываем транскрипт
	clientSig := ed25519.Sign(id.Private, txBytes)
	confirm := confirmMsg{Signature: base64.StdEncoding.EncodeToString(clientSig)}
	if err := writeMsg(conn, confirm); err != nil {
		return nil, fmt.Errorf("send confirm: %w", err)
	}

	// Деривация ключей (isClient = true)
	sendKey, recvKey, err := crypto.DeriveKeys(sharedSecret, []byte(sessionID), []byte(hkdfInfo), true)
	if err != nil {
		return nil, err
	}

	_ = conn.SetDeadline(time.Time{})
	return &Session{
		ID:      sessionID,
		PeerID:  serverID,
		sendKey: sendKey,
		recvKey: recvKey,
	}, nil
}

// ServerHandshake выполняет рукопожатие со стороны сервера (ответчика).
// Возвращает сессию и NodeID клиента.
func ServerHandshake(conn net.Conn, id *identity.Identity) (*Session, *identity.NodeID, error) {
	_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))

	var hello helloMsg
	if err := readMsg(conn, &hello); err != nil {
		return nil, nil, fmt.Errorf("read hello: %w", err)
	}
	if hello.Version != protocolVersion {
		return nil, nil, errors.New("unsupported protocol version")
	}

	clientID, clientPub, clientEphPub, err := verifyClientHello(hello)
	if err != nil {
		return nil, nil, err
	}

	// Генерируем свой эфемерный ключ
	ephPub, ephPriv, err := crypto.GenerateX25519Keypair()
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}

	// Транскрипт
	tx := transcript{
		Version:            protocolVersion,
		SessionID:          hello.SessionID,
		ClientNodeID:       hello.NodeID,
		ClientIdentityPub:  hello.IdentityPub,
		ClientEphemeralPub: hello.EphemeralPub,
		ClientNonce:        hello.Nonce,
		ServerNodeID:       hex.EncodeToString(id.NodeID[:]),
		ServerIdentityPub:  base64.StdEncoding.EncodeToString(id.Public),
		ServerEphemeralPub: base64.StdEncoding.EncodeToString(ephPub),
		ServerNonce:        base64.StdEncoding.EncodeToString(nonce),
	}
	txBytes, _ := json.Marshal(tx)
	serverSig := ed25519.Sign(id.Private, txBytes)

	reply := helloReplyMsg{
		helloMsg: helloMsg{
			Version:      protocolVersion,
			SessionID:    hello.SessionID,
			NodeID:       tx.ServerNodeID,
			IdentityPub:  tx.ServerIdentityPub,
			EphemeralPub: tx.ServerEphemeralPub,
			Nonce:        tx.ServerNonce,
		},
		Signature: base64.StdEncoding.EncodeToString(serverSig),
	}
	if err := writeMsg(conn, reply); err != nil {
		return nil, nil, err
	}

	// Читаем подтверждение клиента
	var confirm confirmMsg
	if err := readMsg(conn, &confirm); err != nil {
		return nil, nil, err
	}
	clientSig, err := base64.StdEncoding.DecodeString(confirm.Signature)
	if err != nil {
		return nil, nil, err
	}
	if !ed25519.Verify(clientPub, txBytes, clientSig) {
		return nil, nil, errors.New("invalid client signature")
	}

	// Общий секрет
	sharedSecret, err := crypto.ComputeSharedSecret(ephPriv, clientEphPub)
	if err != nil {
		return nil, nil, err
	}

	// Деривация (isClient = false)
	sendKey, recvKey, err := crypto.DeriveKeys(sharedSecret, []byte(hello.SessionID), []byte(hkdfInfo), false)
	if err != nil {
		return nil, nil, err
	}

	_ = conn.SetDeadline(time.Time{})
	return &Session{
		ID:      hello.SessionID,
		PeerID:  clientID,
		sendKey: sendKey,
		recvKey: recvKey,
	}, &clientID, nil
}

func verifyClientHello(h helloMsg) (identity.NodeID, ed25519.PublicKey, []byte, error) {
	var zeroID identity.NodeID

	clientID, err := parseNodeID(h.NodeID)
	if err != nil {
		return zeroID, nil, nil, fmt.Errorf("client node id: %w", err)
	}
	clientPub, err := base64.StdEncoding.DecodeString(h.IdentityPub)
	if err != nil || len(clientPub) != ed25519.PublicKeySize {
		return zeroID, nil, nil, errors.New("bad client pubkey")
	}
	if identity.DeriveNodeID(clientPub) != clientID {
		return zeroID, nil, nil, errors.New("client NodeID does not match pubkey")
	}
	clientEphPub, err := base64.StdEncoding.DecodeString(h.EphemeralPub)
	if err != nil || len(clientEphPub) != 32 {
		return zeroID, nil, nil, errors.New("bad client ephemeral pubkey")
	}
	return clientID, clientPub, clientEphPub, nil
}

func verifyServerReply(hello helloMsg, reply helloReplyMsg) (identity.NodeID, ed25519.PublicKey, []byte, []byte, error) {
	var zeroID identity.NodeID

	serverID, err := parseNodeID(reply.NodeID)
	if err != nil {
		return zeroID, nil, nil, nil, fmt.Errorf("server node id: %w", err)
	}
	serverPub, err := base64.StdEncoding.DecodeString(reply.IdentityPub)
	if err != nil || len(serverPub) != ed25519.PublicKeySize {
		return zeroID, nil, nil, nil, errors.New("bad server pubkey")
	}
	if identity.DeriveNodeID(serverPub) != serverID {
		return zeroID, nil, nil, nil, errors.New("server NodeID does not match pubkey")
	}
	serverEphPub, err := base64.StdEncoding.DecodeString(reply.EphemeralPub)
	if err != nil || len(serverEphPub) != 32 {
		return zeroID, nil, nil, nil, errors.New("bad server ephemeral pubkey")
	}

	tx := transcript{
		Version:            protocolVersion,
		SessionID:          hello.SessionID,
		ClientNodeID:       hello.NodeID,
		ClientIdentityPub:  hello.IdentityPub,
		ClientEphemeralPub: hello.EphemeralPub,
		ClientNonce:        hello.Nonce,
		ServerNodeID:       reply.NodeID,
		ServerIdentityPub:  reply.IdentityPub,
		ServerEphemeralPub: reply.EphemeralPub,
		ServerNonce:        reply.Nonce,
	}
	txBytes, _ := json.Marshal(tx)

	sig, err := base64.StdEncoding.DecodeString(reply.Signature)
	if err != nil {
		return zeroID, nil, nil, nil, err
	}
	if !ed25519.Verify(serverPub, txBytes, sig) {
		return zeroID, nil, nil, nil, errors.New("invalid server signature")
	}
	return serverID, serverPub, serverEphPub, txBytes, nil
}

func writeMsg(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(data) > maxHandshakeSize {
		return errors.New("handshake message too large")
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func readMsg(r io.Reader, v any) error {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(lenBuf[:])
	if n > maxHandshakeSize {
		return errors.New("handshake message too large")
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(r, data); err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func parseNodeID(s string) (identity.NodeID, error) {
	var id identity.NodeID
	b, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}
	if len(b) != 32 {
		return id, fmt.Errorf("node id must be 32 bytes, got %d", len(b))
	}
	copy(id[:], b)
	return id, nil
}
