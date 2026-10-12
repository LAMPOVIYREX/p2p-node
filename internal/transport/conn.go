package transport

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
)

type Conn struct {
	nc              net.Conn
	mu              sync.Mutex
	logger          *slog.Logger
	peer            string
	maxPayload      uint32
	expectedVersion uint8
}

func NewConn(nc net.Conn, logger *slog.Logger, maxPayload uint32, expectedVersion uint8) *Conn {
	peer := nc.RemoteAddr().String()
	if logger == nil {
		logger = logging.Discard()
	}
	return &Conn{
		nc:              nc,
		logger:          logger.With("peer", peer),
		peer:            peer,
		maxPayload:      maxPayload,
		expectedVersion: expectedVersion,
	}
}

func (c *Conn) WriteFrame(f *Frame, timeout time.Duration) error {
	if uint32(len(f.Payload)) > c.maxPayload {
		return fmt.Errorf("%w: len=%d max=%d", ErrPayloadTooLarge, len(f.Payload), c.maxPayload)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.nc.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	if _, err := c.nc.Write(Encode(f)); err != nil {
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
	f, err := Decode(c.nc, c.maxPayload, c.expectedVersion)
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