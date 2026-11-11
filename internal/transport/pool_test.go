package transport

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"testing"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/identity"
	"github.com/LAMPOVIYREX/p2p-node/internal/session"
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

// TestPoolEvictDoesNotRemoveNewConn проверяет, что Evict для старого
// соединения не удаляет новое, созданное после него по тому же адресу.
func TestPoolEvictDoesNotRemoveNewConn(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srvID := mkTestIdentity(t)
	cliID := mkTestIdentity(t)

	// Мини-сервер, который принимает рукопожатие и держит соединение.
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go func(nc net.Conn) {
				_, _, err := session.ServerHandshake(nc, srvID)
				if err != nil {
					_ = nc.Close()
					return
				}
				buf := make([]byte, 1)
				for {
					if _, err := nc.Read(buf); err != nil {
						_ = nc.Close()
						return
					}
				}
			}(nc)
		}
	}()

	pool := NewPool(500*time.Millisecond, nil, MaxFramePayload, ProtocolVersion, cliID)
	defer pool.CloseAll()
	addr := ln.Addr().String()

	c1, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}

	pool.Evict(addr, c1)

	c2, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if c1 == c2 {
		t.Fatal("expected new connection after evict")
	}

	// Эвикт старого c1 не должен удалить c2.
	pool.Evict(addr, c1)

	c3, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("third Get: %v", err)
	}
	if c3 != c2 {
		t.Fatal("Evict of old conn removed the new active connection")
	}
}

// TestPoolGetReturnsCached проверяет, что повторный Get возвращает
// то же самое соединение из пула.
func TestPoolGetReturnsCached(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srvID := mkTestIdentity(t)
	cliID := mkTestIdentity(t)

	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go func(nc net.Conn) {
				_, _, _ = session.ServerHandshake(nc, srvID)
				buf := make([]byte, 1)
				for {
					if _, err := nc.Read(buf); err != nil {
						_ = nc.Close()
						return
					}
				}
			}(nc)
		}
	}()

	pool := NewPool(500*time.Millisecond, nil, MaxFramePayload, ProtocolVersion, cliID)
	defer pool.CloseAll()
	addr := ln.Addr().String()

	c1, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	c2, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if c1 != c2 {
		t.Fatal("pool did not return cached connection")
	}
}