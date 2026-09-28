package routing

import (
	"crypto/ed25519"
	"crypto/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/identity"
)

// mkContact создаёт валидный Contact со свежей случайной Ed25519-парой.
// NodeID детерминированно вычисляется из публичного ключа, поэтому
// произвольно задать его нельзя — только перебором.
func mkContact(t *testing.T) *Contact {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &Contact{
		NodeID:            identity.DeriveNodeID(pub),
		IdentityAlgorithm: "ed25519",
		IdentityPublicKey: pub,
		Host:              "127.0.0.1",
		Port:              9000,
	}
}

// mkContactInBucket перебирает случайные ключи, пока не найдёт контакт,
// bucketIndex(self, c.NodeID) которого равен want.
// При self = 0 bucket = позиции старшего установленного бита в NodeID,
// вероятность попасть в bucket 255 — 1/2, поэтому перебор быстрый.
func mkContactInBucket(t *testing.T, self NodeID, want int) *Contact {
	t.Helper()
	for i := 0; i < 100000; i++ {
		c := mkContact(t)
		if bucketIndex(self, c.NodeID) == want {
			return c
		}
	}
	t.Fatalf("could not find contact in bucket %d after 100000 attempts", want)
	return nil
}

// T2.6: вставка в неполный bucket добавляет контакт в конец.
func TestT2_6_AddToNonFullBucket(t *testing.T) {
	var self NodeID
	var pingerCalled int32
	tbl := New(self, 4, func(*Contact) bool {
		atomic.AddInt32(&pingerCalled, 1)
		return true
	}, nil)

	var created []*Contact
	for i := 0; i < 3; i++ {
		c := mkContactInBucket(t, self, 255)
		created = append(created, c)
		tbl.AddOrUpdate(c)
	}

	if n := atomic.LoadInt32(&pingerCalled); n != 0 {
		t.Fatalf("pinger called %d times for non-full bucket", n)
	}

	all := tbl.AllContacts()
	if len(all) != len(created) {
		t.Fatalf("got %d contacts, want %d", len(all), len(created))
	}
	for i, c := range created {
		if all[i].NodeID != c.NodeID {
			t.Fatalf("order mismatch at %d", i)
		}
	}
}

// T2.7: повторное наблюдение перемещает контакт в конец bucket.
func TestT2_7_ReObserveMovesToEnd(t *testing.T) {
	var self NodeID
	tbl := New(self, 4, func(*Contact) bool { return true }, nil)

	var contacts []*Contact
	for i := 0; i < 3; i++ {
		c := mkContactInBucket(t, self, 255)
		contacts = append(contacts, c)
		tbl.AddOrUpdate(c)
	}

	// Повторно наблюдаем первый — он должен уехать в конец.
	tbl.AddOrUpdate(contacts[0])

	all := tbl.AllContacts()
	if len(all) != 3 {
		t.Fatalf("unexpected table size %d", len(all))
	}
	if all[len(all)-1].NodeID != contacts[0].NodeID {
		t.Fatal("re-observed contact not at end")
	}
}

// T2.8: полный bucket, живой LRU — старый сохраняется, новый не добавляется.
func TestT2_8_FullBucketAliveLRU(t *testing.T) {
	var self NodeID
	var pingerCalled int32
	tbl := New(self, 4, func(*Contact) bool {
		atomic.AddInt32(&pingerCalled, 1)
		return true // жив
	}, nil)

	for i := 0; i < 4; i++ {
		c := mkContactInBucket(t, self, 255)
		tbl.AddOrUpdate(c)
	}

	newcomer := mkContactInBucket(t, self, 255)
	tbl.AddOrUpdate(newcomer)

	// Ждём асинхронный pinger.
	waitFor(t, func() bool { return atomic.LoadInt32(&pingerCalled) > 0 })

	all := tbl.AllContacts()
	if len(all) != 4 {
		t.Fatalf("table size changed: %d", len(all))
	}
	for _, c := range all {
		if c.NodeID == newcomer.NodeID {
			t.Fatal("newcomer added although LRU alive")
		}
	}
}

// T2.9: полный bucket, мёртвый LRU — LRU удаляется, новый добавляется.
func TestT2_9_FullBucketDeadLRU(t *testing.T) {
	var self NodeID
	tbl := New(self, 4, func(*Contact) bool { return false }, nil)

	var inBucket []*Contact
	for i := 0; i < 4; i++ {
		c := mkContactInBucket(t, self, 255)
		inBucket = append(inBucket, c)
		tbl.AddOrUpdate(c)
	}
	lruID := inBucket[0].NodeID

	newcomer := mkContactInBucket(t, self, 255)
	tbl.AddOrUpdate(newcomer)

	waitFor(t, func() bool {
		for _, c := range tbl.AllContacts() {
			if c.NodeID == newcomer.NodeID {
				return true
			}
		}
		return false
	})

	for _, c := range tbl.AllContacts() {
		if c.NodeID == lruID {
			t.Fatal("dead LRU not removed")
		}
	}
}

// waitFor опрашивает cond до 2 секунд, затем валит тест.
// Используется для проверки асинхронных изменений в Table.AddOrUpdate.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within 2s")
}
