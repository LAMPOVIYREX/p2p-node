package node

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/LAMPOVIYREX/p2p-node/internal/identity"
	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
	"github.com/LAMPOVIYREX/p2p-node/internal/protocol"
	"github.com/LAMPOVIYREX/p2p-node/internal/routing"
	"github.com/LAMPOVIYREX/p2p-node/internal/rpc"
	"github.com/LAMPOVIYREX/p2p-node/internal/transport"
)

type Node struct {
	cfg        *Config
	self       *identity.Identity
	table      *routing.Table
	rpc        *rpc.Client
	pool       *transport.Pool
	server     *transport.Server
	disp       *rpc.Dispatcher
	logger     *slog.Logger
	listenPort int // реальный порт после Start (важно при cfg.ListenPort == 0)
}

func New(cfg *Config, logger *slog.Logger) (*Node, error) {
	id, err := identity.LoadOrCreate(cfg.NodeStateDir)
	if err != nil {
		return nil, err
	}
	logger = logger.With("node_id", logging.ShortBytes(id.NodeID[:]))
	logger.Info("identity loaded",
		"state_dir", cfg.NodeStateDir,
		"full_node_id", fmt.Sprintf("%x", id.NodeID),
	)

	pool := transport.NewPool(cfg.ConnectTimeout(), logging.WithComponent(logger, "transport"))
	n := &Node{
		cfg:    cfg,
		self:   id,
		pool:   pool,
		logger: logger,
	}
	n.rpc = rpc.NewClient(pool, logging.WithComponent(logger, "rpc"))
	n.table = routing.New(
		id.NodeID, cfg.KBucketSize, n.pingContact,
		logging.WithComponent(logger, "routing"),
	)
	n.disp = rpc.NewDispatcher(
		map[uint8]rpc.HandlerFunc{
			protocol.MsgPing:            n.HandlePing,
			protocol.MsgFindNodeRequest: n.HandleFindNode,
		},
		logging.WithComponent(logger, "rpc"),
	)
	pool.SetOnNewConn(n.handleOutgoingConn)
	return n, nil
}

// Start открывает TCP-сервер и запускает accept-loop.
// Возвращается после того, как сервер готов принимать соединения.
// Если cfg.ListenPort == 0, ОС выдаёт свободный порт; реальный порт
// сохраняется в n.listenPort и используется в SelfContact.
func (n *Node) Start() error {
	srv, err := transport.NewServer(
		n.cfg.ListenHost, n.cfg.ListenPort,
		logging.WithComponent(n.logger, "transport"),
		n.handleIncomingConn,
	)
	if err != nil {
		n.logger.Error("tcp listen failed",
			"host", n.cfg.ListenHost,
			"port", n.cfg.ListenPort,
			"err", err.Error(),
		)
		return err
	}
	n.server = srv
	if tcpAddr, ok := srv.Addr().(*net.TCPAddr); ok {
		n.listenPort = tcpAddr.Port
	}
	n.logger.Info("tcp server started", "addr", srv.Addr().String())
	go srv.Serve()
	return nil
}

// BootstrapPeers запускает присоединение, если в конфиге есть peers.
func (n *Node) BootstrapPeers() error {
	if len(n.cfg.BootstrapPeers) == 0 {
		return nil
	}
	n.logger.Info("starting bootstrap", "peers", n.cfg.BootstrapPeers)
	return n.Bootstrap(n.cfg.BootstrapPeers)
}

func (n *Node) Run(ctx context.Context) error {
	if err := n.Start(); err != nil {
		return err
	}
	go n.HandleSignals(ctx)
	if err := n.BootstrapPeers(); err != nil {
		n.logger.Warn("bootstrap failed", "err", err.Error())
	}
	n.Wait(ctx)
	return nil
}

// Wait блокируется до отмены ctx, затем закрывает сервер.
func (n *Node) Wait(ctx context.Context) {
	<-ctx.Done()
	n.logger.Info("shutting down", "table_size", len(n.table.AllContacts()))
	if path, err := n.SaveTableDump(); err != nil {
		n.logger.Warn("shutdown table dump failed", "err", err.Error())
	} else {
		n.logger.Debug("shutdown table saved", "path", path)
	}
	if n.server != nil {
		_ = n.server.Close()
	}
}

// Addr возвращает реальный адрес слушающего сокета.
func (n *Node) Addr() net.Addr {
	if n.server == nil {
		return nil
	}
	return n.server.Addr()
}

func (n *Node) ShortID() string {
	return fmt.Sprintf("%x", n.self.NodeID[:4])
}

func (n *Node) SelfContact() *routing.Contact {
	return &routing.Contact{
		NodeID:            n.self.NodeID,
		IdentityAlgorithm: "ed25519",
		IdentityPublicKey: n.self.Public,
		Host:              n.cfg.AdvertiseHost,
		Port:              uint16(n.listenPort),
	}
}

func (n *Node) SelfContactWire() protocol.ContactWire {
	return contactToWire(n.SelfContact())
}

func (n *Node) handleIncomingConn(c *transport.Conn) {
	defer c.Close()
	defer n.logger.Debug("incoming connection closed", "peer", c.RemoteAddr().String())
	for {
		f, err := c.ReadFrame(n.cfg.ReadTimeout())
		if err != nil {
			return
		}
		if n.rpc.Deliver(f) {
			continue
		}
		n.disp.Dispatch(&rpc.ConnContext{Conn: c}, f)
	}
}

func (n *Node) handleOutgoingConn(c *transport.Conn) {
	defer n.logger.Debug("outgoing connection closed", "peer", c.RemoteAddr().String())
	for {
		f, err := c.ReadFrame(n.cfg.ReadTimeout())
		if err != nil {
			return
		}
		if n.rpc.Deliver(f) {
			continue
		}
		n.disp.Dispatch(&rpc.ConnContext{Conn: c}, f)
	}
}

func (n *Node) Logger() *slog.Logger {
	return n.logger
}

// TableSnapshot возвращает текущий список контактов.
// Обёртка над routing.Table.AllContacts.
func (n *Node) TableSnapshot() []*routing.Contact {
	return n.table.AllContacts()
}
