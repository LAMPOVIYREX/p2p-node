package routing

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"testing"
)

type xorVec struct {
	A    string `json:"a"`
	B    string `json:"b"`
	Dist string `json:"dist"`
}

func loadVectors(t *testing.T) []xorVec {
	t.Helper()
	data, err := os.ReadFile("../../testdata/xor_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vecs []xorVec
	if err := json.Unmarshal(data, &vecs); err != nil {
		t.Fatal(err)
	}
	if len(vecs) == 0 {
		t.Fatal("no vectors")
	}
	return vecs
}

// T2.5 (часть 1): XOR-расстояние на эталонных векторах.
func TestT2_5_XorVectors(t *testing.T) {
	for i, v := range loadVectors(t) {
		ab, err := hex.DecodeString(v.A)
		if err != nil {
			t.Fatalf("vec %d: a: %v", i, err)
		}
		bb, err := hex.DecodeString(v.B)
		if err != nil {
			t.Fatalf("vec %d: b: %v", i, err)
		}
		wb, err := hex.DecodeString(v.Dist)
		if err != nil {
			t.Fatalf("vec %d: dist: %v", i, err)
		}
		if len(ab) != 32 || len(bb) != 32 || len(wb) != 32 {
			t.Fatalf("vec %d: bad hex length", i)
		}
		var a, b, want NodeID
		copy(a[:], ab)
		copy(b[:], bb)
		copy(want[:], wb)

		got := Xor(a, b)
		if got != want {
			t.Fatalf("vec %d: got %x want %x", i, got, want)
		}
	}
}

// T2.5 (часть 2): сортировка контактов по XOR-расстоянию до цели.
func TestT2_5_XorSort(t *testing.T) {
	var target NodeID
	for i := range target {
		target[i] = 0xFF
	}

	ids := [][32]byte{
		{}, // dist = FF..FF (максимум)
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFE}, // dist = 1
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, // dist = 0
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00}, // dist = 0x..FF
	}

	sort.Slice(ids, func(i, j int) bool {
		return CompareDist(Xor(ids[i], target), Xor(ids[j], target)) < 0
	})

	// После сортировки первый — точное совпадение (dist=0).
	if ids[0][31] != 0xFF {
		t.Fatalf("smallest distance not first: %x", ids[0])
	}
	// Последний — максимальное расстояние (нули).
	if ids[len(ids)-1] != [32]byte{} {
		t.Fatalf("largest distance not last: %x", ids[len(ids)-1])
	}
}
