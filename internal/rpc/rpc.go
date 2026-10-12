package rpc

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
	"github.com/LAMPOVIYREX/p2p-node/internal/protocol"
	"github.com/LAMPOVIYREX/p2p-node/internal/transport"
)

var ErrRPCTimeout = errors.New("rpc timeout")

type pending struct {
	ch chan *transport.Frame
}

type Client struct {
	pool    *transport.Pool
	logger  *slog.Logger
	version uint8
	mu      sync.Mutex
	waiters map[[16]byte]*pending
}

func NewClient(pool *transport.Pool, logger *slog.Logger, version uint8) *Client {
	if logger == nil {
		logger = logging.Discard()
	}
	return &Client{
		pool:    pool,
		logger:  logger,
		version: version,
		waiters: make(map[[16]byte]*pending),
	}
}

func (c *Client) SendRequest(addr string, msgType uint8, payload any, timeout time.Duration) (*transport.Frame, error) {
	plBytes, err := protocol.EncodePayload(payload)
	if err != nil {
		return nil, err
	}
	id := uuid.New()
	var rid [16]byte
	copy(rid[:], id[:])

	f := &transport.Frame{
		Version:   c.version,
		Type:      msgType,
		RequestID: rid,
		Payload:   plBytes,
	}

	ch := make(chan *transport.Frame, 1)
	c.mu.Lock()
	c.waiters[rid] = &pending{ch: ch}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.waiters, rid)
		c.mu.Unlock()
	}()

	start := time.Now()
	c.logger.Debug("rpc send",
		"addr", addr,
		"type", msgType,
		"rid", logging.ShortBytes(rid[:]),
	)

	conn, err := c.pool.Get(addr)
	if err != nil {
		return nil, err
	}
	if err := conn.WriteFrame(f, 3*time.Second); err != nil {
		c.pool.Evict(addr)
		return nil, err
	}
	select {
	case resp := <-ch:
		c.logger.Debug("rpc response",
			"addr", addr,
			"type", msgType,
			"rid", logging.ShortBytes(rid[:]),
			"rtt_ms", time.Since(start).Milliseconds(),
		)
		return resp, nil
	case <-time.After(timeout):
		c.logger.Warn("rpc timeout",
			"addr", addr,
			"type", msgType,
			"rid", logging.ShortBytes(rid[:]),
			"timeout_ms", timeout.Milliseconds(),
		)
		return nil, ErrRPCTimeout
	}
}

func (c *Client) Deliver(f *transport.Frame) bool {
	c.mu.Lock()
	p, ok := c.waiters[f.RequestID]
	c.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case p.ch <- f:
	default:
	}
	return true
}
