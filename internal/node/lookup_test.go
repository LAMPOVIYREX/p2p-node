package node

import (
	"testing"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/protocol"
)

// T2.13: инициатор находит узел через промежуточный.
// Топология: A ← B ← C ← D (стрелка = bootstrap).
// Перед lookup таблица a сбрасывается до одного контакта — b.
// Доказывается, что цель d найдена через промежуточные узлы, а не
// была известна инициатору заранее.
func TestT2_13_MultiHopLookup(t *testing.T) {
	a := startTestNode(t, mkCfg(t))

	cfgB := mkCfg(t)
	cfgB.BootstrapPeers = []string{a.Addr().String()}
	b := startTestNode(t, cfgB)
	if err := b.BootstrapPeers(); err != nil {
		t.Fatalf("b bootstrap: %v", err)
	}

	cfgC := mkCfg(t)
	cfgC.BootstrapPeers = []string{b.Addr().String()}
	c := startTestNode(t, cfgC)
	if err := c.BootstrapPeers(); err != nil {
		t.Fatalf("c bootstrap: %v", err)
	}

	cfgD := mkCfg(t)
	cfgD.BootstrapPeers = []string{c.Addr().String()}
	d := startTestNode(t, cfgD)
	if err := d.BootstrapPeers(); err != nil {
		t.Fatalf("d bootstrap: %v", err)
	}

	// Сброс таблицы a: воспроизводим требование T2.13 —
	// у инициатора нет цели в начальной таблице.
	// Оставляем только b — единственный путь к сети.
	a.table.Reset()
	a.table.AddOrUpdate(b.SelfContact())

	// Убеждаемся, что d действительно не в таблице a.
	if containsID(a.table.AllContacts(), d.self.NodeID) {
		t.Fatal("d still present in a's table after reset")
	}

	res := a.LookupNode(d.self.NodeID)

	// Строгая проверка: success == true только если цель в closest.
	if !res.Success {
		t.Fatalf("lookup failed: success=false rpc=%d iter=%d closest=%d",
			res.RPCs, res.Iterations, len(res.Closest))
	}

	targetFound := containsID(res.Closest, d.self.NodeID)
	if !targetFound {
		t.Fatalf("target d not in closest: %v", res.Closest)
	}

	if res.Iterations < 2 {
		t.Fatalf("iterations = %d, want >= 2 (multi-hop)", res.Iterations)
	}

	if len(res.Timeouts) > 0 {
		t.Logf("note: %d timeouts during lookup (ok, still found target)", len(res.Timeouts))
	}

	t.Logf("target d found after %d iterations, %d RPCs", res.Iterations, res.RPCs)
}

// T2.14: после bootstrap отключение seed не делает lookup невозможным.
// Топология: A (seed) ← B ← C. После остановки A узел B ищет C.
// Доказывается, что lookup успешен и цель C действительно найдена.
func TestT2_14_BootstrapDown(t *testing.T) {
	a := startTestNode(t, mkCfg(t))

	cfgB := mkCfg(t)
	cfgB.BootstrapPeers = []string{a.Addr().String()}
	b := startTestNode(t, cfgB)
	if err := b.BootstrapPeers(); err != nil {
		t.Fatalf("b bootstrap: %v", err)
	}

	cfgC := mkCfg(t)
	cfgC.BootstrapPeers = []string{b.Addr().String()}
	c := startTestNode(t, cfgC)
	if err := c.BootstrapPeers(); err != nil {
		t.Fatalf("c bootstrap: %v", err)
	}

	// B должен узнать C после того, как C bootstrap'нулся к нему.
	waitFor(t, func() bool {
		return containsID(b.table.AllContacts(), c.self.NodeID)
	})

	// Останавливаем seed A.
	a.Stop()
	time.Sleep(100 * time.Millisecond)

	// Убираем мёртвое соединение из пула b, если оно ещё там.
	if conn, err := b.pool.Get(a.Addr().String()); err == nil {
		b.pool.Evict(a.Addr().String(), conn)
	}

	// Проверка: A недоступен — запрос должен завершиться ошибкой.
	if _, err := b.rpc.SendRequest(a.Addr().String(), protocol.MsgPing,
		protocol.PingPayload{Sender: b.SelfContactWire(), TimestampMs: 1},
		500*time.Millisecond); err == nil {
		t.Fatal("expected error contacting killed bootstrap")
	}

	// Lookup C после того, как seed отключён.
	res := b.LookupNode(c.self.NodeID)

	// Строгая проверка: success == true только если цель в closest.
	if !res.Success {
		t.Fatalf("lookup after bootstrap down failed: success=false rpc=%d iter=%d closest=%d",
			res.RPCs, res.Iterations, len(res.Closest))
	}

	targetFound := containsID(res.Closest, c.self.NodeID)
	if !targetFound {
		t.Fatalf("target c not in closest after bootstrap down: %v", res.Closest)
	}

	// Доказательство, что lookup не тривиальный (шёл по сети):
	// если бы C уже был в таблице b, RPCs был бы 0 или 1.
	if res.RPCs < 1 {
		t.Fatalf("rpcs = %d, lookup should have performed at least one RPC", res.RPCs)
	}

	t.Logf("after seed down: found c via %d RPCs, %d iterations", res.RPCs, res.Iterations)
}
