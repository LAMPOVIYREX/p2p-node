package node

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
    "time"

	"github.com/LAMPOVIYREX/p2p-node/internal/dht"
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
	storage    *dht.Storage
	listenPort int 
    stopRepublish chan struct{}
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

	maxPayload := uint32(cfg.MaxFramePayload)
	version := uint8(cfg.ProtocolVersion)

	pool := transport.NewPool(
		cfg.ConnectTimeout(),
		logging.WithComponent(logger, "transport"),
		maxPayload,
		version,
	)
	n := &Node{
		cfg:    cfg,
		self:   id,
		pool:   pool,
		logger: logger,
	}
	n.rpc = rpc.NewClient(pool, logging.WithComponent(logger, "rpc"), version)
	n.table = routing.New(
		id.NodeID, cfg.KBucketSize, n.pingContact,
		logging.WithComponent(logger, "routing"),
	)
	n.storage = dht.NewStorage()
	n.disp = rpc.NewDispatcher(map[uint8]rpc.HandlerFunc{
		protocol.MsgPing:             n.HandlePing,
		protocol.MsgFindNodeRequest:  n.HandleFindNode,
		protocol.MsgStoreRequest:     n.HandleStore,     // NEW
		protocol.MsgFindValueRequest: n.HandleFindValue, // NEW
	}, logging.WithComponent(logger, "rpc"))
	pool.SetOnNewConn(n.handleOutgoingConn)
	return n, nil
}

// Start открывает TCP-сервер и запускает accept-loop.
// Возвращается после того, как сервер готов принимать соединения.
// Если cfg.ListenPort == 0, ОС выдаёт свободный порт; реальный порт
// сохраняется в n.listenPort и используется в SelfContact.
func (n *Node) Start() error {
	srv, err := transport.NewServer(
		n.cfg.ListenHost,
		n.cfg.ListenPort,
		logging.WithComponent(n.logger, "transport"),
		uint32(n.cfg.MaxFramePayload),
		uint8(n.cfg.ProtocolVersion),
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
    n.stopRepublish = make(chan struct{})
    go n.republishLoop()
    
    return nil
}

func (n *Node) republishLoop() {
    ticker := time.NewTicker(2 * time.Minute) // для тестов и экспериментов
    defer ticker.Stop()
    for {
        select {
        case <-n.stopRepublish:
            return
        case <-ticker.C:
            n.republishOwnedRecords()
        }
    }
}

func (n *Node) republishOwnedRecords() {
    records := n.storage.GetOwnedRecords(n.self.NodeID)
    for _, rec := range records {
        // Если осталось меньше половины TTL — продлеваем и публикуем заново
        now := uint64(time.Now().UnixMilli())
        ttl := rec.ExpiresAt - rec.IssuedAt
        if rec.ExpiresAt-now < ttl/2 {
            newRec := dht.NewNodeRecord(n.self, rec.Addresses, rec.SequenceNumber+1, 5*time.Minute, rec.Alias)
            _ = newRec.Sign(n.self.Private)
            _ = n.PutRecord(newRec)
            n.logger.Info("republished record", "alias", rec.Alias)
        }
    }
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

// Identity возвращает identity узла (ключи и NodeID).
// Возвращает указатель — caller может использовать Private для подписи.
func (n *Node) Identity() *identity.Identity {
	return n.self
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

// Stop закрывает TCP-сервер и все исходящие соединения.
// Используется для штатной остановки узла и в интеграционных тестах.
// После Stop узел не принимает входящие соединения и не инициирует
// новые исходящие. NodeID и state_dir сохраняются — при следующем
// создании узла из того же каталога идентичность восстановится.
func (n *Node) Stop() {
    if n.stopRepublish != nil {
        close(n.stopRepublish)
    }
	if n.server != nil {
		_ = n.server.Close()
	}
	n.pool.CloseAll()
	n.storage.Close()
	n.logger.Debug("node stopped")
}

func (n *Node) handleIncomingConn(c *transport.Conn) {
	defer c.Close()
	defer n.logger.Debug("incoming connection closed", "peer", c.Peer())
	for {
		f, err := c.ReadFrame(n.cfg.ReadTimeout())
		if err != nil {
			if isReadTimeout(err) {
				continue
			}
			return
		}
		if n.rpc.Deliver(f) {
			continue
		}
		n.disp.Dispatch(&rpc.ConnContext{Conn: c}, f)
	}
}

func (n *Node) handleOutgoingConn(c *transport.Conn) {
	// Соединение принадлежит пулу: при выходе из readLoop
	// оно закрывается и удаляется из пула. Если пул уже создал
	// новое соединение по этому адресу — Evict его не тронет,
	// так как проверяется идентичность *Conn.
	defer func() {
		_ = c.Close()
		n.pool.Evict(c.Peer(), c)
		n.logger.Debug("outgoing connection closed", "peer", c.Peer())
	}()

	for {
		f, err := c.ReadFrame(n.cfg.ReadTimeout())
		if err != nil {
			if isReadTimeout(err) {
				continue
			}
			// net.ErrClosed — соединение закрыто снаружи (WriteFrame или Evict).
			// Любая другая ошибка — разрыв TCP.
			return
		}
		if n.rpc.Deliver(f) {
			continue
		}
		n.disp.Dispatch(&rpc.ConnContext{Conn: c}, f)
	}
}

func isReadTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

var _ = fmt.Sprintf

func (n *Node) Logger() *slog.Logger {
	return n.logger
}

// TableSnapshot возвращает текущий список контактов.
// Обёртка над routing.Table.AllContacts.
func (n *Node) TableSnapshot() []*routing.Contact {
	return n.table.AllContacts()
}
