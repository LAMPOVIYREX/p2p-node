package node

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/dht"
	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
	"github.com/LAMPOVIYREX/p2p-node/internal/protocol"
	"github.com/LAMPOVIYREX/p2p-node/internal/routing"
	"github.com/LAMPOVIYREX/p2p-node/internal/rpc"
	"github.com/LAMPOVIYREX/p2p-node/internal/transport"
)

func (n *Node) HandlePing(ctx *rpc.ConnContext, f *transport.Frame) *transport.Frame {
	var p protocol.PingPayload
	if err := protocol.DecodePayload(f.Payload, &p); err != nil {
		// + лог ошибки декодирования payload
		n.logger.Warn("ping bad payload",
			"rid", logging.ShortBytes(f.RequestID[:]),
			"err", err.Error(),
		)
		return n.errorFrame(f.RequestID, "bad_payload", err.Error())
	}
	sender, err := contactFromWire(p.Sender)
	if err != nil {
		// + лог ошибки контакта отправителя
		n.logger.Warn("ping bad contact",
			"rid", logging.ShortBytes(f.RequestID[:]),
			"err", err.Error(),
		)
		return n.errorFrame(f.RequestID, "bad_contact", err.Error())
	}
	n.table.AddOrUpdate(sender)

	// + лог успешной обработки PING на DEBUG
	now := uint64(time.Now().UnixMilli())
	n.logger.Debug("ping handled",
		"from", logging.ShortBytes(sender.NodeID[:]),
		"rtt_hint_ms", int64(now)-int64(p.TimestampMs),
	)

	return n.buildFrame(protocol.MsgPong, f.RequestID, protocol.PongPayload{
		Responder:            n.SelfContactWire(),
		PingTimestampMs:      p.TimestampMs,
		ResponderTimestampMs: now,
	})
}

func (n *Node) HandleFindNode(ctx *rpc.ConnContext, f *transport.Frame) *transport.Frame {
	var req protocol.FindNodeRequestPayload
	if err := protocol.DecodePayload(f.Payload, &req); err != nil {
		// + лог ошибки декодирования
		n.logger.Warn("find_node bad payload",
			"rid", logging.ShortBytes(f.RequestID[:]),
			"err", err.Error(),
		)
		return n.errorFrame(f.RequestID, "bad_payload", err.Error())
	}
	sender, err := contactFromWire(req.Sender)
	if err != nil {
		// + лог ошибки контакта отправителя
		n.logger.Warn("find_node bad contact",
			"rid", logging.ShortBytes(f.RequestID[:]),
			"err", err.Error(),
		)
		return n.errorFrame(f.RequestID, "bad_contact", err.Error())
	}
	n.table.AddOrUpdate(sender)

	target, err := parseNodeIDHex(req.TargetNodeID)
	if err != nil {
		// + лог ошибки парсинга target
		n.logger.Warn("find_node bad target",
			"rid", logging.ShortBytes(f.RequestID[:]),
			"raw", req.TargetNodeID,
			"err", err.Error(),
		)
		return n.errorFrame(f.RequestID, "bad_target", err.Error())
	}

	closest := n.table.FindClosest(target, n.cfg.KBucketSize)
	wires := make([]protocol.ContactWire, 0, len(closest))
	for _, c := range closest {
		wires = append(wires, contactToWire(c))
	}

	// + лог успешной обработки FIND_NODE
	n.logger.Debug("find_node handled",
		"from", logging.ShortBytes(sender.NodeID[:]),
		"target", logging.ShortBytes(target[:]),
		"returned", len(wires),
	)

	return n.buildFrame(protocol.MsgFindNodeResponse, f.RequestID, protocol.FindNodeResponsePayload{
		Responder:    n.SelfContactWire(),
		TargetNodeID: req.TargetNodeID,
		Contacts:     wires,
	})
}

// HandleStore принимает подписанную запись и сохраняет её локально.
func (n *Node) HandleStore(ctx *rpc.ConnContext, f *transport.Frame) *transport.Frame {
	var req protocol.StoreRequestPayload
	if err := protocol.DecodePayload(f.Payload, &req); err != nil {
		return n.errorFrame(f.RequestID, "bad_payload", err.Error())
	}

	sender, err := contactFromWire(req.Sender)
	if err == nil {
		n.table.AddOrUpdate(sender)
	}

	rec, err := dht.NodeRecordFromWire(req.Record)
	if err != nil {
		return n.buildFrame(protocol.MsgStoreResponse, f.RequestID, protocol.StoreResponsePayload{
			Success: false, Error: "bad_record_format",
		})
	}

	if err := rec.Validate(time.Now()); err != nil {
		n.logger.Debug("store: record validation failed", "err", err.Error())
		return n.buildFrame(protocol.MsgStoreResponse, f.RequestID, protocol.StoreResponsePayload{
			Success: false, Error: "invalid_record",
		})
	}

	if err := n.storage.Put(rec); err != nil {
		return n.buildFrame(protocol.MsgStoreResponse, f.RequestID, protocol.StoreResponsePayload{
			Success: false, Error: "storage_rejected",
		})
	}

	keyBytes := rec.DHTKey()
	n.logger.Debug("stored record", "key", hex.EncodeToString(keyBytes[:]))
	return n.buildFrame(protocol.MsgStoreResponse, f.RequestID, protocol.StoreResponsePayload{Success: true})
}

