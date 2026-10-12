package transport

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
)

type ConnHandler func(*Conn)

type Server struct {
	listener        net.Listener
	handler         ConnHandler
	logger          *slog.Logger
	maxPayload      uint32
	expectedVersion uint8
}

func NewServer(
	host string,
	port int,
	logger *slog.Logger,
	maxPayload uint32,
	expectedVersion uint8,
	onConn ConnHandler,
) (*Server, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = logging.Discard()
	}
	return &Server{
		listener:        ln,
		handler:         onConn,
		logger:          logger,
		maxPayload:      maxPayload,
		expectedVersion: expectedVersion,
	}, nil
}

func (s *Server) Addr() net.Addr { return s.listener.Addr() }

func (s *Server) Serve() {
	for {
		nc, err := s.listener.Accept()
		if err != nil {
			s.logger.Info("accept loop stopped", "err", err.Error())
			return
		}
		s.logger.Debug("incoming connection", "peer", nc.RemoteAddr().String())
		go s.handler(NewConn(nc, s.logger, s.maxPayload, s.expectedVersion))
	}
}

func (s *Server) Close() error { return s.listener.Close() }