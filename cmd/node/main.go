package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
	"github.com/LAMPOVIYREX/p2p-node/internal/node"
)

func main() {
	cfgPath := flag.String("config", "", "path to config yaml")
	flag.Parse()
	if *cfgPath == "" {
		log.Fatal("--config is required")
	}
	cfg, err := node.LoadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	logger := logging.New(cfg.LogLevel)
	n, err := node.New(cfg, logger)
	if err != nil {
		logger.Error("create node", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := n.Run(ctx); err != nil {
		logger.Error("run", "err", err)
		os.Exit(1)
	}
}
