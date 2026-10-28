package node

import (
	"testing"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/dht"
)

// T3.1: STORE + FIND_VALUE на 3+ узлах.
func TestT3_1_StoreFindValue(t *testing.T) {
	a := startTestNode(t, mkCfg(t))

	cfgB := mkCfg(t)
	cfgB.BootstrapPeers = []string{a.Addr().String()}
	b := startTestNode(t, cfgB)
	_ = b.BootstrapPeers()

	cfgC := mkCfg(t)
	cfgC.BootstrapPeers = []string{a.Addr().String()}
	c := startTestNode(t, cfgC)
	_ = c.BootstrapPeers()

	waitFor(t, func() bool { return len(a.table.AllContacts()) >= 2 })

	// A публикует запись
	rec := dht.NewNodeRecord(a.Identity(), []string{a.SelfContact().Addr()}, 1, 5*time.Minute, "alice")
	if err := rec.Sign(a.Identity().Private); err != nil {
		t.Fatal(err)
	}
	if err := a.PutRecord(rec); err != nil {
		t.Fatalf("PutRecord: %v", err)
	}

	// C ищет запись
	got, err := c.GetRecord(rec.DHTKey())
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if got.Alias != "alice" {
		t.Fatalf("wrong record: alias=%s", got.Alias)
	}
}

// T3.3: FIND_VALUE работает после отказа одного хранителя.
func TestT3_3_FindValueAfterFailover(t *testing.T) {
	a := startTestNode(t, mkCfg(t))
	b := startTestNode(t, mkCfg(t))
	c := startTestNode(t, mkCfg(t))

	// B и C подключаются к A
	b.cfg.BootstrapPeers = []string{a.Addr().String()}
	_ = b.BootstrapPeers()
	c.cfg.BootstrapPeers = []string{a.Addr().String()}
	_ = c.BootstrapPeers()

	waitFor(t, func() bool { return len(a.table.AllContacts()) >= 2 })

	// A публикует запись
	rec := dht.NewNodeRecord(a.Identity(), []string{a.SelfContact().Addr()}, 1, 5*time.Minute, "bob")
	_ = rec.Sign(a.Identity().Private)
	if err := a.PutRecord(rec); err != nil {
		t.Fatalf("PutRecord: %v", err)
	}

	// Останавливаем одного хранителя (b)
	b.Stop()
	time.Sleep(100 * time.Millisecond)

	// C всё равно должен найти запись через A
	got, err := c.GetRecord(rec.DHTKey())
	if err != nil {
		t.Fatalf("GetRecord after failover: %v", err)
	}
	if got.Alias != "bob" {
		t.Fatalf("wrong record: alias=%s", got.Alias)
	}
}
