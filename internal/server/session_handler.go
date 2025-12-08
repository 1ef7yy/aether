package server

import (
	"context"
	"fmt"

	"github.com/1ef7yy/aether/internal/protocol"
)

// SessionHandler handles client connections in session pooling mode
// In session mode, each client keeps the same backend connection for the entire session
type SessionHandler struct {
	client *Client
}

func NewSessionHandler(client *Client) *SessionHandler {
	return &SessionHandler{client: client}
}

// HandleMessage processes a message from the client in session mode
func (h *SessionHandler) HandleMessage(ctx context.Context, msg *protocol.Message) error {
	if h.client.backendConn == nil {
		return fmt.Errorf("no backend connection in session mode")
	}

	// Track transaction state for proper cleanup
	if msg.Type == protocol.MsgQuery {
		h.trackQueryState(msg.Data)
	}

	// Forward message to backend
	if err := h.client.backendConn.WriteMessage(msg); err != nil {
		return fmt.Errorf("backend write error: %w", err)
	}

	return nil
}

// trackQueryState monitors queries to track transaction and session state
func (h *SessionHandler) trackQueryState(data []byte) {
	if len(data) == 0 {
		return
	}

	query := extractQuery(data)

	// Check for transaction commands
	if len(query) >= 5 {
		switch query[:5] {
		case "BEGIN", "begin", "START", "start":
			h.client.backendConn.MarkInTransaction(true)
		case "COMMI", "commi": // COMMIT
			h.client.backendConn.MarkInTransaction(false)
		case "ROLLB", "rollb": // ROLLBACK
			h.client.backendConn.MarkInTransaction(false)
		}
	}

	// Check for session-level state changes
	if len(query) >= 3 {
		switch query[:3] {
		case "SET", "set":
			h.client.backendConn.MarkDirty()
		case "CRE", "cre": // CREATE TEMP TABLE
			if len(query) > 12 {
				temp := query[7:min(len(query), 11)]
				if temp == "TEMP" || temp == "temp" {
					h.client.backendConn.MarkDirty()
				}
			}
		case "PRE", "pre": // PREPARE
			h.client.backendConn.MarkDirty()
		}
	}
}

// Cleanup performs any necessary cleanup when the session ends
func (h *SessionHandler) Cleanup() error {
	// In session mode, connection is released when client disconnects
	return nil
}
