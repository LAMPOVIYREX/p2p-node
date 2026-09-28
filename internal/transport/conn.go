package transport

import (
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
)

type Conn struct {
	nc     net.Conn
	mu     sync.Mutex
	logger *slog.Logger
	peer   string
}

func NewConn(nc net.Conn, logger *slog.Logger) *Conn {
	peer := nc.RemoteAddr().String()
	if logger == nil {
		logger = logging.Discard()
	}
	return &Conn{
		nc:     nc,
		logger: logger.With("peer", peer),
		peer:   peer,
	}
}

func (c *Conn) WriteFrame(f *Frame, timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.nc.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	_, err := c.nc.Write(Encode(f))
	if err != nil {
		c.logger.Debug("frame send failed",
			"type", f.Type,
			"rid", logging.ShortBytes(f.RequestID[:]),
			"err", err.Error(),
		)
		return err
	}
	c.logger.Debug("frame sent",
		"type", f.Type,
		"rid", logging.ShortBytes(f.RequestID[:]),
		"size", len(f.Payload),
	)
	return nil
}

func (c *Conn) ReadFrame(timeout time.Duration) (*Frame, error) {
	if err := c.nc.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	f, err := Decode(c.nc)
	if err != nil {
		if isTimeout(err) {
			c.logger.Debug("frame read timeout")
		} else {
			c.logger.Debug("frame read failed", "err", err.Error())
		}
		return nil, err
	}
	c.logger.Debug("frame received",
		"type", f.Type,
		"rid", logging.ShortBytes(f.RequestID[:]),
		"size", len(f.Payload),
	)
	return f, nil
}

func (c *Conn) Close() error         { return c.nc.Close() }
func (c *Conn) RemoteAddr() net.Addr { return c.nc.RemoteAddr() }

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
