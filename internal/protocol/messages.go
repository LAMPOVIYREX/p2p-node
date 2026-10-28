package protocol

type ContactWire struct {
	NodeID            string `json:"node_id"`
	IdentityAlgorithm string `json:"identity_algorithm"`
	IdentityPublicKey string `json:"identity_public_key"`
	Host              string `json:"host"`
	Port              uint16 `json:"port"`
}

type PingPayload struct {
	Sender      ContactWire `json:"sender"`
	TimestampMs uint64      `json:"timestamp_ms"`
}

type PongPayload struct {
	Responder            ContactWire `json:"responder"`
	PingTimestampMs      uint64      `json:"ping_timestamp_ms"`
	ResponderTimestampMs uint64      `json:"responder_timestamp_ms"`
}

type FindNodeRequestPayload struct {
	Sender       ContactWire `json:"sender"`
	TargetNodeID string      `json:"target_node_id"`
}

type FindNodeResponsePayload struct {
	Responder    ContactWire   `json:"responder"`
	TargetNodeID string        `json:"target_node_id"`
	Contacts     []ContactWire `json:"contacts"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type WireNodeRecord struct {
	NodeID            string   `json:"node_id"`
	IdentityPublicKey string   `json:"identity_public_key"`
	Addresses         []string `json:"addresses"`
	SequenceNumber    uint64   `json:"sequence_number"`
	IssuedAt          uint64   `json:"issued_at"`
	ExpiresAt         uint64   `json:"expires_at"`
	Alias             string   `json:"alias,omitempty"`
	Signature         string   `json:"signature"`
}

type StoreRequestPayload struct {
	Sender ContactWire    `json:"sender"`
	Record WireNodeRecord `json:"record"`
}

type StoreResponsePayload struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type FindValueRequestPayload struct {
	Sender ContactWire `json:"sender"`
	Key    string      `json:"key"` // hex (SHA-256 от "node:..." или "alias:...")
}

type FindValueResponsePayload struct {
	Responder ContactWire     `json:"responder"`
	Key       string          `json:"key"`
	Record    *WireNodeRecord `json:"record,omitempty"` // nil, если не найдено
	Contacts  []ContactWire   `json:"contacts"`         // ближайшие узлы, если записи нет (fallback)
}
