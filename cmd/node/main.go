package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/LAMPOVIYREX/p2p-node/internal/logging"
	"github.com/LAMPOVIYREX/p2p-node/internal/node"
	"github.com/LAMPOVIYREX/p2p-node/internal/routing"
)

func main() {
	cfgPath := flag.String("config", "", "path to config yaml (daemon or lookup mode)")
	lookupHex := flag.String("lookup", "", "run single lookup for target NodeID (hex) and exit")
	dumpAll := flag.Bool("dump-tables", false, "aggregate all state/node-*/table.json and exit")
	stateDir := flag.String("state-dir", "", "state directory for --dump-tables (default: ./state)")
	waitMs := flag.Int("wait-ms", 3000, "ms to wait for bootstrap convergence before lookup")
	flag.Parse()

	if *dumpAll {
		if err := runDumpTables(*stateDir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

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

	if *lookupHex != "" {
		if err := runLookupMode(ctx, n, *lookupHex, *waitMs); err != nil {
			logger.Error("lookup mode", "err", err)
			os.Exit(1)
		}
		return
	}

	if err := n.Run(ctx); err != nil {
		logger.Error("run", "err", err)
		os.Exit(1)
	}
}

// ---------- lookup mode ----------

type lookupReport struct {
	LookupID    string   `json:"lookup_id"`
	InitiatorID string   `json:"initiator_id"`
	TargetHex   string   `json:"target_hex"`
	Success     bool     `json:"success"`
	RPCs        int      `json:"rpcs"`
	Iterations  int      `json:"iterations"`
	Timeouts    []string `json:"timeouts_hex"`
	DurationMs  int64    `json:"duration_ms"`
	ClosestHex  []string `json:"closest_hex"`
	TableBefore int      `json:"table_size_before"`
	TableAfter  int      `json:"table_size_after"`
	Timestamp   string   `json:"timestamp"`
}

func runLookupMode(ctx context.Context, n *node.Node, targetHex string, waitMs int) error {
	target, err := parseHexNodeID(targetHex)
	if err != nil {
		return fmt.Errorf("bad target: %w", err)
	}

	if err := n.Start(); err != nil {
		return err
	}
	if err := n.BootstrapPeers(); err != nil {
		// не фатально: если у цели/инициатора уже есть контакты,
		// lookup может пройти без bootstrap.
		n.Logger().Warn("bootstrap failed in lookup mode", "err", err.Error())
	}

	// Ждём сходимости: за это время другие узлы успевают прислать PING
	// и заполнить нашу таблицу. Значение задаётся --wait-ms.
	if waitMs > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(waitMs) * time.Millisecond):
		}
	}

	before := len(n.TableSnapshot())

	res := n.LookupNode(target)

	report := lookupReport{
		LookupID:    res.LookupID,
		InitiatorID: n.ShortID(),
		TargetHex:   targetHex,
		Success:     res.Success,
		RPCs:        res.RPCs,
		Iterations:  res.Iterations,
		DurationMs:  res.Duration.Milliseconds(),
		TableBefore: before,
		TableAfter:  len(n.TableSnapshot()),
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	for _, id := range res.Timeouts {
		report.Timeouts = append(report.Timeouts, hex.EncodeToString(id[:]))
	}
	for _, c := range res.Closest {
		report.ClosestHex = append(report.ClosestHex, hex.EncodeToString(c.NodeID[:]))
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return err
	}
	return nil
}

func parseHexNodeID(s string) (routing.NodeID, error) {
	var id routing.NodeID
	b, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}
	if len(b) != 32 {
		return id, fmt.Errorf("node id must be 32 bytes, got %d", len(b))
	}
	copy(id[:], b)
	return id, nil
}

// ---------- dump-tables mode ----------

type dumpSummary struct {
	NodeShort string `json:"node_short"`
	SelfID    string `json:"self_id"`
	TableSize int    `json:"table_size"`
	Path      string `json:"path"`
}

func runDumpTables(stateDir string) error {
	base := stateDir
	if base == "" {
		root, err := os.Getwd()
		if err != nil {
			return err
		}
		base = filepath.Join(root, "state")
	}
	pattern := filepath.Join(base, "node-*", "table.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("no table.json found under state/node-*/")
	}
	sort.Strings(matches)

	summaries := make([]dumpSummary, 0, len(matches))
	totalSize := 0
	for _, p := range matches {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var d struct {
			SelfID    string `json:"self_id"`
			SelfShort string `json:"self_short"`
			TableSize int    `json:"table_size"`
		}
		if err := json.Unmarshal(data, &d); err != nil {
			continue
		}
		summaries = append(summaries, dumpSummary{
			NodeShort: d.SelfShort,
			SelfID:    d.SelfID,
			TableSize: d.TableSize,
			Path:      p,
		})
		totalSize += d.TableSize
	}

	out := struct {
		Count     int           `json:"count"`
		TotalSize int           `json:"total_size"`
		AvgSize   float64       `json:"avg_size"`
		Nodes     []dumpSummary `json:"nodes"`
	}{
		Count:     len(summaries),
		TotalSize: totalSize,
		Nodes:     summaries,
	}
	if len(summaries) > 0 {
		out.AvgSize = float64(totalSize) / float64(len(summaries))
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
