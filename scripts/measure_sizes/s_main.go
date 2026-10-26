// Команда measure_sizes сравнивает размеры сериализации
// одного и того же payload в JSON, MessagePack и CBOR.
// Используется для обоснования выбора JSON (требование E1-8).
package main

import (
	"encoding/json"
	"fmt"

	"github.com/fxamacker/cbor/v2"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/LAMPOVIYREX/p2p-node/internal/protocol"
)

func main() {
	c := protocol.ContactWire{
		NodeID:            "3b45a1f579a0f665a4116f21cadca55bb8e2123c3bf44e9ccf4467972fd16176",
		IdentityAlgorithm: "ed25519",
		IdentityPublicKey: "2P8Lp+1JOXZtpOVlfMOcVDUAlm4TaYhZ2X+qCWBW8j0=",
		Host:              "127.0.0.1",
		Port:              9101,
	}

	contacts4 := make([]protocol.ContactWire, 4)
	contacts10 := make([]protocol.ContactWire, 10)
	for i := range contacts4 {
		contacts4[i] = c
	}
	for i := range contacts10 {
		contacts10[i] = c
	}

	ping := protocol.PingPayload{
		Sender:      c,
		TimestampMs: 1737123456789,
	}

	find4 := protocol.FindNodeResponsePayload{
		Responder:    c,
		TargetNodeID: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		Contacts:     contacts4,
	}

	find10 := protocol.FindNodeResponsePayload{
		Responder:    c,
		TargetNodeID: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		Contacts:     contacts10,
	}

	fmt.Println("| Payload                     | JSON | MsgPack | CBOR |")
	fmt.Println("|-----------------------------|-----:|--------:|-----:|")
	measure("PING", ping)
	measure("FIND_NODE_RESPONSE (4)      ", find4)
	measure("FIND_NODE_RESPONSE (10)     ", find10)
}

func measure(name string, v interface{}) {
	j, _ := json.Marshal(v)
	m, _ := msgpack.Marshal(v)
	c, _ := cbor.Marshal(v)
	fmt.Printf("| %-27s | %4d | %7d | %4d |\n", name, len(j), len(m), len(c))
}