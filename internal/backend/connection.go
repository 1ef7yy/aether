package backend

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/1ef7yy/aether/internal/protocol"
)

type Connection struct {
	conn   net.Conn
	config Config
}

type Config struct {
	Host     string
	Port     string
	Database string
	User     string
	Password string

	ConnectTimeout time.Duration
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

	bc := &Connection{
		conn:   conn,
		config: config,
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
