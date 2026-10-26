package node

import (
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
	"github.com/LAMPOVIYREX/p2p-node/internal/protocol"
	"github.com/LAMPOVIYREX/p2p-node/internal/routing"
)

type LookupResult struct {
	LookupID   string
	Target     routing.NodeID
	Closest    []*routing.Contact
	RPCs       int
	Iterations int
	Timeouts   []routing.NodeID
	Duration   time.Duration
	Success    bool
}

func (n *Node) LookupNode(target routing.NodeID) *LookupResult {
	start := time.Now()
	lid := uuid.New().String()[:8]
	logger := n.logger.With("lookup_id", lid)
	logger.Info("lookup start", "target", logging.ShortBytes(target[:]))

	res := &LookupResult{LookupID: lid, Target: target}

	candidates := n.table.FindClosest(target, n.cfg.KBucketSize)
	queried := map[routing.NodeID]bool{}    // кому отправили запрос
	successful := map[routing.NodeID]bool{} // кто успешно ответил

	for {
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

		res.Iterations++
		logger.Debug("lookup iteration",
			"iter", res.Iterations,
			"candidates", len(candidates),
			"batch", len(batch),
			"successful_so_far", len(successful),
		)

		type reply struct {
			c        *routing.Contact
			contacts []*routing.Contact
			err      error
		}
		ch := make(chan reply, len(batch))
		for _, c := range batch {
			queried[c.NodeID] = true
			go func(c *routing.Contact) {
				cs, err := n.findNodeRPC(c, target)
				ch <- reply{c, cs, err}
			}(c)
		}

		for range batch {
			r := <-ch
			res.RPCs++
			if r.err != nil {
				res.Timeouts = append(res.Timeouts, r.c.NodeID)
				logger.Debug("rpc failed",
					"peer", logging.ShortBytes(r.c.NodeID[:]),
					"err", r.err.Error(),
				)
				continue // не добавляем в successful
			}

			successful[r.c.NodeID] = true

			newCount := 0
			for _, nc := range r.contacts {
				if !containsID(candidates, nc.NodeID) && nc.NodeID != n.self.NodeID {
					candidates = append(candidates, nc)
					newCount++
				}
			}
			logger.Debug("rpc ok",
				"peer", logging.ShortBytes(r.c.NodeID[:]),
				"returned", len(r.contacts),
				"new", newCount,
			)
			n.table.AddOrUpdate(r.c)
		}

		if lookupConverged(candidates, queried, successful, target, n.cfg.KBucketSize) {
			break
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return routing.CompareDist(
			routing.Xor(candidates[i].NodeID, target),
			routing.Xor(candidates[j].NodeID, target),
		) < 0
	})
	if len(candidates) > n.cfg.KBucketSize {
		candidates = candidates[:n.cfg.KBucketSize]
	}
	res.Closest = candidates
	res.Duration = time.Since(start)

	res.Success = false
	for _, c := range candidates {
		if c.NodeID == target {
			res.Success = true
			break
		}
	}

	closestIDs := make([]string, len(candidates))
	for i, c := range candidates {
		closestIDs[i] = logging.ShortBytes(c.NodeID[:])
	}
	logger.Info("lookup complete",
		"target", logging.ShortBytes(target[:]),
		"success", res.Success,
		"rpc", res.RPCs,
		"iterations", res.Iterations,
		"successful_nodes", len(successful),
		"timeouts", len(res.Timeouts),
		"duration_ms", res.Duration.Milliseconds(),
		"closest", closestIDs,
	)
	return res
}

// lookupConverged определяет условие остановки: K ближайших УСПЕШНО опрошенных
// узлов ближе, чем любой из оставшихся НЕопрошенных кандидатов.
// Узлы, не ответившие по тайм-ауту, НЕ учитываются в списке K ближайших.
func lookupConverged(
	candidates []*routing.Contact,
	queried map[routing.NodeID]bool,
	successful map[routing.NodeID]bool,
	target routing.NodeID,
	k int,
) bool {
	var successfulDists [][32]byte
	for _, c := range candidates {
		if successful[c.NodeID] {
			successfulDists = append(successfulDists, routing.Xor(c.NodeID, target))
		}
	}

	// Пока не получили успешный ответ от K узлов, завершение невозможно.
	if len(successfulDists) < k {
		return false
	}

	sort.Slice(successfulDists, func(i, j int) bool {
		return routing.CompareDist(successfulDists[i], successfulDists[j]) < 0
	})
	kth := successfulDists[k-1]

	for _, c := range candidates {
		if successful[c.NodeID] {
			continue // уже успешно опрошен
		}
		if queried[c.NodeID] {
			continue // опрошен, но неуспешно (таймаут) — не блокируем поиск
		}
		// Неопрошенный кандидат ближе, чем K-й успешный — надо опросить.
		if routing.CompareDist(routing.Xor(c.NodeID, target), kth) < 0 {
			return false
		}
	}
	return true
}

func (n *Node) findNodeRPC(c *routing.Contact, target routing.NodeID) ([]*routing.Contact, error) {
	req := protocol.FindNodeRequestPayload{
		Sender:       n.SelfContactWire(),
		TargetNodeID: hex.EncodeToString(target[:]),
	}
	resp, err := n.rpc.SendRequest(c.Addr(), protocol.MsgFindNodeRequest, req, n.cfg.ReadTimeout())
	if err != nil {
		return nil, err
	}
	if resp.Type != protocol.MsgFindNodeResponse {
		return nil, errors.New("unexpected response type")
	}
	var p protocol.FindNodeResponsePayload
	if err := protocol.DecodePayload(resp.Payload, &p); err != nil {
		return nil, err
	}

	if p.TargetNodeID != hex.EncodeToString(target[:]) {
		return nil, errors.New("target_node_id mismatch in response")
	}

	out := make([]*routing.Contact, 0, len(p.Contacts))
	seen := map[routing.NodeID]bool{}
	for _, w := range p.Contacts {
		c, err := contactFromWire(w)
		if err != nil {
			continue
		}
		if seen[c.NodeID] {
			continue
		}
		seen[c.NodeID] = true
		out = append(out, c)
	}
	return out, nil
}

func containsID(cs []*routing.Contact, id routing.NodeID) bool {
	for _, c := range cs {
		if c.NodeID == id {
			return true
		}
	}
	return false
}
