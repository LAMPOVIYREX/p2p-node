package rpc_test

import (
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/rpc"
	"github.com/LAMPOVIYREX/p2p-node/internal/transport"
)

// T1.10: ответ с чужим request_id не принимается как ответ на ожидающий запрос.
func TestT1_10_RequestIDCorrelation(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var calls int32
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go func(nc net.Conn) {
				c := transport.NewConn(nc, nil, transport.MaxFramePayload, transport.ProtocolVersion)
				for {
					f, err := c.ReadFrame(2 * time.Second)
					if err != nil {
						return
					}
					n := atomic.AddInt32(&calls, 1)
					rid := f.RequestID
					if n > 1 {
						// Подменяем request_id — ответ не должен быть принят.
						rid[0] ^= 0xFF
					}
					resp := &transport.Frame{
						Version:   transport.ProtocolVersion,
						Type:      0x02,
						RequestID: rid,
						Payload:   []byte("{}"),
					}
					_ = c.WriteFrame(resp, time.Second)
				}
			}(nc)
		}
	}()

	pool := transport.NewPool(
		500*time.Millisecond,
		nil,
		transport.MaxFramePayload,
		transport.ProtocolVersion,
	)
	cli := rpc.NewClient(pool, nil, transport.ProtocolVersion)

	// В Node чтение исходящего соединения делает handleOutgoingConn.
	// В тесте устанавливаем простой reader.
	pool.SetOnNewConn(func(c *transport.Conn) {
		for {
			f, err := c.ReadFrame(2 * time.Second)
			if err != nil {
				return
			}
			cli.Deliver(f)
		}
	})

	addr := ln.Addr().String()

	// Первый запрос — правильный request_id, должен успеть.
	if _, err := cli.SendRequest(addr, 0x01, map[string]any{"n": 1}, 800*time.Millisecond); err != nil {
		t.Fatalf("first request: %v", err)
	}

	// Второй запрос — подменённый request_id, ожидаем таймаут.
	_, err = cli.SendRequest(addr, 0x01, map[string]any{"n": 2}, 300*time.Millisecond)
	if !errors.Is(err, rpc.ErrRPCTimeout) {
		t.Fatalf("want ErrRPCTimeout, got %v", err)
	}

	if got := atomic.LoadInt32(&calls); got < 2 {
		t.Fatalf("server processed %d calls, want >= 2", got)
	}
}
