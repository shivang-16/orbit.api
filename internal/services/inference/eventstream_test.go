package inference

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"strings"
	"testing"
)

// encodeFrame builds a valid AWS event-stream frame from a set of
// string-valued headers and a payload, mirroring what Bedrock actually
// sends on the wire. Used to test the decoder without any network access.
func encodeFrame(t *testing.T, headers map[string]string, payload []byte) []byte {
	t.Helper()

	var headerBuf bytes.Buffer
	for name, value := range headers {
		headerBuf.WriteByte(byte(len(name)))
		headerBuf.WriteString(name)
		headerBuf.WriteByte(headerTypeString)
		var lenBuf [2]byte
		binary.BigEndian.PutUint16(lenBuf[:], uint16(len(value)))
		headerBuf.Write(lenBuf[:])
		headerBuf.WriteString(value)
	}
	headerBytes := headerBuf.Bytes()

	totalLen := eventStreamPreludeLen + len(headerBytes) + len(payload) + eventStreamTrailerLen

	frame := make([]byte, 0, totalLen)
	prelude := make([]byte, eventStreamPreludeLen)
	binary.BigEndian.PutUint32(prelude[0:4], uint32(totalLen))
	binary.BigEndian.PutUint32(prelude[4:8], uint32(len(headerBytes)))
	binary.BigEndian.PutUint32(prelude[8:12], crc32.ChecksumIEEE(prelude[0:8]))
	frame = append(frame, prelude...)
	frame = append(frame, headerBytes...)
	frame = append(frame, payload...)

	messageCRC := crc32.ChecksumIEEE(frame)
	var crcBuf [4]byte
	binary.BigEndian.PutUint32(crcBuf[:], messageCRC)
	frame = append(frame, crcBuf[:]...)

	return frame
}

func TestReadAWSEventStreamFrame_RoundTrip(t *testing.T) {
	payload := []byte(`{"contentBlockIndex":0,"delta":{"text":"hi"}}`)
	raw := encodeFrame(t, map[string]string{
		":event-type":   "contentBlockDelta",
		":content-type": "application/json",
		":message-type": "event",
	}, payload)

	frame, err := readAWSEventStreamFrame(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if frame.headers[":event-type"] != "contentBlockDelta" {
		t.Fatalf("event-type = %q, want contentBlockDelta", frame.headers[":event-type"])
	}
	if frame.headers[":message-type"] != "event" {
		t.Fatalf("message-type = %q, want event", frame.headers[":message-type"])
	}
	if !bytes.Equal(frame.payload, payload) {
		t.Fatalf("payload = %q, want %q", frame.payload, payload)
	}
}

func TestReadAWSEventStreamFrame_MultipleFrames(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(encodeFrame(t, map[string]string{":event-type": "messageStart", ":message-type": "event"}, []byte(`{"role":"assistant"}`)))
	buf.Write(encodeFrame(t, map[string]string{":event-type": "metadata", ":message-type": "event"}, []byte(`{"usage":{"inputTokens":1,"outputTokens":2}}`)))

	reader := bufio.NewReader(&buf)

	first, err := readAWSEventStreamFrame(reader)
	if err != nil {
		t.Fatalf("first frame: %v", err)
	}
	if first.headers[":event-type"] != "messageStart" {
		t.Fatalf("first event-type = %q", first.headers[":event-type"])
	}

	second, err := readAWSEventStreamFrame(reader)
	if err != nil {
		t.Fatalf("second frame: %v", err)
	}
	if second.headers[":event-type"] != "metadata" {
		t.Fatalf("second event-type = %q", second.headers[":event-type"])
	}

	if _, err := readAWSEventStreamFrame(reader); !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF after last frame, got %v", err)
	}
}

