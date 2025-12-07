package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/1ef7yy/aether/internal/backend"
	"github.com/1ef7yy/aether/internal/pool"
	"github.com/1ef7yy/aether/internal/protocol"
)

type Server struct {
	listener net.Listener
	pool     *pool.Pool
	config   Config
	mu       sync.RWMutex
	clients  map[*Client]struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

type Config struct {
	ListenAddr string
	Pool       pool.Config
	TLS        TLSConfig
}

type TLSConfig struct {
	Enabled    bool
	CertFile   string
	KeyFile    string
	CAFile     string
	MinVersion string
}

func NewServer(config Config) (*Server, error) {
	p, err := pool.NewPool(config.Pool)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		pool:    p,
		config:  config,
		clients: make(map[*Client]struct{}),
		ctx:     ctx,
		cancel:  cancel,
	}, nil
}

func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.config.ListenAddr, err)
	}

	if s.config.TLS.Enabled {
		log.Printf("Aether listening on %s (TLS available)", s.config.ListenAddr)
	} else {
		log.Printf("Aether listening on %s", s.config.ListenAddr)
	}

	s.listener = listener

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				log.Printf("Error accepting connection: %v", err)
				continue
			}
		}

		client := NewClient(conn, s.pool, &s.config)

		s.mu.Lock()
		s.clients[client] = struct{}{}
		s.mu.Unlock()

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() {
				s.mu.Lock()
				delete(s.clients, client)
				s.mu.Unlock()
			}()

			if err := client.Handle(s.ctx); err != nil {
				log.Printf("Client error: %v", err)
			}
		}()
	}
}

func (s *Server) Shutdown(timeout time.Duration) error {
	log.Println("Shutting down server...")

	if s.listener != nil {
		s.listener.Close()
	}

	s.cancel()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All clients disconnected gracefully")
	case <-time.After(timeout):
		log.Println("Shutdown timeout reached, forcing close")

		s.mu.Lock()
		for client := range s.clients {
			client.Close()
		}
		s.mu.Unlock()
	}

	if err := s.pool.Close(); err != nil {
		return fmt.Errorf("error closing pool: %w", err)
	}

	log.Println("Server shutdown complete")
	return nil
}

type Client struct {
	conn        net.Conn
	pool        *pool.Pool
	serverCfg   *Config
	backendConn *backend.Connection
	mu          sync.Mutex
	closed      bool
}

func NewClient(conn net.Conn, pool *pool.Pool, serverCfg *Config) *Client {
	return &Client{
		conn:      conn,
		pool:      pool,
		serverCfg: serverCfg,
	}
}

func (c *Client) Handle(ctx context.Context) error {
	defer c.Close()

	startupMsg, err := protocol.ReadStartupMessage(c.conn)
	if err != nil {
		return fmt.Errorf("failed to read startup message: %w", err)
	}

	startup, err := protocol.ParseStartupMessage(startupMsg.Data)
	if err != nil {
		return fmt.Errorf("failed to parse startup message: %w", err)
	}

	if protocol.IsSSLRequest(startup.ProtocolVersion) {
		if c.serverCfg.TLS.Enabled {
			if _, err := c.conn.Write([]byte{'S'}); err != nil {
				return fmt.Errorf("failed to write SSL accept: %w", err)
			}

			tlsConfig, err := c.serverCfg.LoadTLSConfig()
			if err != nil {
				return fmt.Errorf("failed to load TLS config: %w", err)
			}

			tlsConn := tls.Server(c.conn, tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return fmt.Errorf("TLS handshake failed: %w", err)
			}
			c.conn = tlsConn
		} else {
			if _, err := c.conn.Write([]byte{'N'}); err != nil {
				return fmt.Errorf("failed to write SSL response: %w", err)
			}
		}

		startupMsg, err = protocol.ReadStartupMessage(c.conn)
		if err != nil {
			return fmt.Errorf("failed to read actual startup message: %w", err)
		}

		startup, err = protocol.ParseStartupMessage(startupMsg.Data)
		if err != nil {
			return fmt.Errorf("failed to parse actual startup message: %w", err)
		}
	}

	if protocol.IsCancelRequest(startup.ProtocolVersion) {
		log.Println("Received cancel request")
		return nil
	}

	log.Printf("Client connected: user=%s, database=%s",
		startup.Parameters["user"],
		startup.Parameters["database"])

	backendConn, err := c.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire backend connection: %w", err)
	}
	c.backendConn = backendConn

	if err := c.forwardStartupMessages(); err != nil {
		return fmt.Errorf("failed to forward startup messages: %w", err)
	}

	log.Printf("Client authenticated and ready")

	errChan := make(chan error, 2)

	go func() {
		errChan <- c.clientToBackend(ctx)
	}()

	go func() {
		errChan <- c.backendToClient(ctx)
	}()

	err = <-errChan
	return err
}

func (c *Client) forwardStartupMessages() error {
	authOK := &protocol.Message{
		Type: protocol.MsgAuthRequest,
		Data: protocol.EncodeAuthenticationOK(),
	}
	if err := protocol.WriteMessage(c.conn, authOK); err != nil {
		return fmt.Errorf("failed to write auth OK: %w", err)
	}

	params := []struct{ name, value string }{
		{"server_version", "14.0"},
		{"server_encoding", "UTF8"},
		{"client_encoding", "UTF8"},
		{"DateStyle", "ISO, MDY"},
		{"TimeZone", "UTC"},
	}

	for _, param := range params {
		msg := &protocol.Message{
			Type: protocol.MsgParameterStatus,
			Data: append(append([]byte(param.name), 0), append([]byte(param.value), 0)...),
		}
		if err := protocol.WriteMessage(c.conn, msg); err != nil {
			return err
		}
	}

	readyMsg := &protocol.Message{
		Type: protocol.MsgReadyForQuery,
		Data: []byte{'I'},
	}
	if err := protocol.WriteMessage(c.conn, readyMsg); err != nil {
		return fmt.Errorf("failed to write ready: %w", err)
	}

	return nil
}

func (c *Client) clientToBackend(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		msg, err := protocol.ReadMessage(c.conn)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("client read error: %w", err)
		}

		if msg.Type == protocol.MsgTerminate {
			return nil
		}

		if err := c.backendConn.WriteMessage(msg); err != nil {
			return fmt.Errorf("backend write error: %w", err)
		}
	}
}

func (c *Client) backendToClient(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		msg, err := c.backendConn.ReadMessage()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("backend read error: %w", err)
		}

		if err := protocol.WriteMessage(c.conn, msg); err != nil {
			return fmt.Errorf("client write error: %w", err)
		}
	}
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true

	if c.backendConn != nil {
		c.pool.Release(c.backendConn)
		c.backendConn = nil
	}

	return c.conn.Close()
}
