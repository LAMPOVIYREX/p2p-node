// Скрипт analyze_experiment строит сводную таблицу по результатам
// экспериментов: статистика lookup (RPC, итерации, длительность),
// распределение размеров таблиц и гистограмма контактов по bucket.
// Использование: go run ./scripts/analyze_experiment
package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/LAMPOVIYREX/p2p-node/internal/routing"
)

type lookupReport struct {
	Success     bool  `json:"success"`
	TargetFound bool  `json:"target_found"`
	RPCs        int   `json:"rpcs"`
	Iterations  int   `json:"iterations"`
	DurationMs  int64 `json:"duration_ms"`
}

type tableDump struct {
	SelfID   string `json:"self_id"`
	Contacts []struct {
		NodeID string `json:"node_id"`
	} `json:"contacts"`
}

func main() {
	for _, topo := range []string{"star", "ring"} {
		fmt.Printf("## Topology: %s\n\n", topo)
		analyzeTopo(topo)
		fmt.Println()
	}
}

func analyzeTopo(topo string) {
	base := filepath.Join("docs", "experiments", topo)

	lookups := readLookups(base)
	if len(lookups) == 0 {
		fmt.Println("_no lookup data_")
		return
	}

	rpcs := make([]float64, len(lookups))
	iters := make([]float64, len(lookups))
	durs := make([]float64, len(lookups))
	successCount, foundCount := 0, 0
	for i, l := range lookups {
		rpcs[i] = float64(l.RPCs)
		iters[i] = float64(l.Iterations)
		durs[i] = float64(l.DurationMs)
		if l.Success {
			successCount++
		}
		if l.TargetFound {
			foundCount++
		}
	}

	fmt.Printf("### Lookup statistics (n=%d)\n\n", len(lookups))
	fmt.Println("| Metric | Min | Max | Mean | Median |")
	fmt.Println("|---|---:|---:|---:|---:|")
	printStats("RPCs", rpcs)
	printStats("Iterations", iters)
	printStats("Duration (ms)", durs)
	fmt.Printf("\n- success: %d/%d\n", successCount, len(lookups))
	fmt.Printf("- target_found: %d/%d\n\n", foundCount, len(lookups))

	if sizes := readTableSizes(base); len(sizes) > 0 {
		fmt.Printf("### Table size distribution (n=%d)\n\n", len(sizes))
		fmt.Println("| Metric | Min | Max | Mean | Median |")
		fmt.Println("|---|---:|---:|---:|---:|")
		printStats("Contacts per table", sizes)
		fmt.Println()
	}

	if buckets := readBucketDistribution(base); len(buckets) > 0 {
		fmt.Println("### Contact distribution across buckets")
		fmt.Println()
		fmt.Println("| Bucket | Contacts |")
		fmt.Println("|---:|---:|")
		keys := make([]int, 0, len(buckets))
		for k := range buckets {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		for _, k := range keys {
			fmt.Printf("| %d | %d |\n", k, buckets[k])
		}
		fmt.Println()
	}
}

func readLookups(base string) []lookupReport {
	files, _ := filepath.Glob(filepath.Join(base, "lookup-[0-9]*.json"))
	var res []lookupReport
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var l lookupReport
		if err := json.Unmarshal(data, &l); err != nil {
			continue
		}
		res = append(res, l)
	}
	return res
}

func readTableSizes(base string) []float64 {
	data, err := os.ReadFile(filepath.Join(base, "table-summary.json"))
	if err != nil {
		return nil
	}
	var s struct {
		Nodes []struct {
			TableSize int `json:"table_size"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return nil
	}
	res := make([]float64, len(s.Nodes))
	for i, n := range s.Nodes {
		res[i] = float64(n.TableSize)
	}
	return res
}

func readBucketDistribution(base string) map[int]int {
	files, _ := filepath.Glob(filepath.Join(base, "tables", "node-*.json"))
	if len(files) == 0 {
		return nil
	}
	hist := make(map[int]int)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var td tableDump
		if err := json.Unmarshal(data, &td); err != nil {
			continue
		}
		selfBytes, err := hex.DecodeString(td.SelfID)
		if err != nil || len(selfBytes) != 32 {
			continue
		}
		var self routing.NodeID
		copy(self[:], selfBytes)
		for _, c := range td.Contacts {
			b, err := hex.DecodeString(c.NodeID)
			if err != nil || len(b) != 32 {
				continue
			}
			var other routing.NodeID
			copy(other[:], b)
			idx := routing.BucketIndex(self, other)
			if idx >= 0 {
				hist[idx]++
			}
		}
	}
	return hist
}

func printStats(name string, vals []float64) {
	if len(vals) == 0 {
		return
	}
	sort.Float64s(vals)
	min := vals[0]
	max := vals[len(vals)-1]
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	mean := sum / float64(len(vals))
	var median float64
	if len(vals)%2 == 1 {
		median = vals[len(vals)/2]
	} else {
		median = (vals[len(vals)/2-1] + vals[len(vals)/2]) / 2
	}
	fmt.Printf("| %s | %.2f | %.2f | %.2f | %.2f |\n", name, min, max, mean, median)
}
