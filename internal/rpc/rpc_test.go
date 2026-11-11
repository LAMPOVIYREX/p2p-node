package rpc_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/LAMPOVIYREX/p2p-node/internal/identity"
)

func mkRpcTestIdentity(t *testing.T) *identity.Identity {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &identity.Identity{
		Private: priv,
		Public:  pub,
		NodeID:  identity.DeriveNodeID(pub),
	}
}
