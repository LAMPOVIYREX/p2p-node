package dht

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/identity"
)

func mkTestIdentity(t *testing.T) *identity.Identity {
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

func TestNodeRecordSignAndValidate(t *testing.T) {
	id := mkTestIdentity(t)
	rec := NewNodeRecord(id, []string{"127.0.0.1:9101"}, 1, 5*time.Minute, "my-alias")

	if err := rec.Validate(time.Now()); err == nil {
		t.Fatal("unsigned record should fail validation")
	}

	if err := rec.Sign(id.Private); err != nil {
		t.Fatal(err)
	}

	if err := rec.Validate(time.Now()); err != nil {
		t.Fatalf("valid record failed: %v", err)
	}
}

func TestNodeRecordTampered(t *testing.T) {
	id := mkTestIdentity(t)
	rec := NewNodeRecord(id, []string{"127.0.0.1:9101"}, 1, 5*time.Minute, "")
	_ = rec.Sign(id.Private)

	// Меняем sequence_number после подписи (симуляция атаки)
	rec.SequenceNumber = 2
	if err := rec.Validate(time.Now()); err != ErrInvalidSignature {
		t.Fatalf("want ErrInvalidSignature, got %v", err)
	}
}

func TestNodeRecordMismatchedKey(t *testing.T) {
	id := mkTestIdentity(t)
	otherId := mkTestIdentity(t)

	rec := NewNodeRecord(id, []string{"127.0.0.1:9101"}, 1, 5*time.Minute, "")
	// Подписываем чужим ключом
	_ = rec.Sign(otherId.Private)

	if err := rec.Validate(time.Now()); err != ErrInvalidSignature && err != ErrIDKeyMismatch {
		t.Fatalf("want signature or id mismatch error, got %v", err)
	}
}

func TestNodeRecordExpired(t *testing.T) {
	id := mkTestIdentity(t)
	// Создаём запись с нормальным TTL (1 минута)
	rec := NewNodeRecord(id, []string{"127.0.0.1:9101"}, 1, 1*time.Minute, "")
	_ = rec.Sign(id.Private)

	// Проверяем валидацию с временем, наступившим через 2 минуты
	future := time.Now().Add(2 * time.Minute)
	if err := rec.Validate(future); err != ErrExpired {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}

func TestDHTKey(t *testing.T) {
	id := mkTestIdentity(t)

	// Без alias
	rec1 := NewNodeRecord(id, []string{"addr"}, 1, time.Minute, "")
	key1 := rec1.DHTKey()

	expectedData := append([]byte("node:"), id.NodeID[:]...)
	expected1 := sha256.Sum256(expectedData)
	if key1 != expected1 {
		t.Fatal("DHTKey mismatch for node_id")
	}

	// С alias
	rec2 := NewNodeRecord(id, []string{"addr"}, 1, time.Minute, "  Ivan228 ")
	key2 := rec2.DHTKey()

	expected2 := sha256.Sum256([]byte("alias:ivan228"))
	if key2 != expected2 {
		t.Fatal("DHTKey mismatch for alias (normalization failed)")
	}
}
