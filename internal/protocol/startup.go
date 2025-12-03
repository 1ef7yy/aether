package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	ProtocolVersion   = 196608
	SSLRequestCode    = 80877103
	CancelRequestCode = 80877102
)

type StartupMessage struct {
	ProtocolVersion uint32
	Parameters      map[string]string
}

func ParseStartupMessage(data []byte) (*StartupMessage, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("startup message too short")
	}

	msg := &StartupMessage{
		Parameters: make(map[string]string),
	}

	msg.ProtocolVersion = binary.BigEndian.Uint32(data[:4])

	pos := 4
	for pos < len(data) {
		nullIdx := bytes.IndexByte(data[pos:], 0)
		if nullIdx == -1 {
			break
		}

		key := string(data[pos : pos+nullIdx])
		pos += nullIdx + 1

		if pos >= len(data) {
			break
		}

		nullIdx = bytes.IndexByte(data[pos:], 0)
		if nullIdx == -1 {
			break
		}

		value := string(data[pos : pos+nullIdx])
		pos += nullIdx + 1

		if key != "" {
			msg.Parameters[key] = value
		}
	}

	return msg, nil
}

func EncodeStartupMessage(params map[string]string) []byte {
	var buf bytes.Buffer

	versionBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(versionBuf, ProtocolVersion)
	buf.Write(versionBuf)

	for key, value := range params {
		buf.WriteString(key)
		buf.WriteByte(0)
		buf.WriteString(value)
		buf.WriteByte(0)
	}

	buf.WriteByte(0)

	return buf.Bytes()
}

func IsSSLRequest(protocolVersion uint32) bool {
	return protocolVersion == SSLRequestCode
}

func IsCancelRequest(protocolVersion uint32) bool {
	return protocolVersion == CancelRequestCode
}
