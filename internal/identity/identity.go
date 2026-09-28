package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
)

var ErrBadKeyFile = errors.New("bad identity key file")

type Identity struct {
	Private ed25519.PrivateKey
	Public  ed25519.PublicKey
	NodeID  NodeID
}

func LoadOrCreate(dir string) (*Identity, error) {
	keyPath := filepath.Join(dir, "identity.key")
	if data, err := os.ReadFile(keyPath); err == nil {
		if len(data) != ed25519.PrivateKeySize {
			return nil, ErrBadKeyFile
		}
		priv := ed25519.PrivateKey(data)
		pub := priv.Public().(ed25519.PublicKey)
		return &Identity{Private: priv, Public: pub, NodeID: DeriveNodeID(pub)}, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, priv, 0o600); err != nil {
		return nil, err
	}
	return &Identity{Private: priv, Public: pub, NodeID: DeriveNodeID(pub)}, nil
}
