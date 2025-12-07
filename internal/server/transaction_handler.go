package server

import (
	"context"
	"fmt"
	"time"

	"github.com/1ef7yy/aether/internal/protocol"
)

// TransactionHandler handles client connections in transaction pooling mode
// In transaction mode, connections are acquired per transaction and released after commit/rollback
type TransactionHandler struct {
	client *Client
}

func NewTransactionHandler(client *Client) *TransactionHandler {
	return &TransactionHandler{client: client}
}

// HandleMessage processes a message from the client in transaction mode
func (h *TransactionHandler) HandleMessage(ctx context.Context, msg *protocol.Message) error {
	if msg.Type != protocol.MsgQuery {
		// Only queries can start/end transactions
		if h.client.backendConn == nil {
			return fmt.Errorf("no backend connection (query required to start transaction)")
		}
		if err := h.client.backendConn.WriteMessage(msg); err != nil {
			return fmt.Errorf("backend write error: %w", err)
		}
		return nil
	}

	queryStr := extractQuery(msg.Data)
	isBegin := isBeginQuery(queryStr)
	isCommit := isCommitQuery(queryStr)
	isRollback := isRollbackQuery(queryStr)

	// Acquire connection on BEGIN or for standalone queries
	needsConnection := h.client.backendConn == nil
	if needsConnection {
		backendConn, err := h.client.pool.Acquire(ctx)
		if err != nil {
			return fmt.Errorf("failed to acquire backend connection: %w", err)
		}
		h.client.mu.Lock()
		h.client.backendConn = backendConn
		// Only mark as in transaction if this is a BEGIN statement
		if isBegin {
			h.client.inTransaction = true
		}
		h.client.mu.Unlock()
	} else if isBegin && !h.client.inTransaction {
		// Already have a connection, just mark as in transaction
		h.client.mu.Lock()
		h.client.inTransaction = true
		h.client.mu.Unlock()
	}

	if err := h.client.backendConn.WriteMessage(msg); err != nil {
		return fmt.Errorf("backend write error: %w", err)
	}

	// Release connection on COMMIT/ROLLBACK
	if (isCommit || isRollback) && h.client.inTransaction {
		h.client.mu.Lock()
		if h.client.backendConn != nil {
			// We need to wait for ReadyForQuery before releasing
			// This will be handled in HandleBackendMessage
			h.client.inTransaction = false
		}
		h.client.mu.Unlock()
	}

	return nil
}

// HandleBackendMessage processes messages from the backend in transaction mode
func (h *TransactionHandler) HandleBackendMessage(msg *protocol.Message) error {
	// Release connection after ReadyForQuery when not in a transaction
	// This handles both: standalone queries and COMMIT/ROLLBACK
	if msg.Type == protocol.MsgReadyForQuery {
		h.client.mu.Lock()
		if !h.client.inTransaction && h.client.backendConn != nil {
			h.client.pool.Release(h.client.backendConn)
			h.client.backendConn = nil
		}
		h.client.mu.Unlock()
	}
	return nil
}

// ShouldReadBackend returns true if there's a backend connection to read from
func (h *TransactionHandler) ShouldReadBackend() bool {
	h.client.mu.Lock()
	hasBackend := h.client.backendConn != nil
	h.client.mu.Unlock()

	if !hasBackend {
		time.Sleep(10 * time.Millisecond)
		return false
	}
	return true
}

// Cleanup performs any necessary cleanup when the session ends
func (h *TransactionHandler) Cleanup() error {
	// Release any held connection
	h.client.mu.Lock()
	if h.client.backendConn != nil {
		h.client.pool.Release(h.client.backendConn)
		h.client.backendConn = nil
	}
	h.client.mu.Unlock()
	return nil
}
