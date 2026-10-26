package transport

import (
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
)

type Pool struct {
	mu              sync.Mutex
	conns           map[string]*Conn
	dialer          *net.Dialer
	logger          *slog.Logger
	onNew           func(*Conn)
	maxPayload      uint32
	expectedVersion uint8
}

func NewPool(
	connectTimeout time.Duration,
	logger *slog.Logger,
	maxPayload uint32,
	expectedVersion uint8,
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
	}
}

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
		// Соединение мертво — удаляем
		_ = c.Close()
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
	c := NewConn(nc, p.logger, p.maxPayload, p.expectedVersion)

	p.mu.Lock()
	if existing, ok := p.conns[addr]; ok {
		// Кто-то успел создать соединение раньше нас — используем его
		p.mu.Unlock()
		_ = c.Close()
		return existing, nil
	}
	p.conns[addr] = c
	p.mu.Unlock()

	p.logger.Debug("connection established", "addr", addr)
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

// CloseAll закрывает все соединения в пуле. Используется при штатной
// остановке узла и в тестах, где нужен чистый перезапуск.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for addr, c := range p.conns {
		_ = c.Close()
		delete(p.conns, addr)
	}
}
