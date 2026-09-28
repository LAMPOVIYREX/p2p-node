package protocol

import (
	"bytes"
	"encoding/json"
)

func EncodePayload(v any) ([]byte, error) {
	return json.Marshal(v)
}

func DecodePayload(data []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}
