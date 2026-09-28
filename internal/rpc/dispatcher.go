package rpc

import (
	"log/slog"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
	"github.com/LAMPOVIYREX/p2p-node/internal/protocol"
	"github.com/LAMPOVIYREX/p2p-node/internal/transport"
)

type ConnContext struct {
	Conn *transport.Conn
}

type HandlerFunc func(ctx *ConnContext, f *transport.Frame) *transport.Frame

type Dispatcher struct {
	handlers map[uint8]HandlerFunc
	logger   *slog.Logger
}

func NewDispatcher(handlers map[uint8]HandlerFunc, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = logging.Discard()
	}
	return &Dispatcher{handlers: handlers, logger: logger}
}

func (d *Dispatcher) Dispatch(ctx *ConnContext, f *transport.Frame) {
	h, ok := d.handlers[f.Type]
	if !ok {
		d.logger.Debug("unknown message type",
			"type", f.Type,
			"rid", logging.ShortBytes(f.RequestID[:]),
		)
		d.sendError(ctx, f.RequestID, "unknown_type", "unknown message type")
		return
	}
	resp := h(ctx, f)
	if resp != nil {
		_ = ctx.Conn.WriteFrame(resp, 3*time.Second)
	}
}

func (d *Dispatcher) sendError(ctx *ConnContext, rid [16]byte, code, msg string) {
	payload, err := protocol.EncodePayload(protocol.ErrorPayload{Code: code, Message: msg})
	if err != nil {
		_ = ctx.Conn.Close()
		return
	}
	resp := &transport.Frame{
		Version:   transport.ProtocolVersion,
		Type:      protocol.MsgError,
		RequestID: rid,
		Payload:   payload,
	}
	// Пытаемся отправить ERROR, после чего закрываем соединение —
	// требование 10: нарушение протокола закрывает соединение.
	_ = ctx.Conn.WriteFrame(resp, 3*time.Second)
	_ = ctx.Conn.Close()
}