// HandleFindValue ищет запись локально, а при отсутствии — возвращает
// ближайших известных узлов (fallback на логику FIND_NODE).
func (n *Node) HandleFindValue(ctx *rpc.ConnContext, f *transport.Frame) *transport.Frame {
	var req protocol.FindValueRequestPayload
	if err := protocol.DecodePayload(f.Payload, &req); err != nil {
		return n.errorFrame(f.RequestID, "bad_payload", err.Error())
	}

	sender, err := contactFromWire(req.Sender)
	if err == nil {
		n.table.AddOrUpdate(sender)
	}

	var target [32]byte
	b, err := hex.DecodeString(req.Key)
	if err != nil || len(b) != 32 {
		return n.errorFrame(f.RequestID, "bad_key", "key must be 32-byte hex")
	}
	copy(target[:], b)

	// Ищем локально
	if rec, ok := n.storage.Get(target); ok {
		n.logger.Debug("find_value: found locally", "key", req.Key)
		wire := rec.ToWire()
		return n.buildFrame(protocol.MsgFindValueResponse, f.RequestID, protocol.FindValueResponsePayload{
			Responder: n.SelfContactWire(),
			Key:       req.Key,
			Record:    &wire,
			Contacts:  nil,
		})
	}

	// Fallback: возвращаем ближайших соседей.
	// Для этого используем NodeID, сформированный из target, чтобы
	// отсортировать контакты по XOR-расстоянию до ключа.
	var targetNodeID routing.NodeID
	copy(targetNodeID[:], target[:])
	closest := n.table.FindClosest(targetNodeID, n.cfg.KBucketSize)
	wires := make([]protocol.ContactWire, 0, len(closest))
	for _, c := range closest {
		wires = append(wires, contactToWire(c))
	}

	n.logger.Debug("find_value: not found, returning closest", "key", req.Key, "n", len(wires))
	return n.buildFrame(protocol.MsgFindValueResponse, f.RequestID, protocol.FindValueResponsePayload{
		Responder: n.SelfContactWire(),
		Key:       req.Key,
		Record:    nil,
		Contacts:  wires,
	})
}

func (n *Node) buildFrame(t uint8, rid [16]byte, payload any) *transport.Frame {
	b, err := protocol.EncodePayload(payload)
	if err != nil {
		return nil
	}
	return &transport.Frame{
		Version:   uint8(n.cfg.ProtocolVersion),
		Type:      t,
		RequestID: rid,
		Payload:   b,
	}
}

func (n *Node) errorFrame(rid [16]byte, code, msg string) *transport.Frame {
	return n.buildFrame(protocol.MsgError, rid, protocol.ErrorPayload{Code: code, Message: msg})
}

func (n *Node) pingPeer(addr string) (*routing.Contact, error) {
	resp, err := n.rpc.SendRequest(addr, protocol.MsgPing, protocol.PingPayload{
		Sender:      n.SelfContactWire(),
		TimestampMs: uint64(time.Now().UnixMilli()),
	}, n.cfg.PingTimeout())
	if err != nil {
		return nil, err
	}
	if resp.Type != protocol.MsgPong {
		return nil, fmt.Errorf("unexpected response type 0x%02x", resp.Type)
	}
	var p protocol.PongPayload
	if err := protocol.DecodePayload(resp.Payload, &p); err != nil {
		return nil, err
	}
	c, err := contactFromWire(p.Responder)
	if err != nil {
		return nil, err
	}
	n.table.AddOrUpdate(c)
	return c, nil
}

func (n *Node) pingContact(c *routing.Contact) bool {
	resp, err := n.rpc.SendRequest(c.Addr(), protocol.MsgPing, protocol.PingPayload{
		Sender:      n.SelfContactWire(),
		TimestampMs: uint64(time.Now().UnixMilli()),
	}, n.cfg.PingTimeout())
	if err != nil {
		return false
	}
	if resp.Type != protocol.MsgPong {
		return false
	}
	var p protocol.PongPayload
	if err := protocol.DecodePayload(resp.Payload, &p); err != nil {
		return false
	}
	responder, err := contactFromWire(p.Responder)
	if err != nil {
		return false
	}
	return responder.NodeID == c.NodeID
}

func contactFromWire(w protocol.ContactWire) (*routing.Contact, error) {
	id, err := parseNodeIDHex(w.NodeID)
	if err != nil {
		return nil, err
	}
	pub, err := base64.StdEncoding.DecodeString(w.IdentityPublicKey)
	if err != nil {
		return nil, err
	}
	c := &routing.Contact{
		NodeID:            id,
		IdentityAlgorithm: w.IdentityAlgorithm,
		IdentityPublicKey: pub,
		Host:              w.Host,
		Port:              w.Port,
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func contactToWire(c *routing.Contact) protocol.ContactWire {
	return protocol.ContactWire{
		NodeID:            hex.EncodeToString(c.NodeID[:]),
		IdentityAlgorithm: c.IdentityAlgorithm,
		IdentityPublicKey: base64.StdEncoding.EncodeToString(c.IdentityPublicKey),
		Host:              c.Host,
		Port:              c.Port,
	}
}

func parseNodeIDHex(s string) (routing.NodeID, error) {
	var id routing.NodeID
	b, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}
	if len(b) != 32 {
		return id, errors.New("node id must be 32 bytes")
	}
	copy(id[:], b)
	return id, nil
}

var _ = fmt.Sprintf
