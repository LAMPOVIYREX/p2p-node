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
