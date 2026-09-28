package routing

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"net"

	"github.com/LAMPOVIYREX/p2p-node/internal/identity"
)

var (
	ErrBadPubKey      = errors.New("bad public key")
	ErrBadAlgo        = errors.New("bad identity algorithm")
	ErrNodeIDMismatch = errors.New("node id does not match public key")
	ErrBadAddr        = errors.New("bad address")
)

type Contact struct {
	NodeID            NodeID
	IdentityAlgorithm string
	IdentityPublicKey ed25519.PublicKey
	Host              string
	Port              uint16
	LastSeenMs        uint64
	LastVerifiedMs    uint64
}

func (c *Contact) Validate() error {
	if len(c.IdentityPublicKey) != ed25519.PublicKeySize {
		return ErrBadPubKey
	}
	if c.IdentityAlgorithm != "ed25519" {
		return ErrBadAlgo
	}
	if identity.DeriveNodeID(c.IdentityPublicKey) != c.NodeID {
		return ErrNodeIDMismatch
	}
	if c.Host == "" || c.Port == 0 {
		return ErrBadAddr
	}
	return nil
}

func (c *Contact) Addr() string {
	return net.JoinHostPort(c.Host, fmt.Sprint(c.Port))
}
