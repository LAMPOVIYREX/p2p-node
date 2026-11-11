package transport

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/LAMPOVIYREX/p2p-node/internal/identity"
	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
	"github.com/LAMPOVIYREX/p2p-node/internal/session"
)

type ConnHandler func(*Conn)

type Server struct {
	listener        net.Listener
	handler         ConnHandler
	logger          *slog.Logger
	maxPayload      uint32
	expectedVersion uint8
	identity        *identity.Identity
}

func NewServer(
	host string,
	port int,
	logger *slog.Logger,
	maxPayload uint32,
	expectedVersion uint8,
	id *identity.Identity,
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
		identity:        id,
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
		peer := nc.RemoteAddr().String()
		s.logger.Debug("incoming connection, starting handshake", "peer", peer)

		go func(nc net.Conn) {
			sess, peerID, err := session.ServerHandshake(nc, s.identity)
			if err != nil {
				s.logger.Warn("handshake failed", "peer", peer, "err", err.Error())
				_ = nc.Close()
				return
			}
			s.logger.Debug("handshake ok (server)",
				"peer", peer,
				"peer_id", logging.ShortBytes(peerID[:]),
				"session_id", sess.ID,
			)
			secureConn := session.NewSecureConn(nc, sess)
			s.handler(NewConn(secureConn, s.logger, s.maxPayload, s.expectedVersion))
		}(nc)
	}
}

func (s *Server) Close() error { return s.listener.Close() }
