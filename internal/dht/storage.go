package dht

import (
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/identity"
)

// Storage — in-memory хранилище DHT-записей с автоматической очисткой по TTL.
type Storage struct {
	mu      sync.RWMutex
	records map[string]*NodeRecord
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

func NewStorage() *Storage {
	s := &Storage{
		records: make(map[string]*NodeRecord),
		stopCh:  make(chan struct{}),
	}
	s.wg.Add(1)
	go s.cleanupLoop()
	return s
}

// Put сохраняет запись. Отклоняет откат версии (sequence_number).
func (s *Storage) Put(rec *NodeRecord) error {
	keyBytes := rec.DHTKey()
	key := hex.EncodeToString(keyBytes[:])

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.records[key]; ok {
		if rec.SequenceNumber < existing.SequenceNumber {
			return fmt.Errorf("rollback rejected: seq %d < existing %d", rec.SequenceNumber, existing.SequenceNumber)
		}
		if rec.SequenceNumber == existing.SequenceNumber {
			// Дубликат (например, повторный STORE) — просто обновляем expire
			existing.ExpiresAt = rec.ExpiresAt
			return nil
		}
	}

	s.records[key] = rec
	return nil
}

// Get возвращает запись по ключу, если она есть и не истекла.
func (s *Storage) Get(key [32]byte) (*NodeRecord, bool) {
	keyHex := hex.EncodeToString(key[:])

	s.mu.RLock()
	rec, ok := s.records[keyHex]
	s.mu.RUnlock()

	if !ok {
		return nil, false
	}
	if uint64(time.Now().UnixMilli()) > rec.ExpiresAt {
		s.mu.Lock()
		delete(s.records, keyHex)
		s.mu.Unlock()
		return nil, false
	}
	return rec, true
}

// Count возвращает количество записей (для метрик).
func (s *Storage) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}

// Close останавливает фоновую горутину очистки.
func (s *Storage) Close() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Storage) cleanupLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.cleanupExpired()
		}
	}
}

func (s *Storage) cleanupExpired() {
	now := uint64(time.Now().UnixMilli())
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, rec := range s.records {
		if now > rec.ExpiresAt {
			delete(s.records, k)
		}
	}
}

// GetOwnedRecords возвращает записи, владельцем которых является self.
// Нужно для автоматического переопубликования.
func (s *Storage) GetOwnedRecords(self identity.NodeID) []*NodeRecord { // <--- identity.NodeID
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*NodeRecord
	for _, rec := range s.records {
		if rec.NodeID == self {
			result = append(result, rec)
		}
	}
	return result
}
