package node

import (
	"testing"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
)

// TestE2_1_PeerRestart проверяет, что при перезапуске узла с тем же
// state_dir:
//  1. NodeID не меняется (требование E2-1).
//  2. Узел снова присоединяется к сети через bootstrap.
//  3. Другие узлы снова видят его в таблице.
func TestE2_1_PeerRestart(t *testing.T) {
	// Seed-узел A.
	a := startTestNode(t, mkCfg(t))

	// Пир B со своим state_dir.
	stateDirB := t.TempDir()
	cfgB := mkCfg(t)
	cfgB.NodeStateDir = stateDirB
	cfgB.BootstrapPeers = []string{a.Addr().String()}

	// Первый запуск B.
	b1, err := New(cfgB, logging.New("WARN"))
	if err != nil {
		t.Fatalf("New b1: %v", err)
	}
	if err := b1.Start(); err != nil {
		t.Fatalf("Start b1: %v", err)
	}
	if err := b1.BootstrapPeers(); err != nil {
		t.Fatalf("Bootstrap b1: %v", err)
	}
	idB := b1.self.NodeID
	portB := b1.cfg.ListenPort

	// Останавливаем B1.
	b1.Stop()
	time.Sleep(100 * time.Millisecond)

	// Перезапускаем B2 из того же state_dir.
	b2, err := New(cfgB, logging.New("WARN"))
	if err != nil {
		t.Fatalf("New b2: %v", err)
	}
	t.Cleanup(func() { b2.Stop() })

	// 1. NodeID сохранился.
	if b2.self.NodeID != idB {
		t.Fatalf("NodeID changed after restart: %x -> %x", idB, b2.self.NodeID)
	}

	// 2. B2 снова присоединяется к сети.
	if err := b2.Start(); err != nil {
		t.Fatalf("Start b2: %v", err)
	}
	if err := b2.BootstrapPeers(); err != nil {
		t.Fatalf("Bootstrap b2: %v", err)
	}

	// 3. A видит B снова.
	waitFor(t, func() bool {
		for _, c := range a.table.AllContacts() {
			if c.NodeID == idB {
				return true
			}
		}
		return false
	})

	// 4. B2 видит A в своей таблице.
	waitFor(t, func() bool {
		for _, c := range b2.table.AllContacts() {
			if c.NodeID == a.self.NodeID {
				return true
			}
		}
		return false
	})

	t.Logf("NodeID preserved across restart: %x, port_before=%d, port_after=%d",
		idB, portB, b2.cfg.ListenPort)
}
