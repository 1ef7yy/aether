package backend

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/1ef7yy/aether/internal/protocol"
)

type SessionState struct {
	InTransaction bool
	Dirty         bool // Has session-level state (temp tables, settings, etc.)
	LastUsed      time.Time
	CreatedAt     time.Time
	SessionCount  int // Number of times this connection was reused
}

type Connection struct {
	conn         net.Conn
	config       Config
	sessionState SessionState
	mu           sync.RWMutex
}

type Config struct {
	Host     string
	Port     string
	Database string
	User     string
	Password string
	TLS      TLSConfig

	ConnectTimeout time.Duration
}

type TLSConfig struct {
	Enabled            bool
	CAFile             string
	InsecureSkipVerify bool
	MinVersion         string
}

func NewConnection(ctx context.Context, config Config) (*Connection, error) {
	if config.ConnectTimeout == 0 {
		config.ConnectTimeout = 10 * time.Second
	}

	dialer := &net.Dialer{
		Timeout: config.ConnectTimeout,
	}

	addr := net.JoinHostPort(config.Host, config.Port)
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to backend %s: %w", addr, err)
	}

	if config.TLS.Enabled {
		sslRequest := []byte{0, 0, 0, 8, 0x04, 0xd2, 0x16, 0x2f}
		if _, err := conn.Write(sslRequest); err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to send SSL request: %w", err)
		}

		response := make([]byte, 1)
		if _, err := conn.Read(response); err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to read SSL response: %w", err)
		}

		if response[0] != 'S' {
			conn.Close()
			return nil, fmt.Errorf("backend does not support SSL")
		}

		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: config.Host,
		}

		if config.TLS.MinVersion == "1.3" {
			tlsConfig.MinVersion = tls.VersionTLS13
		}

		if config.TLS.CAFile != "" {
			caCert, err := os.ReadFile(config.TLS.CAFile)
			if err != nil {
				conn.Close()
				return nil, fmt.Errorf("failed to read CA file: %w", err)
			}
			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				conn.Close()
				return nil, fmt.Errorf("failed to parse CA certificate")
			}
			tlsConfig.RootCAs = caCertPool
		}

		if config.TLS.InsecureSkipVerify {
			tlsConfig.InsecureSkipVerify = true
		}

		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			return nil, fmt.Errorf("TLS handshake with backend failed: %w", err)
		}
		conn = tlsConn
	}

	bc := &Connection{
		conn:   conn,
		config: config,
		sessionState: SessionState{
			CreatedAt: time.Now(),
			LastUsed:  time.Now(),
		},
	}

	if err := bc.startup(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("startup failed: %w", err)
	}

	return bc, nil
}

func (c *Connection) startup() error {
	params := map[string]string{
		"user":     c.config.User,
		"database": c.config.Database,
	}

	startupData := protocol.EncodeStartupMessage(params)
	startupMsg := &protocol.Message{
		Type: protocol.MsgStartup,
		Data: startupData,
	}

	if err := c.writeStartupMessage(startupMsg); err != nil {
		return fmt.Errorf("failed to write startup: %w", err)
	}

	for {
		msg, err := protocol.ReadMessage(c.conn)
		if err != nil {
			return fmt.Errorf("failed to read auth response: %w", err)
		}

		switch msg.Type {
		case protocol.MsgAuthRequest:
			authMsg, err := protocol.ParseAuthenticationMessage(msg.Data)
			if err != nil {
				return fmt.Errorf("failed to parse auth: %w", err)
			}

			if authMsg.Type == protocol.AuthOK {
				continue
			} else if authMsg.Type == protocol.AuthCleartextPassword {
				passwordMsg := &protocol.Message{
					Type: protocol.MsgPassword,
					Data: append([]byte(c.config.Password), 0),
				}
				if err := protocol.WriteMessage(c.conn, passwordMsg); err != nil {
					return fmt.Errorf("failed to send password: %w", err)
				}
			} else if authMsg.Type == protocol.AuthMD5Password {
				if len(authMsg.Data) < 4 {
					return fmt.Errorf("invalid MD5 salt length")
				}
				var salt [4]byte
				copy(salt[:], authMsg.Data[:4])

				md5Pass := protocol.EncodeMD5Password(c.config.User, c.config.Password, salt)
				passwordMsg := &protocol.Message{
					Type: protocol.MsgPassword,
					Data: md5Pass,
				}
				if err := protocol.WriteMessage(c.conn, passwordMsg); err != nil {
					return fmt.Errorf("failed to send MD5 password: %w", err)
				}
			} else {
				return fmt.Errorf("unsupported auth type: %d", authMsg.Type)
			}

		case protocol.MsgParameterStatus:
			continue

		case protocol.MsgBackendKeyData:
			continue

		case protocol.MsgReadyForQuery:
			return nil

		case protocol.MsgErrorResponse:
			return fmt.Errorf("backend error: %s", string(msg.Data))

		default:
			continue
		}
	}
}

