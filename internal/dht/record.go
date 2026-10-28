// Package dht реализует логику хранения и валидации DHT-записей.
package dht

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/identity"
	"github.com/LAMPOVIYREX/p2p-node/internal/protocol"
)

var (
	ErrExpired          = errors.New("record has expired")
	ErrInvalidSignature = errors.New("invalid record signature")
	ErrIDKeyMismatch    = errors.New("record NodeID does not match public key")
	ErrBadAddress       = errors.New("record must contain at least one address")
	ErrBadTimeRange     = errors.New("expires_at must be greater than issued_at")
)

// NodeRecord — подписанная адресная запись узла.
type NodeRecord struct {
	NodeID            identity.NodeID
	IdentityPublicKey ed25519.PublicKey
	Addresses         []string
	SequenceNumber    uint64
	IssuedAt          uint64 // Unix ms
	ExpiresAt         uint64 // Unix ms
	Alias             string
	Signature         []byte
}

// recordPayload — каноническое представление записи для подписи и хэширования.
// Не содержит поля Signature. Порядок полей фиксирован.
type recordPayload struct {
	NodeID            string   `json:"node_id"`
	IdentityPublicKey string   `json:"identity_public_key"`
	Addresses         []string `json:"addresses"`
	SequenceNumber    uint64   `json:"sequence_number"`
	IssuedAt          uint64   `json:"issued_at"`
	ExpiresAt         uint64   `json:"expires_at"`
	Alias             string   `json:"alias,omitempty"`
}

// NewNodeRecord создаёт новую запись для узла.
func NewNodeRecord(id *identity.Identity, addresses []string, seq uint64, ttl time.Duration, alias string) *NodeRecord {
	now := uint64(time.Now().UnixMilli())
	return &NodeRecord{
		NodeID:            id.NodeID,
		IdentityPublicKey: id.Public,
		Addresses:         addresses,
		SequenceNumber:    seq,
		IssuedAt:          now,
		ExpiresAt:         now + uint64(ttl.Milliseconds()),
		Alias:             alias,
	}
}

// canonicalBytes возвращает детерминированное байтовое представление для подписи.
func (r *NodeRecord) canonicalBytes() ([]byte, error) {
	p := recordPayload{
		NodeID:            hex.EncodeToString(r.NodeID[:]),
		IdentityPublicKey: base64.StdEncoding.EncodeToString(r.IdentityPublicKey),
		Addresses:         r.Addresses,
		SequenceNumber:    r.SequenceNumber,
		IssuedAt:          r.IssuedAt,
		ExpiresAt:         r.ExpiresAt,
		Alias:             r.Alias,
	}
	return json.Marshal(p)
}

// Sign вычисляет и сохраняет подпись записи.
func (r *NodeRecord) Sign(priv ed25519.PrivateKey) error {
	data, err := r.canonicalBytes()
	if err != nil {
		return err
	}
	r.Signature = ed25519.Sign(priv, data)
	return nil
}

// Validate проверяет запись перед сохранением или использованием.
// now передаётся для удобства тестирования (проверка TTL).
func (r *NodeRecord) Validate(now time.Time) error {
	if len(r.Addresses) == 0 {
		return ErrBadAddress
	}
	if r.ExpiresAt <= r.IssuedAt {
		return ErrBadTimeRange
	}
	if uint64(now.UnixMilli()) > r.ExpiresAt {
		return ErrExpired
	}
	if identity.DeriveNodeID(r.IdentityPublicKey) != r.NodeID {
		return ErrIDKeyMismatch
	}

	data, err := r.canonicalBytes()
	if err != nil {
		return err
	}
	if !ed25519.Verify(r.IdentityPublicKey, data, r.Signature) {
		return ErrInvalidSignature
	}
	return nil
}

// NormalizeAlias приводит псевдоним к каноническому виду.
func NormalizeAlias(alias string) string {
	return strings.ToLower(strings.TrimSpace(alias))
}

// DHTKey возвращает ключ, по которому запись будет храниться в DHT.
// Если задан alias — хэшируется он, иначе — NodeID.
func (r *NodeRecord) DHTKey() [32]byte {
	var data []byte
	if r.Alias != "" {
		data = []byte("alias:" + NormalizeAlias(r.Alias))
	} else {
		data = append([]byte("node:"), r.NodeID[:]...)
	}
	return sha256.Sum256(data)
}

// ToWire конвертирует запись в формат для передачи по сети.
func (r *NodeRecord) ToWire() protocol.WireNodeRecord {
	return protocol.WireNodeRecord{
		NodeID:            hex.EncodeToString(r.NodeID[:]),
		IdentityPublicKey: base64.StdEncoding.EncodeToString(r.IdentityPublicKey),
		Addresses:         r.Addresses,
		SequenceNumber:    r.SequenceNumber,
		IssuedAt:          r.IssuedAt,
		ExpiresAt:         r.ExpiresAt,
		Alias:             r.Alias,
		Signature:         base64.StdEncoding.EncodeToString(r.Signature),
	}
}

// NodeRecordFromWire декодирует сетевую структуру.
func NodeRecordFromWire(w protocol.WireNodeRecord) (*NodeRecord, error) {
	idBytes, err := hex.DecodeString(w.NodeID)
	if err != nil || len(idBytes) != 32 {
		return nil, fmt.Errorf("bad node_id: %w", err)
	}
	pub, err := base64.StdEncoding.DecodeString(w.IdentityPublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("bad public key: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(w.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf("bad signature: %w", err)
	}

	var id identity.NodeID
	copy(id[:], idBytes)

	return &NodeRecord{
		NodeID:            id,
		IdentityPublicKey: ed25519.PublicKey(pub),
		Addresses:         w.Addresses,
		SequenceNumber:    w.SequenceNumber,
		IssuedAt:          w.IssuedAt,
		ExpiresAt:         w.ExpiresAt,
		Alias:             w.Alias,
		Signature:         sig,
	}, nil
}
