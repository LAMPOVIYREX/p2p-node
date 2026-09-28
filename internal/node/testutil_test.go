package node

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/identity"
	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
	"github.com/LAMPOVIYREX/p2p-node/internal/routing"
)

// mkCfg создаёт минимальный Config для тестов.
func mkCfg(t *testing.T) *Config {
	t.Helper()
	return &Config{
		NodeStateDir:     t.TempDir(),
		ListenHost:       "127.0.0.1",
		ListenPort:       0, // ОС выдаст свободный
		AdvertiseHost:    "127.0.0.1",
		NodeIDBits:       256,
		KBucketSize:      4,
		Alpha:            3,
		ConnectTimeoutMs: 500,
		ReadTimeoutMs:    1000,
		PingTimeoutMs:    1000,
		MaxFramePayload:  65536,
		ProtocolVersion:  1,
		LogLevel:         "WARN",
	}
}

// startTestNode поднимает Node в фоне и регистрирует t.Cleanup.
func startTestNode(t *testing.T, cfg *Config) *Node {
	t.Helper()
	n, err := New(cfg, logging.New("WARN"))
	if err != nil {
		t.Fatalf("node.New: %v", err)
	}
	if err := n.Start(); err != nil {
		t.Fatalf("node.Start: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		n.Wait(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Log("node.Wait did not return in 2s")
		}
	})
	return n
}

// waitFor опрашивает cond до 3 секунд, затем валит тест.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within 3s")
}

// mkContact создаёт валидный Contact со случайной Ed25519-парой.
func mkContact(t *testing.T) *routing.Contact {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &routing.Contact{
		NodeID:            identity.DeriveNodeID(pub),
		IdentityAlgorithm: "ed25519",
		IdentityPublicKey: pub,
		Host:              "127.0.0.1",
		Port:              9999,
	}
}