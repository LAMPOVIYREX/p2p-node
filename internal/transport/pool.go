package transport

import (
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/identity"
	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
	"github.com/LAMPOVIYREX/p2p-node/internal/session"
)

type Pool struct {
	mu              sync.Mutex
	conns           map[string]*Conn
	dialer          *net.Dialer
	logger          *slog.Logger
	onNew           func(*Conn)
	maxPayload      uint32
	expectedVersion uint8
	identity        *identity.Identity
}

func NewPool(
	connectTimeout time.Duration,
	logger *slog.Logger,
	maxPayload uint32,
	expectedVersion uint8,
	id *identity.Identity,
) *Pool {
	if logger == nil {
		logger = logging.Discard()
	}
	return &Pool{
		conns:           make(map[string]*Conn),
		dialer:          &net.Dialer{Timeout: connectTimeout},
		logger:          logger,
		maxPayload:      maxPayload,
		expectedVersion: expectedVersion,
		identity:        id,
	}
}

// SetOnNewConn ...
func (p *Pool) SetOnNewConn(fn func(*Conn)) {
	p.mu.Lock()
	p.onNew = fn
	p.mu.Unlock()
}

func (p *Pool) Get(addr string) (*Conn, error) {
	p.mu.Lock()
	if c, ok := p.conns[addr]; ok {
		if !c.IsClosed() {
			p.mu.Unlock()
			p.logger.Debug("pool hit", "addr", addr)
			return c, nil
		}
		delete(p.conns, addr)
	}
	onNew := p.onNew
	p.mu.Unlock()

	p.logger.Debug("dialing", "addr", addr)
	nc, err := p.dialer.Dial("tcp", addr)
	if err != nil {
		p.logger.Warn("dial failed", "addr", addr, "err", err.Error())
		return nil, err
	}

	p.logger.Debug("tcp connected, starting handshake", "addr", addr)
	sess, err := session.ClientHandshake(nc, p.identity)
	if err != nil {
		p.logger.Warn("handshake failed", "addr", addr, "err", err.Error())
		_ = nc.Close()
		return nil, err
	}
	p.logger.Debug("handshake ok (client)",
		"addr", addr,
		"peer_id", logging.ShortBytes(sess.PeerID[:]),
		"session_id", sess.ID,
	)

	secureConn := session.NewSecureConn(nc, sess)
	c := NewConn(secureConn, p.logger, p.maxPayload, p.expectedVersion)

	p.mu.Lock()
	if existing, ok := p.conns[addr]; ok {
		p.mu.Unlock()
		_ = c.Close()
		return existing, nil
	}
	p.conns[addr] = c
	p.mu.Unlock()

	if onNew != nil {
		go onNew(c)
	}
	return c, nil
}

func (p *Pool) Evict(addr string, c *Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.conns[addr]; ok && existing == c {
		_ = c.Close()
		delete(p.conns, addr)
		p.logger.Debug("connection evicted", "addr", addr)
	}
}

func (p *Pool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for addr, c := range p.conns {
		_ = c.Close()
		delete(p.conns, addr)
	}
}
