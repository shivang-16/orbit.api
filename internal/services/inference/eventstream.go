package inference

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

// Bedrock's ConverseStream operation does not return plain-text
// Server-Sent-Events over HTTP: it returns AWS's binary "vnd.amazon.event-
// stream" framing (the same wire format used by S3 Select, Kinesis, and
// Transcribe), regardless of whether the call is authenticated with SigV4
// or a bearer API key. Each frame looks like:
//
//	[4]  total length        (big-endian uint32, includes everything below)
//	[4]  headers length       (big-endian uint32)
//	[4]  prelude CRC32        (of the two length fields above)
//	[headers length]  headers (name/type/value triples, see readHeaders)
//	[payload]                 (raw bytes, JSON for every event Bedrock sends)
//	[4]  message CRC32        (of everything from total length through payload)
//
// The header ":event-type" names which ConverseStreamOutput union member
// the payload is (messageStart, contentBlockDelta, ..., metadata), and
// ":message-type" is "event" for normal frames or "exception" when Bedrock
// aborts the stream after already sending a 200 (see
// API_runtime_ConverseStreamOutput in the Bedrock API reference).
type awsEventStreamFrame struct {
	headers map[string]string
	payload []byte
}

const (
	eventStreamPreludeLen = 12 // total length + headers length + prelude CRC
	eventStreamTrailerLen = 4  // message CRC

	// maxEventStreamFrameLen caps how large a single frame we'll allocate
	// for. totalLen is attacker/corruption-controlled network input (a
	// uint32, so up to ~4GB) read before we've confirmed that many bytes
	// actually exist; without a cap, a corrupted or malicious response
	// could force a multi-gigabyte allocation per frame. Bedrock's real
	// ConverseStream chunks are a few hundred bytes to a few KB, so 1MiB
	// leaves generous headroom.
	maxEventStreamFrameLen = 1 << 20
)

var errEventStreamCRCMismatch = errors.New("aws event-stream: crc mismatch")

// readAWSEventStreamFrame reads exactly one frame from r. It returns
// io.EOF (unwrapped) only when the stream ends cleanly between frames.
func readAWSEventStreamFrame(r *bufio.Reader) (*awsEventStreamFrame, error) {
	prelude := make([]byte, eventStreamPreludeLen)
	if _, err := io.ReadFull(r, prelude); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("aws event-stream: truncated prelude: %w", err)
		}
		return nil, err // typically io.EOF between frames
	}

	totalLen := binary.BigEndian.Uint32(prelude[0:4])
	headersLen := binary.BigEndian.Uint32(prelude[4:8])
	preludeCRC := binary.BigEndian.Uint32(prelude[8:12])

	if got := crc32.ChecksumIEEE(prelude[0:8]); got != preludeCRC {
		return nil, errEventStreamCRCMismatch
	}
	// 16 = prelude (12) + message CRC (4); anything smaller is malformed.
	if totalLen < uint32(eventStreamPreludeLen+eventStreamTrailerLen) || headersLen > totalLen {
		return nil, fmt.Errorf("aws event-stream: invalid frame lengths total=%d headers=%d", totalLen, headersLen)
	}
	if totalLen > maxEventStreamFrameLen {
		return nil, fmt.Errorf("aws event-stream: frame length %d exceeds max %d", totalLen, maxEventStreamFrameLen)
	}

	rest := make([]byte, totalLen-eventStreamPreludeLen)
	if _, err := io.ReadFull(r, rest); err != nil {
		return nil, fmt.Errorf("aws event-stream: truncated frame: %w", err)
	}

	headerBytes := rest[:headersLen]
	payloadEnd := len(rest) - eventStreamTrailerLen
	payload := rest[headersLen:payloadEnd]
	messageCRC := binary.BigEndian.Uint32(rest[payloadEnd:])

	full := make([]byte, 0, len(prelude)+len(rest))
	full = append(full, prelude...)
	full = append(full, rest...)
	if got := crc32.ChecksumIEEE(full[:len(full)-eventStreamTrailerLen]); got != messageCRC {
		return nil, errEventStreamCRCMismatch
	}

	headers, err := parseEventStreamHeaders(headerBytes)
	if err != nil {
		return nil, err
	}
	return &awsEventStreamFrame{headers: headers, payload: payload}, nil
}

// Header value type tags, per the AWS event-stream spec.
const (
	headerTypeBoolTrue  = 0
	headerTypeBoolFalse = 1
	headerTypeByte      = 2
	headerTypeShort     = 3
	headerTypeInteger   = 4
	headerTypeLong      = 5
	headerTypeByteArray = 6
	headerTypeString    = 7
	headerTypeTimestamp = 8
	headerTypeUUID      = 9
)

// parseEventStreamHeaders decodes the repeated name/type/value triples in
// a frame's header block. Bedrock only ever sends string-valued headers
// (:event-type, :content-type, :message-type, :exception-type), but every
// type is parsed far enough to skip its bytes correctly so a header of an
// unexpected type can't desync the rest of the block.
func parseEventStreamHeaders(b []byte) (map[string]string, error) {
	headers := make(map[string]string)
	for len(b) > 0 {
		if len(b) < 1 {
			return nil, fmt.Errorf("aws event-stream: truncated header name length")
		}
		nameLen := int(b[0])
		b = b[1:]
		if len(b) < nameLen+1 {
			return nil, fmt.Errorf("aws event-stream: truncated header name/type")
		}
		name := string(b[:nameLen])
		b = b[nameLen:]
		valueType := b[0]
		b = b[1:]

		switch valueType {
		case headerTypeBoolTrue, headerTypeBoolFalse:
			headers[name] = fmt.Sprintf("%t", valueType == headerTypeBoolTrue)
		case headerTypeByte:
			if len(b) < 1 {
				return nil, fmt.Errorf("aws event-stream: truncated byte header %q", name)
			}
			b = b[1:]
		case headerTypeShort:
			if len(b) < 2 {
				return nil, fmt.Errorf("aws event-stream: truncated short header %q", name)
			}
			b = b[2:]
		case headerTypeInteger:
			if len(b) < 4 {
				return nil, fmt.Errorf("aws event-stream: truncated integer header %q", name)
			}
			b = b[4:]
		case headerTypeLong, headerTypeTimestamp:
			if len(b) < 8 {
				return nil, fmt.Errorf("aws event-stream: truncated long/timestamp header %q", name)
			}
			b = b[8:]
		case headerTypeUUID:
			if len(b) < 16 {
				return nil, fmt.Errorf("aws event-stream: truncated uuid header %q", name)
			}
			b = b[16:]
		case headerTypeByteArray, headerTypeString:
			if len(b) < 2 {
				return nil, fmt.Errorf("aws event-stream: truncated length for header %q", name)
			}
			valLen := int(binary.BigEndian.Uint16(b[:2]))
			b = b[2:]
			if len(b) < valLen {
				return nil, fmt.Errorf("aws event-stream: truncated value for header %q", name)
			}
			if valueType == headerTypeString {
				headers[name] = string(b[:valLen])
			}
			b = b[valLen:]
		default:
			return nil, fmt.Errorf("aws event-stream: unknown header value type %d for %q", valueType, name)
		}
	}
	return headers, nil
}
