# Session Pooling

Aether now implements comprehensive session pooling for PostgreSQL connections. This document describes the session pooling features and how to use them.

## Overview

Session pooling reuses database connections across multiple client sessions, significantly improving performance and resource utilization. When a client disconnects, the connection is returned to the pool and can be reused by another client.

## Features

### 1. Session State Tracking

Each connection maintains detailed session state information:

- **InTransaction**: Tracks whether the connection is currently in a transaction
- **Dirty**: Indicates if the connection has session-level state (temp tables, prepared statements, SET commands)
- **LastUsed**: Timestamp of last usage
- **CreatedAt**: When the connection was created
- **SessionCount**: Number of times the connection has been reused

### 2. Automatic Connection Reset

When a connection is returned to the pool and reacquired by a new client:

1. **Transaction Rollback**: Any uncommitted transaction is automatically rolled back
2. **Session Cleanup**: `DISCARD ALL` is executed to clear:
   - Temporary tables
   - Prepared statements
   - Session variables set with SET
   - Advisory locks
   - LISTEN/NOTIFY subscriptions

### 3. Connection Health Checks

Before reusing a connection, the pool verifies:

- Connection is still alive
- No network errors
- Reset was successful

Unhealthy connections are automatically closed and replaced.

### 4. Connection Lifecycle Management

- **Max Lifetime**: Connections are closed after a configurable lifetime (default: 1 hour)
- **Max Idle Time**: Idle connections are closed after exceeding idle timeout (default: 10 minutes)
- **Automatic Maintenance**: Background routine periodically cleans up stale connections

### 5. Query State Tracking

The proxy automatically tracks queries to maintain accurate session state:

- Detects `BEGIN`, `START TRANSACTION`, `COMMIT`, `ROLLBACK`
- Detects `SET` commands (marks session as dirty)
- Detects `CREATE TEMP TABLE` (marks session as dirty)
- Detects `PREPARE` statements (marks session as dirty)

## Configuration

Configure session pooling in your `aether.yaml`:

```yaml
pool:
  mode: session              # Pool mode: session, transaction, or statement
  max_connections: 100       # Maximum connections to backend
  min_idle_connections: 10   # Minimum idle connections to maintain
  max_idle_time: 10m         # Max time a connection can be idle
  max_lifetime: 1h           # Max lifetime of a connection
  acquire_timeout: 30s       # Timeout waiting for connection
```

## Pool Modes

### Session Mode (Recommended)

```yaml
pool:
  mode: session
```

- Connections are reused across sessions
- Full connection reset between clients
- Best for most applications
- Excellent performance with safety

### Transaction Mode

```yaml
pool:
  mode: transaction
```

- Connections returned to pool after each transaction
- More aggressive pooling
- Requires careful application design

### Statement Mode

```yaml
pool:
  mode: statement
```

- Connections returned after each statement
- Most aggressive pooling
- Only for specific use cases

## Monitoring

Get pool statistics at runtime:

```go
stats := pool.Stats()
fmt.Printf("Active: %d, Idle: %d, Total Sessions: %d\n",
    stats.ActiveConnections,
    stats.IdleConnections,
    stats.TotalSessions)
```

Pool statistics include:

- `MaxConnections`: Maximum allowed connections
- `ActiveConnections`: Currently active connections
- `IdleConnections`: Available idle connections
- `WaitingClients`: Clients waiting for connections
- `TotalConnections`: Total connections in pool
- `TotalSessions`: Cumulative session count (reuse metric)
- `AvgIdleTime`: Average idle time of connections
- `Mode`: Current pool mode

## Best Practices

### 1. Choose Appropriate Limits

```yaml
pool:
  max_connections: 100        # Based on backend capacity
  min_idle_connections: 10    # Based on expected load
```

### 2. Set Reasonable Timeouts

```yaml
pool:
  max_idle_time: 10m    # Balance resource usage vs connection overhead
  max_lifetime: 1h      # Prevent connection leaks
  acquire_timeout: 30s   # Fail fast on overload
```

### 3. Monitor Pool Metrics

- Watch for high `WaitingClients` (may need more connections)
- Monitor `TotalSessions` for reuse efficiency
- Check `AvgIdleTime` to tune `min_idle_connections`

### 4. Application Considerations

- Always commit or rollback transactions
- Avoid relying on session-level state across requests
- Use connection pooling on the application side for best results

## How It Works

1. **Client Connects**: Client establishes connection to Aether proxy
2. **Connection Acquisition**: Proxy acquires backend connection from pool
3. **Session Reset**: If reusing connection, automatically reset state
4. **Health Check**: Verify connection is healthy
5. **Proxy Traffic**: Bidirectional message forwarding
6. **State Tracking**: Monitor queries for transaction/session state
7. **Client Disconnects**: Return connection to pool
8. **Maintenance**: Background cleanup of stale connections

## Performance Benefits

Session pooling provides:

- **Reduced Latency**: No connection establishment overhead
- **Better Throughput**: More efficient resource usage
- **Lower Backend Load**: Fewer connections to PostgreSQL
- **Scalability**: Handle more clients with fewer backend connections

Example: With 1000 clients and session pooling, you might only need 50-100 backend connections instead of 1000.

## Safety

Session pooling in Aether is safe because:

1. **Automatic Reset**: Connections are cleaned between sessions
2. **Transaction Rollback**: Uncommitted transactions are rolled back
3. **State Clearing**: `DISCARD ALL` removes session state
4. **Health Checks**: Broken connections are detected and replaced
5. **Lifetime Limits**: Connections are recycled periodically

## Example Configuration

Production-ready configuration:

```yaml
server:
  listen_addr: "0.0.0.0:6432"
  shutdown_timeout: 30s

pool:
  mode: session
  max_connections: 200
  min_idle_connections: 20
  max_idle_time: 15m
  max_lifetime: 2h
  acquire_timeout: 30s
  
  backend:
    host: "postgres.example.com"
    port: "5432"
    database: "mydb"
    user: "app_user"
    password: "secret"
```

## Troubleshooting

### High Wait Times

If `WaitingClients` is consistently high:

1. Increase `max_connections`
2. Check if backend can handle more connections
3. Review query performance

### Connection Errors

If seeing connection errors:

1. Check `max_lifetime` isn't too short
2. Verify network stability
3. Review backend connection limits

### Stale Data

If seeing stale session state:

1. Verify `mode: session` is set
2. Check application properly commits transactions
3. Ensure `DISCARD ALL` is working (check logs)

## Migration from Connection-per-Client

To migrate from traditional connection pooling:

1. Update configuration to use session pooling
2. Ensure application doesn't rely on session state
3. Test thoroughly in staging
4. Monitor pool statistics after deployment
5. Tune parameters based on metrics

## Advanced: Custom Reset Logic

For advanced use cases, you can modify the reset behavior in `internal/backend/connection.go`:

```go
func (c *Connection) ResetSession(ctx context.Context) error {
    // Custom reset logic here
    // Example: Additional cleanup commands
}
```
