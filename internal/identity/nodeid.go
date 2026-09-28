package identity

import (
	"crypto/ed25519"
	"crypto/sha256"
)

type NodeID [32]byte

const CanonicalPrefix = "p2p:identity:ed25519:v1:"

func CanonicalEncode(pub ed25519.PublicKey) []byte {
	out := make([]byte, 0, len(CanonicalPrefix)+len(pub))
	out = append(out, CanonicalPrefix...)
	out = append(out, pub...)
	return out
}

func DeriveNodeID(pub ed25519.PublicKey) NodeID {
	h := sha256.Sum256(CanonicalEncode(pub))
	var id NodeID
	copy(id[:], h[:])
	return id
}
