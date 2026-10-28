package node

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/dht"
	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
	"github.com/LAMPOVIYREX/p2p-node/internal/protocol"
	"github.com/LAMPOVIYREX/p2p-node/internal/routing"
)

var (
	ErrNotEnoughReplicas = errors.New("not enough replicas confirmed store")
	ErrRecordNotFound    = errors.New("record not found in DHT")
)

// PutRecord размещает подписанную запись на R=3 ближайших к ключу узлах.
// Требуется минимум 2 подтверждения. Локально тоже сохраняет.
func (n *Node) PutRecord(rec *dht.NodeRecord) error {
	if err := rec.Validate(time.Now()); err != nil {
		return fmt.Errorf("invalid record: %w", err)
	}

	keyBytes := rec.DHTKey()
	var key routing.NodeID
	copy(key[:], keyBytes[:])

	closest := n.table.FindClosest(key, n.cfg.KBucketSize)
	if len(closest) == 0 {
		// Только локально
		return n.storage.Put(rec)
	}

	// Берём до 3 ближайших (без себя)
	replicas := closest
	if len(replicas) > 3 {
		replicas = replicas[:3]
	}

	stored := 0
	for _, c := range replicas {
		if err := n.storeRPC(c, rec); err != nil {
			n.logger.Debug("store rpc failed",
				"peer", logging.ShortBytes(c.NodeID[:]),
				"err", err.Error(),
			)
			continue
		}
		stored++
	}

	n.logger.Info("put record",
		"key", logging.ShortBytes(keyBytes[:]),
		"replicas_attempted", len(replicas),
		"replicas_ok", stored,
	)

	if stored < 2 && len(replicas) >= 2 {
		return ErrNotEnoughReplicas
	}

	// Сохраняем локально тоже (полезно для последующих lookup)
	_ = n.storage.Put(rec)
	return nil
}

// GetRecord выполняет итеративный поиск значения по DHT.
// Возвращает запись или ErrRecordNotFound.
func (n *Node) GetRecord(key [32]byte) (*dht.NodeRecord, error) {
	lid := fmt.Sprintf("get-%d", time.Now().UnixNano()%10000)
	logger := n.logger.With("lookup_id", lid)
	logger.Info("get record start", "key", logging.ShortBytes(key[:]))

	// Проверяем локально
	if rec, ok := n.storage.Get(key); ok {
		logger.Info("get record: found locally")
		return rec, nil
	}

	var target routing.NodeID
	copy(target[:], key[:])

	candidates := n.table.FindClosest(target, n.cfg.KBucketSize)
	queried := map[routing.NodeID]bool{}
	successful := map[routing.NodeID]bool{}

	for iter := 1; iter <= 10; iter++ {
		sort.Slice(candidates, func(i, j int) bool {
			return routing.CompareDist(
				routing.Xor(candidates[i].NodeID, target),
				routing.Xor(candidates[j].NodeID, target),
			) < 0
		})

		var batch []*routing.Contact
		for _, c := range candidates {
			if len(batch) >= n.cfg.Alpha {
				break
			}
			if queried[c.NodeID] || c.NodeID == n.self.NodeID {
				continue
			}
			batch = append(batch, c)
		}
		if len(batch) == 0 {
			break
		}

		logger.Debug("get iter", "iter", iter, "batch", len(batch))

		type reply struct {
			c        *routing.Contact
			rec      *dht.NodeRecord
			contacts []*routing.Contact
			err      error
		}
		ch := make(chan reply, len(batch))

		for _, c := range batch {
			queried[c.NodeID] = true
			go func(c *routing.Contact) {
				rec, cs, err := n.findValueRPC(c, key)
				ch <- reply{c, rec, cs, err}
			}(c)
		}

		for range batch {
			r := <-ch
			if r.err != nil {
				logger.Debug("get rpc failed",
					"peer", logging.ShortBytes(r.c.NodeID[:]),
					"err", r.err.Error(),
				)
				continue
			}
			successful[r.c.NodeID] = true
			if r.rec != nil {
				// Нашли!
				logger.Info("get record: found via network",
					"peer", logging.ShortBytes(r.c.NodeID[:]),
					"iter", iter,
				)
				// Кэшируем локально
				_ = n.storage.Put(r.rec)
				return r.rec, nil
			}

			// Записи нет — добавляем новых кандидатов
			for _, nc := range r.contacts {
				if !containsID(candidates, nc.NodeID) && nc.NodeID != n.self.NodeID {
					candidates = append(candidates, nc)
				}
			}
			n.table.AddOrUpdate(r.c)
		}

		if len(successful) >= n.cfg.KBucketSize {
			break
		}
	}

	logger.Info("get record: not found")
	return nil, ErrRecordNotFound
}

// storeRPC отправляет STORE одному узлу.
func (n *Node) storeRPC(c *routing.Contact, rec *dht.NodeRecord) error {
	req := protocol.StoreRequestPayload{
		Sender: n.SelfContactWire(),
		Record: rec.ToWire(),
	}
	resp, err := n.rpc.SendRequest(c.Addr(), protocol.MsgStoreRequest, req, n.cfg.ReadTimeout())
	if err != nil {
		return err
	}
	if resp.Type != protocol.MsgStoreResponse {
		return fmt.Errorf("unexpected response type 0x%02x", resp.Type)
	}
	var p protocol.StoreResponsePayload
	if err := protocol.DecodePayload(resp.Payload, &p); err != nil {
		return err
	}
	if !p.Success {
		return fmt.Errorf("store rejected: %s", p.Error)
	}
	return nil
}

// findValueRPC отправляет FIND_VALUE одному узлу.
func (n *Node) findValueRPC(c *routing.Contact, key [32]byte) (*dht.NodeRecord, []*routing.Contact, error) {
	req := protocol.FindValueRequestPayload{
		Sender: n.SelfContactWire(),
		Key:    fmt.Sprintf("%x", key[:]),
	}
	resp, err := n.rpc.SendRequest(c.Addr(), protocol.MsgFindValueRequest, req, n.cfg.ReadTimeout())
	if err != nil {
		return nil, nil, err
	}
	if resp.Type != protocol.MsgFindValueResponse {
		return nil, nil, fmt.Errorf("unexpected response type 0x%02x", resp.Type)
	}
	var p protocol.FindValueResponsePayload
	if err := protocol.DecodePayload(resp.Payload, &p); err != nil {
		return nil, nil, err
	}

	if p.Record != nil {
		rec, err := dht.NodeRecordFromWire(*p.Record)
		if err != nil {
			return nil, nil, err
		}
		if err := rec.Validate(time.Now()); err != nil {
			return nil, nil, err
		}
		return rec, nil, nil
	}

	// Записи нет — возвращаем ближайших
	var contacts []*routing.Contact
	seen := map[routing.NodeID]bool{}
	for _, w := range p.Contacts {
		ct, err := contactFromWire(w)
		if err != nil {
			continue
		}
		if seen[ct.NodeID] {
			continue
		}
		seen[ct.NodeID] = true
		contacts = append(contacts, ct)
	}
	return nil, contacts, nil
}