func (c *Connection) writeStartupMessage(msg *protocol.Message) error {
	length := uint32(len(msg.Data) + 4)
	lenBuf := make([]byte, 4)
	lenBuf[0] = byte(length >> 24)
	lenBuf[1] = byte(length >> 16)
	lenBuf[2] = byte(length >> 8)
	lenBuf[3] = byte(length)

	if _, err := c.conn.Write(lenBuf); err != nil {
		return err
	}
	if _, err := c.conn.Write(msg.Data); err != nil {
		return err
	}
	return nil
}

func (c *Connection) Conn() net.Conn {
	return c.conn
}

func (c *Connection) Close() error {
	terminateMsg := &protocol.Message{
		Type: protocol.MsgTerminate,
		Data: []byte{},
	}
	protocol.WriteMessage(c.conn, terminateMsg)

	return c.conn.Close()
}

func (c *Connection) ReadMessage() (*protocol.Message, error) {
	return protocol.ReadMessage(c.conn)
}

func (c *Connection) WriteMessage(msg *protocol.Message) error {
	return protocol.WriteMessage(c.conn, msg)
}

// ResetSession resets the connection state for session pooling
func (c *Connection) ResetSession(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If we're in a transaction, roll it back
	if c.sessionState.InTransaction {
		if err := c.executeSimpleQuery("ROLLBACK"); err != nil {
			return fmt.Errorf("failed to rollback transaction: %w", err)
		}
		c.sessionState.InTransaction = false
	}

	// If the session is dirty (has session-level state), perform DISCARD ALL
	if c.sessionState.Dirty {
		if err := c.executeSimpleQuery("DISCARD ALL"); err != nil {
			return fmt.Errorf("failed to discard session state: %w", err)
		}
		c.sessionState.Dirty = false
	}

	c.sessionState.LastUsed = time.Now()
	c.sessionState.SessionCount++

	return nil
}

// executeSimpleQuery sends a simple query and waits for completion
func (c *Connection) executeSimpleQuery(query string) error {
	queryMsg := &protocol.Message{
		Type: protocol.MsgQuery,
		Data: append([]byte(query), 0),
	}

	if err := protocol.WriteMessage(c.conn, queryMsg); err != nil {
		return err
	}

	// Read messages until ReadyForQuery
	for {
		msg, err := protocol.ReadMessage(c.conn)
		if err != nil {
			return err
		}

		switch msg.Type {
		case protocol.MsgReadyForQuery:
			return nil
		case protocol.MsgErrorResponse:
			return fmt.Errorf("query error: %s", string(msg.Data))
		default:
			// Continue reading other messages (CommandComplete, etc.)
			continue
		}
	}
}

// IsHealthy checks if the connection is still healthy
func (c *Connection) IsHealthy(ctx context.Context) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check if connection is still alive
	c.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	defer c.conn.SetReadDeadline(time.Time{})

	// Try to peek at the connection
	one := make([]byte, 1)
	_, err := c.conn.Read(one)
	if err == nil {
		// We read something, which shouldn't happen in idle state
		return false
	}

	// Check if it's a timeout (expected for healthy idle connection)
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}

	// Any other error means connection is broken
	return false
}

// GetSessionState returns a copy of the session state
func (c *Connection) GetSessionState() SessionState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionState
}

// MarkDirty marks the session as having session-level state
func (c *Connection) MarkDirty() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionState.Dirty = true
}

// MarkInTransaction marks the session as being in a transaction
func (c *Connection) MarkInTransaction(inTx bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionState.InTransaction = inTx
}

// UpdateLastUsed updates the last used timestamp
func (c *Connection) UpdateLastUsed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionState.LastUsed = time.Now()
}
