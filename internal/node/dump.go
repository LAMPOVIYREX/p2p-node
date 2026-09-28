package node

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TableDump — сериализуемое представление строки таблицы маршрутизации.
// Все поля, кроме last_seen_ms и last_verified_ms, — публичные данные.
// Эти два — локальные метаданные узла-наблюдателя.
type TableDump struct {
	NodeID            string `json:"node_id"`
	IdentityAlgorithm string `json:"identity_algorithm"`
	IdentityPublicKey string `json:"identity_public_key"`
	Host              string `json:"host"`
	Port              uint16 `json:"port"`
	LastSeenMs        uint64 `json:"last_seen_ms"`
	LastVerifiedMs    uint64 `json:"last_verified_ms"`
}

// FullTableDump — то, что пишется в state/<node>/table.json.
type FullTableDump struct {
	SelfID      string      `json:"self_id"`
	SelfShort   string      `json:"self_short"`
	GeneratedAt string      `json:"generated_at"`
	TableSize   int         `json:"table_size"`
	Contacts    []TableDump `json:"contacts"`
}

// DumpTable собирает текущее состояние таблицы узла.
func (n *Node) DumpTable() FullTableDump {
	contacts := n.table.AllContacts()
	out := FullTableDump{
		SelfID:      fmt.Sprintf("%x", n.self.NodeID),
		SelfShort:   n.ShortID(),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		TableSize:   len(contacts),
		Contacts:    make([]TableDump, 0, len(contacts)),
	}
	for _, c := range contacts {
		out.Contacts = append(out.Contacts, TableDump{
			NodeID:            hex.EncodeToString(c.NodeID[:]),
			IdentityAlgorithm: c.IdentityAlgorithm,
			IdentityPublicKey: base64.StdEncoding.EncodeToString(c.IdentityPublicKey),
			Host:              c.Host,
			Port:              c.Port,
			LastSeenMs:        c.LastSeenMs,
			LastVerifiedMs:    c.LastVerifiedMs,
		})
	}
	return out
}

// SaveTableDump сохраняет текущую таблицу в state/<node>/table.json.
// Пишет через .tmp + rename — читатель никогда не увидит половину файла.
func (n *Node) SaveTableDump() (string, error) {
	dump := n.DumpTable()
	if err := os.MkdirAll(n.cfg.NodeStateDir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(n.cfg.NodeStateDir, "table.json")
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}
