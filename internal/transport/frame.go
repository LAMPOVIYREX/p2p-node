package transport

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	HeaderSize = 24

	// Значения по умолчанию, применяются если в конфиге не задано иное.
	MaxFramePayload = 65536
	ProtocolVersion = 1
)

var (
	ErrBadVersion      = errors.New("bad protocol version")
	ErrPayloadTooLarge = errors.New("payload too large")
)

type Frame struct {
	Version       uint8
	Type          uint8
	Flags         uint16
	RequestID     [16]byte
	PayloadLength uint32
	Payload       []byte
}

func Encode(f *Frame) []byte {
	buf := make([]byte, HeaderSize+len(f.Payload))
	buf[0] = f.Version
	buf[1] = f.Type
	binary.BigEndian.PutUint16(buf[2:4], f.Flags)
	copy(buf[4:20], f.RequestID[:])
	binary.BigEndian.PutUint32(buf[20:24], uint32(len(f.Payload)))
	copy(buf[24:], f.Payload)
	return buf
}

// Decode читает ровно один кадр из r.
// maxPayload — максимально допустимая длина payload (из конфига узла).
// expectedVersion — ожидаемая версия протокола (из конфига узла).
func Decode(r io.Reader, maxPayload uint32, expectedVersion uint8) (*Frame, error) {
	var hdr [HeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}

	version := hdr[0]
	if version != expectedVersion {
		return nil, fmt.Errorf("%w: got=%d want=%d", ErrBadVersion, version, expectedVersion)
	}

	typ := hdr[1]
	flags := binary.BigEndian.Uint16(hdr[2:4])
	var rid [16]byte
	copy(rid[:], hdr[4:20])
	plen := binary.BigEndian.Uint32(hdr[20:24])

	if plen > maxPayload {
		return nil, fmt.Errorf("%w: plen=%d max=%d", ErrPayloadTooLarge, plen, maxPayload)
	}

	payload := make([]byte, plen)
	if plen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}
	}

	return &Frame{
		Version:       version,
		Type:          typ,
		Flags:         flags,
		RequestID:     rid,
		PayloadLength: plen,
		Payload:       payload,
	}, nil
}