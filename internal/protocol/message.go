package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

type MessageType byte

const (
	// Frontend messages (from client)
	MsgStartup   MessageType = 0   // Startup message (no type byte)
	MsgQuery     MessageType = 'Q' // Simple query
	MsgParse     MessageType = 'P' // Parse (prepared statement)
	MsgBind      MessageType = 'B' // Bind
	MsgExecute   MessageType = 'E' // Execute
	MsgDescribe  MessageType = 'D' // Describe
	MsgClose     MessageType = 'C' // Close
	MsgSync      MessageType = 'S' // Sync
	MsgTerminate MessageType = 'X' // Terminate
	MsgPassword  MessageType = 'p' // Password message
	MsgFlush     MessageType = 'H' // Flush

	// Backend messages (from server)
	MsgAuthRequest     MessageType = 'R' // Authentication request
	MsgBackendKeyData  MessageType = 'K' // Backend key data
	MsgBindComplete    MessageType = '2' // Bind complete
	MsgCloseComplete   MessageType = '3' // Close complete
	MsgCommandComplete MessageType = 'C' // Command complete
	MsgDataRow         MessageType = 'D' // Data row
	MsgEmptyQueryResp  MessageType = 'I' // Empty query response
	MsgErrorResponse   MessageType = 'E' // Error response
	MsgNoData          MessageType = 'n' // No data
	MsgNoticeResponse  MessageType = 'N' // Notice response
	MsgParameterStatus MessageType = 'S' // Parameter status
	MsgParseComplete   MessageType = '1' // Parse complete
	MsgReadyForQuery   MessageType = 'Z' // Ready for query
	MsgRowDescription  MessageType = 'T' // Row description
)

type Message struct {
	Type MessageType
	Data []byte
}

func ReadMessage(r io.Reader) (*Message, error) {
	typeBuf := make([]byte, 1)
	if _, err := io.ReadFull(r, typeBuf); err != nil {
		return nil, err
	}

	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lenBuf)

	if length < 4 {
		return nil, fmt.Errorf("invalid message length: %d", length)
	}

	data := make([]byte, length-4)
	if len(data) > 0 {
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}
	}

	return &Message{
		Type: MessageType(typeBuf[0]),
		Data: data,
	}, nil
}

func WriteMessage(w io.Writer, msg *Message) error {
	if _, err := w.Write([]byte{byte(msg.Type)}); err != nil {
		return err
	}

	length := uint32(len(msg.Data) + 4)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, length)
	if _, err := w.Write(lenBuf); err != nil {
		return err
	}

	if len(msg.Data) > 0 {
		if _, err := w.Write(msg.Data); err != nil {
			return err
		}
	}

	return nil
}

func ReadStartupMessage(r io.Reader) (*Message, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lenBuf)

	if length < 4 {
		return nil, fmt.Errorf("invalid startup message length: %d", length)
	}

	data := make([]byte, length-4)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	return &Message{
		Type: MsgStartup,
		Data: data,
	}, nil
}
