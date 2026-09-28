package node

import (
	"encoding/hex"
	"sort"
	"testing"

	"github.com/LAMPOVIYREX/p2p-node/internal/protocol"
	"github.com/LAMPOVIYREX/p2p-node/internal/routing"
	"github.com/LAMPOVIYREX/p2p-node/internal/transport"
)

// T2.10: PING/PONG между двумя узлами, проверка request_id и
// обновления таблицы по требованию 19.3.
func TestT2_10_PingPong(t *testing.T) {
	a := startTestNode(t, mkCfg(t))
	b := startTestNode(t, mkCfg(t))

	// b пингует a — используется обёртка узла, которая по 19.3
	// обновляет таблицу b контактом a.
	c, err := b.pingPeer(a.Addr().String())
	if err != nil {
		t.Fatalf("pingPeer: %v", err)
	}
	if c.NodeID != a.self.NodeID {
		t.Fatalf("responder node_id = %x, want %x",
			c.NodeID[:4], a.self.NodeID[:4])
	}
	if c.Host != "127.0.0.1" || c.Port == 0 {
		t.Fatalf("bad contact addr: %s:%d", c.Host, c.Port)
	}

	// Таблица b должна содержать a сразу после успешного PONG.
	found := false
	for _, ct := range b.table.AllContacts() {
		if ct.NodeID == a.self.NodeID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("contact not added to b.table after PONG")
	}

	// Симметрично: a должен был получить b, потому что a обработал PING
	// через HandlePing, который вызывает table.AddOrUpdate.
	found = false
	for _, ct := range a.table.AllContacts() {
		if ct.NodeID == b.self.NodeID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("contact not added to a.table after handling PING")
	}
}

// T2.11: FIND_NODE_RESPONSE содержит не более K уникальных контактов,
// отсортированных по возрастанию XOR-расстояния до target.
func TestT2_11_FindNodeSorting(t *testing.T) {
	a := startTestNode(t, mkCfg(t))

	// Заполняем таблицу a тремя контактами; все поместятся в bucket
	// (KBucketSize=4), потому что мы добавляем их последовательно.
	for i := 0; i < 3; i++ {
		a.table.AddOrUpdate(mkContact(t))
	}

	// Целевая точка — произвольные 32 байта.
	var target routing.NodeID
	for i := range target {
		target[i] = byte(i)
	}

	// Sender — 4-й контакт; он тоже попадёт в таблицу при обработке запроса.
	sender := mkContact(t)

	req := protocol.FindNodeRequestPayload{
		Sender:       contactToWire(sender),
		TargetNodeID: hex.EncodeToString(target[:]),
	}
	payload, err := protocol.EncodePayload(req)
	if err != nil {
		t.Fatal(err)
	}
	frame := &transport.Frame{
		Version:   transport.ProtocolVersion,
		Type:      protocol.MsgFindNodeRequest,
		RequestID: [16]byte{1, 2, 3, 4},
		Payload:   payload,
	}
	resp := a.HandleFindNode(nil, frame)
	if resp == nil {
		t.Fatal("nil response")
	}
	if resp.Type != protocol.MsgFindNodeResponse {
		t.Fatalf("resp type = 0x%02x", resp.Type)
	}
	var out protocol.FindNodeResponsePayload
	if err := protocol.DecodePayload(resp.Payload, &out); err != nil {
		t.Fatal(err)
	}

	if len(out.Contacts) > a.cfg.KBucketSize {
		t.Fatalf("too many contacts: %d > %d", len(out.Contacts), a.cfg.KBucketSize)
	}

	// Ожидаем: отсортированные по XOR до target первые K контактов из таблицы.
	expected := append([]*routing.Contact{}, a.table.AllContacts()...)
	sort.Slice(expected, func(i, j int) bool {
		di := routing.Xor(expected[i].NodeID, target)
		dj := routing.Xor(expected[j].NodeID, target)
		return routing.CompareDist(di, dj) < 0
	})
	if len(expected) > a.cfg.KBucketSize {
		expected = expected[:a.cfg.KBucketSize]
	}

	if len(out.Contacts) != len(expected) {
		t.Fatalf("returned %d, expected %d", len(out.Contacts), len(expected))
	}
	for i := range out.Contacts {
		got, err := contactFromWire(out.Contacts[i])
		if err != nil {
			t.Fatalf("contact %d: %v", i, err)
		}
		if got.NodeID != expected[i].NodeID {
			t.Fatalf("order mismatch at %d: got %x want %x",
				i, got.NodeID[:4], expected[i].NodeID[:4])
		}
	}

	// Sender должен быть добавлен в таблицу.
	found := false
	for _, c := range a.table.AllContacts() {
		if c.NodeID == sender.NodeID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("sender not added to table")
	}
}
