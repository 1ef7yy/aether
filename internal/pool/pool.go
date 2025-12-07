package pool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/1ef7yy/aether/internal/backend"
)

type PoolMode string

const (
	PoolModeSession     PoolMode = "session"
	PoolModeTransaction PoolMode = "transaction"
	PoolModeStatement   PoolMode = "statement"
)

type Config struct {
	Backend            backend.Config
	Mode               PoolMode
	MaxConnections     int
	MinIdleConnections int
	MaxIdleTime        time.Duration
	MaxLifetime        time.Duration
	AcquireTimeout     time.Duration
}

type Pool struct {
	config            Config
	mu                sync.RWMutex
	idle              []*backend.Connection
	active            map[*backend.Connection]struct{}
	connChan          chan *backend.Connection
	totalConnections  int
	activeConnections int
	waitingClients    int
	ctx               context.Context
	cancel            context.CancelFunc
	closed            bool
}

func (p *Pool) GetConfig() Config {
	return p.config
}

func NewPool(config Config) (*Pool, error) {
	if config.MaxConnections == 0 {
		config.MaxConnections = 100
	}
	if config.MinIdleConnections == 0 {
		config.MinIdleConnections = 10
	}
	if config.MaxIdleTime == 0 {
		config.MaxIdleTime = 10 * time.Minute
	}
	if config.MaxLifetime == 0 {
		config.MaxLifetime = 1 * time.Hour
	}
	if config.AcquireTimeout == 0 {
		config.AcquireTimeout = 30 * time.Second
	}
	if config.Mode == "" {
		config.Mode = PoolModeSession
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &Pool{
		config:   config,
		idle:     make([]*backend.Connection, 0, config.MinIdleConnections),
		active:   make(map[*backend.Connection]struct{}),
		connChan: make(chan *backend.Connection, config.MaxConnections),
		ctx:      ctx,
		cancel:   cancel,
	}

	return pool, nil
}

func (p *Pool) Acquire(ctx context.Context) (*backend.Connection, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, fmt.Errorf("pool is closed")
	}
	p.waitingClients++
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.waitingClients--
		p.mu.Unlock()
	}()

	acquireCtx, cancel := context.WithTimeout(ctx, p.config.AcquireTimeout)
	defer cancel()

	p.mu.Lock()
	if len(p.idle) > 0 {
		conn := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]
		p.active[conn] = struct{}{}
		p.activeConnections++
		p.mu.Unlock()
		return conn, nil
	}

	if p.totalConnections >= p.config.MaxConnections {
		p.mu.Unlock()
		select {
		case conn := <-p.connChan:
			p.mu.Lock()
			p.active[conn] = struct{}{}
			p.activeConnections++
			p.mu.Unlock()
			return conn, nil
		case <-acquireCtx.Done():
			return nil, fmt.Errorf("timeout waiting for connection")
		}
	}

	p.totalConnections++
	p.mu.Unlock()

	conn, err := backend.NewConnection(acquireCtx, p.config.Backend)
	if err != nil {
		p.mu.Lock()
		p.totalConnections--
		p.mu.Unlock()
		return nil, fmt.Errorf("failed to create connection: %w", err)
	}

	p.mu.Lock()
	p.active[conn] = struct{}{}
	p.activeConnections++
	p.mu.Unlock()

	return conn, nil
}

func (p *Pool) Release(conn *backend.Connection) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return conn.Close()
	}

	delete(p.active, conn)
	p.activeConnections--

	select {
	case p.connChan <- conn:
		return nil
	default:
		if len(p.idle) < p.config.MinIdleConnections {
			p.idle = append(p.idle, conn)
			return nil
		}
		p.totalConnections--
		return conn.Close()
	}
}

func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	p.cancel()

	for _, conn := range p.idle {
		conn.Close()
	}
	p.idle = nil

	for conn := range p.active {
		conn.Close()
	}
	p.active = nil

	close(p.connChan)

	return nil
}

func (p *Pool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return PoolStats{
		MaxConnections:    p.config.MaxConnections,
		ActiveConnections: p.activeConnections,
		IdleConnections:   len(p.idle),
		WaitingClients:    p.waitingClients,
		TotalConnections:  p.totalConnections,
	}
}

type PoolStats struct {
	MaxConnections    int
	ActiveConnections int
	IdleConnections   int
	WaitingClients    int
	TotalConnections  int
}
