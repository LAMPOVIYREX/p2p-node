package dht

import (
	"testing"
	"time"
)

func TestStoragePutGet(t *testing.T) {
	s := NewStorage()
	defer s.Close()

	id := mkTestIdentity(t)
	rec := NewNodeRecord(id, []string{"127.0.0.1:9101"}, 1, 5*time.Minute, "")
	_ = rec.Sign(id.Private)

	if err := s.Put(rec); err != nil {
		t.Fatalf("Put: %v", err)
	}

	key := rec.DHTKey()
	got, ok := s.Get(key)
	if !ok {
		t.Fatal("record not found")
	}
	if got.SequenceNumber != 1 {
		t.Fatalf("wrong record returned")
	}
}

func TestStorageRejectRollback(t *testing.T) {
	s := NewStorage()
	defer s.Close()

	id := mkTestIdentity(t)

	// seq=2 сохраняем
	rec2 := NewNodeRecord(id, []string{"127.0.0.1:9101"}, 2, 5*time.Minute, "")
	_ = rec2.Sign(id.Private)
	_ = s.Put(rec2)

	// seq=1 пытаемся сохранить (откат)
	rec1 := NewNodeRecord(id, []string{"127.0.0.1:9101"}, 1, 5*time.Minute, "")
	_ = rec1.Sign(id.Private)

	err := s.Put(rec1)
	if err == nil {
		t.Fatal("expected rollback error, got nil")
	}

	// Проверяем, что в хранилище остался seq=2
	got, _ := s.Get(rec2.DHTKey())
	if got.SequenceNumber != 2 {
		t.Fatalf("rollback replaced record: got seq=%d", got.SequenceNumber)
	}
}

func TestStorageExpiry(t *testing.T) {
	s := NewStorage()
	defer s.Close()

	id := mkTestIdentity(t)
	// TTL = 10ms
	rec := NewNodeRecord(id, []string{"127.0.0.1:9101"}, 1, 10*time.Millisecond, "")
	_ = rec.Sign(id.Private)
	_ = s.Put(rec)

	time.Sleep(20 * time.Millisecond)

	_, ok := s.Get(rec.DHTKey())
	if ok {
		t.Fatal("expired record still returned")
	}
}
