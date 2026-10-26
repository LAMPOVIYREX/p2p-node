package routing

import (
	"bytes"
	"log/slog"
	"math/bits"
	"sort"
	"sync"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
)

type Table struct {
	self    NodeID
	k       int
	buckets [256]*Bucket
	mu      sync.RWMutex
	pinger  func(*Contact) bool
	logger  *slog.Logger
}

func New(self NodeID, k int, pinger func(*Contact) bool, logger *slog.Logger) *Table {
	if logger == nil {
		logger = logging.Discard()
	}
	t := &Table{self: self, k: k, pinger: pinger, logger: logger}
	for i := range t.buckets {
		t.buckets[i] = newBucket(k)
	}
	return t
}

func bucketIndex(self, other NodeID) int {
	d := Xor(self, other)
	for i := 0; i < 32; i++ {
		if d[i] == 0 {
			continue
		}
		j := 7 - bits.LeadingZeros8(d[i])
		return (31-i)*8 + j
	}
	return -1
}

func nowMs() uint64 { return uint64(time.Now().UnixMilli()) }

func (t *Table) AddOrUpdate(c *Contact) {
	if c.NodeID == t.self {
		t.logger.Debug("contact rejected", "reason", "self")
		return
	}
	if err := c.Validate(); err != nil {
		t.logger.Debug("contact rejected",
			"reason", "validate",
			"contact_id", logging.ShortBytes(c.NodeID[:]),
			"err", err.Error(),
		)
		return
	}
	idx := bucketIndex(t.self, c.NodeID)
	if idx < 0 {
		t.logger.Debug("contact rejected",
			"reason", "no bucket",
			"contact_id", logging.ShortBytes(c.NodeID[:]),
		)
		return
	}

	t.mu.Lock()
	b := t.buckets[idx]
	if i := b.Find(c.NodeID); i >= 0 {
		existing := b.contacts[i]
		if !bytes.Equal(existing.IdentityPublicKey, c.IdentityPublicKey) {
			t.logger.Warn("contact key mismatch, rejected",
				"bucket", idx,
				"contact_id", logging.ShortBytes(c.NodeID[:]),
			)
			t.mu.Unlock()
			return
		}
		existing.Host = c.Host
		existing.Port = c.Port
		existing.LastSeenMs = nowMs()
		existing.LastVerifiedMs = nowMs()
		b.MoveToEnd(i)
		t.logger.Debug("contact updated",
			"bucket", idx,
			"contact_id", logging.ShortBytes(c.NodeID[:]),
			"addr", existing.Addr(),
		)
		t.mu.Unlock()
		return
	}
	if !b.IsFull() {
		c.LastSeenMs = nowMs()
		c.LastVerifiedMs = nowMs()
		b.Append(c)
		t.logger.Debug("contact added",
			"bucket", idx,
			"contact_id", logging.ShortBytes(c.NodeID[:]),
			"bucket_size", b.Len(),
		)
		t.mu.Unlock()
		return
	}
	lru := *b.LRU()
	t.mu.Unlock()

	if t.pinger == nil {
		t.logger.Debug("bucket full, no pinger, contact dropped",
			"bucket", idx,
			"contact_id", logging.ShortBytes(c.NodeID[:]),
		)
		return
	}

	lruID := lru.NodeID
	candidateID := c.NodeID
	go func() {
		alive := t.pinger(&lru)
		t.mu.Lock()
		defer t.mu.Unlock()
		b := t.buckets[idx]
		if alive {
			if i := b.Find(lruID); i >= 0 {
				b.MoveToEnd(i)
			}
			t.logger.Debug("lru alive, newcomer rejected",
				"bucket", idx,
				"lru", logging.ShortBytes(lruID[:]),
				"candidate", logging.ShortBytes(candidateID[:]),
			)
			return
		}
		if i := b.Find(lruID); i >= 0 {
			b.contacts = append(b.contacts[:i], b.contacts[i+1:]...)
		}
		c.LastSeenMs = nowMs()
		c.LastVerifiedMs = nowMs()
		b.Append(c)
		t.logger.Info("lru evicted, contact added",
			"bucket", idx,
			"evicted", logging.ShortBytes(lruID[:]),
			"added", logging.ShortBytes(candidateID[:]),
		)
	}()
}

func (t *Table) FindClosest(target NodeID, n int) []*Contact {
	t.mu.RLock()
	var all []*Contact
	for _, b := range t.buckets {
		all = append(all, b.contacts...)
	}
	t.mu.RUnlock()

	sort.Slice(all, func(i, j int) bool {
		return CompareDist(Xor(all[i].NodeID, target), Xor(all[j].NodeID, target)) < 0
	})
	if n > 0 && len(all) > n {
		all = all[:n]
	}
	return all
}

func (t *Table) AllContacts() []*Contact {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var all []*Contact
	for _, b := range t.buckets {
		all = append(all, b.contacts...)
	}
	return all
}

// Reset удаляет все контакты из таблицы.
func (t *Table) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := range t.buckets {
		t.buckets[i] = newBucket(t.k)
	}
}

func BucketIndex(self, other NodeID) int {
	return bucketIndex(self, other)
}
