package transport

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math/rand"
	"testing"
)

func mkFrame(t uint8, payload []byte) *Frame {
	var rid [16]byte
	for i := range rid {
		rid[i] = byte(i)
	}
	return &Frame{
		Version:   ProtocolVersion,
		Type:      t,
		Flags:     0,
		RequestID: rid,
		Payload:   payload,
	}
}

// T1.1: кодирование и декодирование корректного кадра.
func TestT1_1_RoundTrip(t *testing.T) {
	orig := mkFrame(0x01, []byte("hello, world"))
	b := Encode(orig)

	got, err := Decode(bytes.NewReader(b), MaxFramePayload, ProtocolVersion)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Version != orig.Version ||
		got.Type != orig.Type ||
		got.Flags != orig.Flags ||
		got.RequestID != orig.RequestID ||
		!bytes.Equal(got.Payload, orig.Payload) {
		t.Fatalf("frame mismatch:\n  orig=%+v\n  got =%+v", orig, got)
	}
}

// T1.2: частичный заголовок — декодер не должен вернуть Frame.
func TestT1_2_PartialHeader(t *testing.T) {
	for n := 0; n < HeaderSize; n++ {
		r := bytes.NewReader(make([]byte, n))
		_, err := Decode(r, MaxFramePayload, ProtocolVersion)
		if !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			t.Fatalf("n=%d: expected EOF-family, got %v", n, err)
		}
	}
}

// T1.3: частичный payload — декодер не должен вернуть Frame.
func TestT1_3_PartialPayload(t *testing.T) {
	full := Encode(mkFrame(0x01, make([]byte, 50)))
	for extra := 0; extra < 50; extra++ {
		r := bytes.NewReader(full[:HeaderSize+extra])
		_, err := Decode(r, MaxFramePayload, ProtocolVersion)
		if !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			t.Fatalf("extra=%d: expected EOF-family, got %v", extra, err)
		}
	}
}

// T1.4: склеенные кадры — оба кадра читаются по порядку.
func TestT1_4_GluedFrames(t *testing.T) {
	f1 := mkFrame(0x01, []byte("first"))
	f2 := mkFrame(0x02, []byte("second"))
	buf := append(Encode(f1), Encode(f2)...)

	r := bytes.NewReader(buf)
	got1, err := Decode(r, MaxFramePayload, ProtocolVersion)
	if err != nil {
		t.Fatalf("frame1: %v", err)
	}
	got2, err := Decode(r, MaxFramePayload, ProtocolVersion)
	if err != nil {
		t.Fatalf("frame2: %v", err)
	}
	if got1.Type != 0x01 || !bytes.Equal(got1.Payload, []byte("first")) {
		t.Fatalf("frame1 wrong: %+v", got1)
	}
	if got2.Type != 0x02 || !bytes.Equal(got2.Payload, []byte("second")) {
		t.Fatalf("frame2 wrong: %+v", got2)
	}
}

// T1.5: 100 кадров, поток режется на куски 1..17 байт.
// Используется фиксированный seed — тест воспроизводим.
func TestT1_5_ArbitrarySplit(t *testing.T) {
	const N = 100
	r := rand.New(rand.NewSource(42))

	frames := make([]*Frame, N)
	var stream []byte
	for i := 0; i < N; i++ {
		plen := r.Intn(200)
		pl := make([]byte, plen)
		r.Read(pl)
		frames[i] = mkFrame(uint8(i%250+1), pl)
		stream = append(stream, Encode(frames[i])...)
	}

	pr, pw := io.Pipe()

	// Writer: режет поток на куски 1..17 байт.
	go func() {
		defer pw.Close()
		rest := stream
		for len(rest) > 0 {
			n := r.Intn(17) + 1
			if n > len(rest) {
				n = len(rest)
			}
			if _, err := pw.Write(rest[:n]); err != nil {
				return
			}
			rest = rest[n:]
		}
	}()

	// Гарантируем, что writer-горутина разблокируется при любом
	// исходе — в том числе если тест упадёт раньше и вызовет Fatal.
	defer pr.Close()

	for i := 0; i < N; i++ {
		got, err := Decode(pr, MaxFramePayload, ProtocolVersion)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if got.Type != frames[i].Type ||
			!bytes.Equal(got.Payload, frames[i].Payload) {
			t.Fatalf("frame %d mismatch", i)
		}
	}
}

