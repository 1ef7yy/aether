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

	// Try to get an idle connection first
	p.mu.Lock()
	if len(p.idle) > 0 {
		conn := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]
		p.mu.Unlock()

		// Reset connection based on pool mode
		if p.config.Mode == PoolModeSession {
			if err := conn.ResetSession(acquireCtx); err != nil {
				// If reset fails, close this connection and create a new one
				conn.Close()
				p.mu.Lock()
				p.totalConnections--
				p.mu.Unlock()
				return p.createNewConnection(acquireCtx)
			}
		} else if p.config.Mode == PoolModeTransaction {
			if err := conn.ResetTransaction(acquireCtx); err != nil {
				// If reset fails, close this connection and create a new one
				conn.Close()
				p.mu.Lock()
				p.totalConnections--
				p.mu.Unlock()
				return p.createNewConnection(acquireCtx)
			}
		}

		// Verify connection is still healthy
		if !conn.IsHealthy(acquireCtx) {
			conn.Close()
			p.mu.Lock()
			p.totalConnections--
			p.mu.Unlock()
			return p.Acquire(ctx) // Retry with a new connection
		}

		p.mu.Lock()
		p.active[conn] = struct{}{}
		p.activeConnections++
		p.mu.Unlock()

		conn.UpdateLastUsed()
		return conn, nil
	}

	// Check if we can create a new connection
	if p.totalConnections >= p.config.MaxConnections {
		p.mu.Unlock()
		// Wait for a released connection
		select {
		case conn := <-p.connChan:
			p.mu.Lock()
			p.active[conn] = struct{}{}
			p.activeConnections++
			p.mu.Unlock()

			// Reset connection state based on pool mode
			if p.config.Mode == PoolModeSession {
				if err := conn.ResetSession(acquireCtx); err != nil {
					conn.Close()
					p.mu.Lock()
					p.totalConnections--
					p.mu.Unlock()
					return p.Acquire(ctx)
				}
			} else if p.config.Mode == PoolModeTransaction {
				if err := conn.ResetTransaction(acquireCtx); err != nil {
					conn.Close()
					p.mu.Lock()
					p.totalConnections--
					p.mu.Unlock()
					return p.Acquire(ctx)
				}
			}

			conn.UpdateLastUsed()
			return conn, nil
		case <-acquireCtx.Done():
			return nil, fmt.Errorf("timeout waiting for connection")
		}
	}

	p.totalConnections++
	p.mu.Unlock()

	return p.createNewConnection(acquireCtx)
}

// createNewConnection creates a new backend connection
func (p *Pool) createNewConnection(ctx context.Context) (*backend.Connection, error) {
	conn, err := backend.NewConnection(ctx, p.config.Backend)
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

	// Check connection age and health
	state := conn.GetSessionState()
	age := time.Since(state.CreatedAt)

	// If connection exceeded max lifetime, close it
	if age > p.config.MaxLifetime {
		p.totalConnections--
		return conn.Close()
	}

	// Handle connection state based on pool mode
	if p.config.Mode == PoolModeSession {
		// If connection is in a bad state, close it
		if state.InTransaction {
			p.totalConnections--
			return conn.Close()
		}
	} else if p.config.Mode == PoolModeTransaction {
		// For transaction mode, rollback any active transaction
		if state.InTransaction {
			if err := conn.RollbackTransaction(); err != nil {
				// If rollback fails, close the connection
				p.totalConnections--
				return conn.Close()
			}
		}
		// Reset dirty flag for transaction mode
		if state.Dirty {
			if err := conn.ResetTransaction(context.Background()); err != nil {
				p.totalConnections--
				return conn.Close()
			}
		}
	}

	// Try to return to channel first (fast path for waiting clients)
	select {
	case p.connChan <- conn:
		return nil
	default:
		// Channel full, add to idle pool if there's room
		if len(p.idle) < p.config.MinIdleConnections {
			p.idle = append(p.idle, conn)
			return nil
		}
		// Too many idle connections, close this one
		p.totalConnections--
		return conn.Close()
	}
}

// Start begins the pool maintenance goroutine
func (p *Pool) Start() {
	go p.maintenanceLoop()
}

// maintenanceLoop periodically cleans up stale connections
func (p *Pool) maintenanceLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.performMaintenance()
		}
	}
}

// performMaintenance cleans up idle connections that have exceeded MaxIdleTime
func (p *Pool) performMaintenance() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	now := time.Now()
	validIdle := make([]*backend.Connection, 0, len(p.idle))

	for _, conn := range p.idle {
		state := conn.GetSessionState()
		idleTime := now.Sub(state.LastUsed)
		age := now.Sub(state.CreatedAt)

		// Close connections that are too old or idle for too long
		if age > p.config.MaxLifetime || idleTime > p.config.MaxIdleTime {
			conn.Close()
			p.totalConnections--
		} else {
			validIdle = append(validIdle, conn)
		}
	}

	p.idle = validIdle
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

	totalSessionCount := 0
	totalTransactionCount := 0
	var avgIdleTime time.Duration
	now := time.Now()

	for _, conn := range p.idle {
		state := conn.GetSessionState()
		totalSessionCount += state.SessionCount
		totalTransactionCount += state.TransactionCount
		avgIdleTime += now.Sub(state.LastUsed)
	}

	if len(p.idle) > 0 {
		avgIdleTime = avgIdleTime / time.Duration(len(p.idle))
	}

	return PoolStats{
		MaxConnections:    p.config.MaxConnections,
		ActiveConnections: p.activeConnections,
		IdleConnections:   len(p.idle),
		WaitingClients:    p.waitingClients,
		TotalConnections:  p.totalConnections,
		TotalSessions:     totalSessionCount,
		TotalTransactions: totalTransactionCount,
		AvgIdleTime:       avgIdleTime,
		Mode:              p.config.Mode,
	}
}

type PoolStats struct {
	MaxConnections    int
	ActiveConnections int
	IdleConnections   int
	WaitingClients    int
	TotalConnections  int
	TotalSessions     int
	TotalTransactions int
	AvgIdleTime       time.Duration
	Mode              PoolMode
}
