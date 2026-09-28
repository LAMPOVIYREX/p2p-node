package node

import (
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
)

var ErrNoBootstrapReachable = errors.New("no bootstrap peer reachable")

func (n *Node) Bootstrap(peers []string) error {
	for _, addr := range peers {
		host, portStr, err := net.SplitHostPort(addr)
		if err != nil {
			n.logger.Debug("bootstrap peer skipped", "addr", addr, "reason", "bad addr")
			continue
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			n.logger.Debug("bootstrap peer skipped", "addr", addr, "reason", "bad port")
			continue
		}

		start := time.Now()
		peer, err := n.pingPeer(addr)
		if err != nil {
			n.logger.Warn("bootstrap ping failed", "addr", addr, "err", err.Error())
			continue
		}

		// Если адрес в PONG-контакте не совпадает с bootstrap-адресом
		// (например, из-за AdvertiseHost), принудительно выставляем
		// адрес из конфига — он надёжнее на этапах 1–2.
		peer.Host = host
		peer.Port = uint16(port)
		n.table.AddOrUpdate(peer)

		n.logger.Info("bootstrap ok",
			"peer", peer.Addr(),
			"peer_node_id", logging.ShortBytes(peer.NodeID[:]),
			"rtt_ms", time.Since(start).Milliseconds(),
		)

		res := n.LookupNode(n.self.NodeID)
		n.logger.Info("bootstrap self-lookup complete",
			"rpc", res.RPCs,
			"iterations", res.Iterations,
			"timeouts", len(res.Timeouts),
			"duration_ms", res.Duration.Milliseconds(),
			"table_size", len(n.table.AllContacts()),
		)

		// Требование 22.2 п.6: сохранить итоговое состояние таблицы
		// после bootstrap. Ошибка сохранения не критична — просто логируем.
		if path, err := n.SaveTableDump(); err != nil {
			n.logger.Warn("post-bootstrap table dump failed", "err", err.Error())
		} else {
			n.logger.Debug("post-bootstrap table saved", "path", path)
		}
		return nil
	}
	return ErrNoBootstrapReachable
}
