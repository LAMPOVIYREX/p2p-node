package node

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// HandleSignals вешает обработчик SIGUSR1: по сигналу сохраняет дамп
// таблицы в state/<node>/table.json. Блокируется до отмены ctx.
// Запускать в отдельной горутине.
func (n *Node) HandleSignals(ctx context.Context) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGUSR1)
	defer signal.Stop(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			path, err := n.SaveTableDump()
			if err != nil {
				n.logger.Warn("table dump failed", "err", err.Error())
				continue
			}
			n.logger.Info("table dumped", "path", path, "size", len(n.table.AllContacts()))
		}
	}
}