func TestReadAWSEventStreamFrame_ExceptionFrame(t *testing.T) {
	raw := encodeFrame(t, map[string]string{
		":exception-type": "modelStreamErrorException",
		":message-type":   "exception",
	}, []byte(`{"message":"stream failed"}`))

	frame, err := readAWSEventStreamFrame(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if frame.headers[":message-type"] != "exception" {
		t.Fatalf("message-type = %q, want exception", frame.headers[":message-type"])
	}
	if frame.headers[":exception-type"] != "modelStreamErrorException" {
		t.Fatalf("exception-type = %q", frame.headers[":exception-type"])
	}
}

func TestReadAWSEventStreamFrame_CorruptPreludeCRC(t *testing.T) {
	raw := encodeFrame(t, map[string]string{":event-type": "messageStart", ":message-type": "event"}, []byte(`{}`))
	raw[8] ^= 0xFF // flip a bit in the prelude CRC

	_, err := readAWSEventStreamFrame(bufio.NewReader(bytes.NewReader(raw)))
	if !errors.Is(err, errEventStreamCRCMismatch) {
		t.Fatalf("expected crc mismatch, got %v", err)
	}
}

func TestReadAWSEventStreamFrame_CorruptPayload(t *testing.T) {
	raw := encodeFrame(t, map[string]string{":event-type": "messageStart", ":message-type": "event"}, []byte(`{"role":"assistant"}`))
	raw[len(raw)-6] ^= 0xFF // flip a bit inside the payload, after the (valid) prelude

	_, err := readAWSEventStreamFrame(bufio.NewReader(bytes.NewReader(raw)))
	if !errors.Is(err, errEventStreamCRCMismatch) {
		t.Fatalf("expected message crc mismatch, got %v", err)
	}
}

func TestReadAWSEventStreamFrame_TruncatedFrame(t *testing.T) {
	raw := encodeFrame(t, map[string]string{":event-type": "messageStart", ":message-type": "event"}, []byte(`{"role":"assistant"}`))
	truncated := raw[:len(raw)-10]

	_, err := readAWSEventStreamFrame(bufio.NewReader(bytes.NewReader(truncated)))
	if err == nil {
		t.Fatal("expected an error for a truncated frame, got nil")
	}
}

func TestReadAWSEventStreamFrame_OversizedFrameRejected(t *testing.T) {
	prelude := make([]byte, eventStreamPreludeLen)
	binary.BigEndian.PutUint32(prelude[0:4], maxEventStreamFrameLen+1)
	binary.BigEndian.PutUint32(prelude[4:8], 0)
	binary.BigEndian.PutUint32(prelude[8:12], crc32.ChecksumIEEE(prelude[0:8]))

	_, err := readAWSEventStreamFrame(bufio.NewReader(bytes.NewReader(prelude)))
	if err == nil || !strings.Contains(err.Error(), "exceeds max") {
		t.Fatalf("expected a max-frame-length error, got %v", err)
	}
}

func TestReadAWSEventStreamFrame_EmptyStreamIsCleanEOF(t *testing.T) {
	_, err := readAWSEventStreamFrame(bufio.NewReader(bytes.NewReader(nil)))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF on an empty stream, got %v", err)
	}
}

func TestParseEventStreamHeaders_AllValueTypesSkipCorrectly(t *testing.T) {
	var buf bytes.Buffer

	writeHeader := func(name string, valueType byte, value []byte) {
		buf.WriteByte(byte(len(name)))
		buf.WriteString(name)
		buf.WriteByte(valueType)
		buf.Write(value)
	}

	writeHeader("bool-true", headerTypeBoolTrue, nil)
	writeHeader("bool-false", headerTypeBoolFalse, nil)
	writeHeader("a-byte", headerTypeByte, []byte{0x2A})
	writeHeader("a-short", headerTypeShort, []byte{0x00, 0x01})
	writeHeader("an-int", headerTypeInteger, []byte{0x00, 0x00, 0x00, 0x01})
	writeHeader("a-long", headerTypeLong, make([]byte, 8))
	writeHeader("a-timestamp", headerTypeTimestamp, make([]byte, 8))
	writeHeader("a-uuid", headerTypeUUID, make([]byte, 16))

	writeLengthPrefixed := func(name string, valueType byte, value string) {
		buf.WriteByte(byte(len(name)))
		buf.WriteString(name)
		buf.WriteByte(valueType)
		var lenBuf [2]byte
		binary.BigEndian.PutUint16(lenBuf[:], uint16(len(value)))
		buf.Write(lenBuf[:])
		buf.WriteString(value)
	}
	writeLengthPrefixed("a-byte-array", headerTypeByteArray, "\xDE\xAD")
	// A string header placed after every other type, so a bug that
	// mis-skips any preceding value would desync the cursor and this
	// read would fail or return garbage.
	writeLengthPrefixed(":event-type", headerTypeString, "messageStart")

	headers, err := parseEventStreamHeaders(buf.Bytes())
	if err != nil {
		t.Fatalf("parseEventStreamHeaders: %v", err)
	}
	if got := headers[":event-type"]; got != "messageStart" {
		t.Fatalf(":event-type = %q, want messageStart", got)
	}
	// Non-string types aren't surfaced in the map, but must not have
	// desynced parsing of the header that follows them (checked above).
	if _, ok := headers["a-byte-array"]; ok {
		t.Fatalf("byte-array header unexpectedly present in string map")
	}
}
