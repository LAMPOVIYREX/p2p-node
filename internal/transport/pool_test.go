package transport

import (
	"net"
	"sync"
	"testing"
	"time"
)

func TestPoolGetSkipsClosed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				// Держим соединение открытым, пока клиент не закроет.
				buf := make([]byte, 1)
				for {
					if _, err := c.Read(buf); err != nil {
						c.Close()
						return
					}
				}
			}(nc)
		}
	}()

	pool := NewPool(500*time.Millisecond, nil, MaxFramePayload, ProtocolVersion)
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
		t.Fatal("expected same cached connection")
	}

	// Закрываем. Следующий Get должен вернуть новое соединение.
	if err := c1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !c1.IsClosed() {
		t.Fatal("IsClosed should be true after Close")
	}

	c3, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("third Get: %v", err)
	}
	if c3 == c1 {
		t.Fatal("pool returned closed connection")
	}
	if c3.IsClosed() {
		t.Fatal("new connection should not be closed")
	}
}

func TestConnCloseIdempotent(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		nc, err := ln.Accept()
		if err != nil {
			return
		}
		nc.Close()
	}()

	nc, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	c := NewConn(nc, nil, MaxFramePayload, ProtocolVersion)

	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close should be nil, got: %v", err)
	}
	if !c.IsClosed() {
		t.Fatal("IsClosed should be true")
	}
	if err := c.WriteFrame(&Frame{Version: ProtocolVersion}, time.Second); err == nil {
		t.Fatal("WriteFrame on closed conn should fail")
	}
}