func TestT1_5b_AllSplitPoints(t *testing.T) {
	f := mkFrame(0x01, []byte("payload-for-split-test"))
	stream := Encode(f)

	for split := 1; split < len(stream); split++ {
		pr, pw := io.Pipe()
		go func(s int) {
			defer pw.Close()
			_, _ = pw.Write(stream[:s])
			_, _ = pw.Write(stream[s:])
		}(split)

		got, err := Decode(pr, MaxFramePayload, ProtocolVersion)
		_ = pr.Close()

		if err != nil {
			t.Fatalf("split=%d: %v", split, err)
		}
		if got.Type != f.Type || !bytes.Equal(got.Payload, f.Payload) {
			t.Fatalf("split=%d: frame mismatch", split)
		}
	}
}

// T1.6: payload_length > maxPayload отклоняется до аллокации.
// Проверяем как с дефолтным лимитом, так и с конфигурируемым.
func TestT1_6_OverlongPayload(t *testing.T) {
	var hdr [HeaderSize]byte
	hdr[0] = ProtocolVersion
	hdr[1] = 0x01
	binary.BigEndian.PutUint32(hdr[20:24], MaxFramePayload+1)

	_, err := Decode(bytes.NewReader(hdr[:]), MaxFramePayload, ProtocolVersion)
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("default limit: want ErrPayloadTooLarge, got %v", err)
	}

	// Кастомный лимит: maxPayload = 512, кадр с plen = 513 должен отклоняться.
	binary.BigEndian.PutUint32(hdr[20:24], 513)
	_, err = Decode(bytes.NewReader(hdr[:]), 512, ProtocolVersion)
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("custom limit 512: want ErrPayloadTooLarge, got %v", err)
	}

	// Кадр с plen = 512 при лимите 512 должен проходить проверку длины
	// (и упасть дальше на чтении payload, потому что байтов нет).
	binary.BigEndian.PutUint32(hdr[20:24], 512)
	_, err = Decode(bytes.NewReader(hdr[:]), 512, ProtocolVersion)
	if errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("plen == maxPayload should not trigger ErrPayloadTooLarge, got %v", err)
	}
}

// T1.7: неизвестная версия протокола — проверяется и дефолтная, и кастомная.
func TestT1_7_BadVersion(t *testing.T) {
	var hdr [HeaderSize]byte
	hdr[0] = 99
	hdr[1] = 0x01
	_, err := Decode(bytes.NewReader(hdr[:]), MaxFramePayload, ProtocolVersion)
	if !errors.Is(err, ErrBadVersion) {
		t.Fatalf("default version: want ErrBadVersion, got %v", err)
	}

	// Кастомная ожидаемая версия: кадр с version=1, ожидаем version=2.
	hdr[0] = 1
	_, err = Decode(bytes.NewReader(hdr[:]), MaxFramePayload, 2)
	if !errors.Is(err, ErrBadVersion) {
		t.Fatalf("custom version 2: want ErrBadVersion, got %v", err)
	}

	// Кадр с version=2 и ожидаемой version=2 не должен падать на версии
	// (упадёт дальше на чтении payload).
	hdr[0] = 2
	_, err = Decode(bytes.NewReader(hdr[:]), MaxFramePayload, 2)
	if errors.Is(err, ErrBadVersion) {
		t.Fatalf("version match should not trigger ErrBadVersion, got %v", err)
	}
}

// T1.8: неизвестный тип сообщения.
// Транспорт не знает про типы — это ответственность диспетчера.
// Тест фиксирует контракт: Decode обязан вернуть Frame с любым type.
func TestT1_8_UnknownTypePassesThrough(t *testing.T) {
	f := mkFrame(0xAB, []byte("x"))
	got, err := Decode(bytes.NewReader(Encode(f)), MaxFramePayload, ProtocolVersion)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Type != 0xAB {
		t.Fatalf("type got %x want 0xAB", got.Type)
	}
}

// T1.9: разрыв внутри кадра — Frame не возвращается.
func TestT1_9_BrokenMidFrame(t *testing.T) {
	full := Encode(mkFrame(0x01, make([]byte, 100)))
	for extra := 1; extra < 100; extra++ {
		r := bytes.NewReader(full[:HeaderSize+extra])
		_, err := Decode(r, MaxFramePayload, ProtocolVersion)
		if !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			t.Fatalf("extra=%d: want EOF-family, got %v", extra, err)
		}
	}
}
