package routing

import (
	"bytes"

	"github.com/LAMPOVIYREX/p2p-node/internal/identity"
)

type NodeID = identity.NodeID

func Xor(a, b NodeID) [32]byte {
	var d [32]byte
	for i := range a {
		d[i] = a[i] ^ b[i]
	}
	return d
}

func CompareDist(a, b [32]byte) int {
	return bytes.Compare(a[:], b[:])
}
