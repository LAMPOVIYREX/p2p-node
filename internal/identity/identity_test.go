package identity

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// T2.1: перезапуск с тем же NODE_STATE_DIR сохраняет NodeID.
func TestT2_1_PersistentIdentity(t *testing.T) {
	dir := t.TempDir()
	id1, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if id1.NodeID != id2.NodeID {
		t.Fatalf("NodeID changed: %x vs %x", id1.NodeID, id2.NodeID)
	}
	if !bytes.Equal(id1.Public, id2.Public) {
		t.Fatal("public key changed")
	}
}

// T2.2: разные каталоги дают разные NodeID.
func TestT2_2_DifferentNodesDifferentIDs(t *testing.T) {
	id1, err := LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id2, err := LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if id1.NodeID == id2.NodeID {
		t.Fatal("identical NodeIDs from different dirs")
	}
}

// T2.3: NodeID детерминированно пересчитывается из public key.
func TestT2_3_NodeIDFromPublicKey(t *testing.T) {
	id, err := LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if DeriveNodeID(id.Public) != id.NodeID {
		t.Fatal("DeriveNodeID(public) != NodeID")
	}
}

// Файл ключа имеет права 0600; каталог — 0700.
func TestT2_1b_KeyFilePermissions(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadOrCreate(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "identity.key"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key file perms = %o, want 0600", info.Mode().Perm())
	}
}
