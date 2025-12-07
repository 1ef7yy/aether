# Transaction Pooling

Transaction pooling is a connection pooling mode where connections are returned to the pool after each transaction completes. This mode is ideal for applications that execute many short transactions and want to maximize connection reuse while maintaining session-level state like prepared statements.

## Overview

Transaction pooling sits between session pooling and statement pooling in terms of connection reuse:

- **Session Pooling**: Connections are tied to client sessions for their entire lifetime
- **Transaction Pooling**: Connections are returned to the pool after each transaction
- **Statement Pooling**: Connections are returned after each statement (not yet implemented)

## How It Works

1. **Client Connects**: A client connection to Aether is established
2. **Connection Acquisition**: When the client starts a transaction, Aether acquires a backend connection from the pool
3. **Transaction Execution**: All queries within the transaction use the same backend connection
4. **Transaction Completion**: When the transaction commits or rolls back, the backend connection is returned to the pool
5. **Session State Preserved**: Prepared statements and other session-level objects persist across transactions

## Key Features

### Automatic Transaction Management

- **Auto-Rollback**: If a connection is returned while in a transaction, it's automatically rolled back
- **Transaction Tracking**: The pool tracks which connections are in active transactions
- **State Validation**: Connections are validated before being reused

### Session State Persistence

Unlike session pooling, transaction pooling preserves session-level state:

- **Prepared Statements**: Prepared statements persist across transactions
- **Temporary Tables**: Session-scoped temporary tables remain available
- **Connection Settings**: SET commands persist for the connection lifetime
- **Search Path**: Schema search path is maintained

### Connection Lifecycle

```
┌──────────────┐
│   Idle Pool  │
└──────┬───────┘
       │
       ↓ Acquire (BEGIN detected)
┌──────────────┐
│   In Use     │
└──────┬───────┘
       │
       ↓ Release (COMMIT/ROLLBACK)
┌──────────────┐
│ Reset Check  │ ← Rollback if in transaction
└──────┬───────┘
       │
       ↓ Return to pool
┌──────────────┐
│   Idle Pool  │
└──────────────┘
```

## Configuration

Enable transaction pooling in your `aether.yaml`:

```yaml
pool:
  mode: transaction
  max_connections: 100
  min_idle_connections: 10
  max_idle_time: 10m
  max_lifetime: 1h
  acquire_timeout: 30s

  backend:
    host: localhost
    port: "5432"
    database: mydb
    user: myuser
    password: mypassword
```

## Usage Example

### Basic Transaction

```sql
-- Client starts transaction
BEGIN;

-- Aether acquires a backend connection from the pool
-- and forwards the BEGIN command

-- Execute queries
INSERT INTO users (name, email) VALUES ('John', 'john@example.com');
UPDATE accounts SET balance = balance - 100 WHERE user_id = 1;

-- Commit the transaction
COMMIT;

-- Connection is automatically returned to the pool
-- and becomes available for other transactions
```

### With Prepared Statements

```sql
-- Prepare a statement (persists at connection level)
PREPARE get_user AS SELECT * FROM users WHERE id = $1;

-- First transaction
BEGIN;
EXECUTE get_user(1);
COMMIT;

-- Connection returns to pool

-- Second transaction might get the same connection
BEGIN;
EXECUTE get_user(2);  -- Prepared statement still available!
COMMIT;
```

### Error Handling

```sql
BEGIN;
INSERT INTO users (name) VALUES ('Alice');
-- Something goes wrong
-- Client disconnects or doesn't commit

-- Aether automatically rolls back the transaction
-- when the connection is returned to the pool
```

## Performance Characteristics

### Benefits

1. **High Connection Reuse**: Multiple clients can share the same backend connections
2. **Reduced Connection Overhead**: Fewer connections to manage than session pooling
3. **Session State Persistence**: Prepared statements improve query performance
4. **Lower Memory Usage**: Fewer total connections needed
5. **Better Scalability**: Support more concurrent clients with fewer backend connections

### Trade-offs

1. **Transaction State Required**: Clients must properly start/end transactions
2. **Potential Blocking**: Long transactions can hold connections longer
3. **Statement Leakage**: Prepared statements accumulate over connection lifetime
4. **Temporary Objects**: Session-scoped temp tables persist (may cause naming conflicts)

## Metrics and Monitoring

The pool provides statistics for monitoring:

```go
stats := pool.Stats()
fmt.Printf("Mode: %s\n", stats.Mode)
fmt.Printf("Active: %d/%d\n", stats.ActiveConnections, stats.MaxConnections)
fmt.Printf("Idle: %d\n", stats.IdleConnections)
fmt.Printf("Waiting: %d\n", stats.WaitingClients)
fmt.Printf("Total Sessions: %d\n", stats.TotalSessions)
fmt.Printf("Total Transactions: %d\n", stats.TotalTransactions)
fmt.Printf("Avg Idle Time: %s\n", stats.AvgIdleTime)
```

### Key Metrics

- **TotalTransactions**: Number of transactions executed across all pooled connections
- **Transactions per Connection**: `TotalTransactions / TotalConnections`
- **WaitingClients**: Number of clients waiting for a connection (should be low)
- **AvgIdleTime**: How long connections sit idle (tune `min_idle_connections`)

## Best Practices

### 1. Always Use Explicit Transactions

```sql
-- Good: Explicit transaction boundaries
BEGIN;
INSERT INTO table1 VALUES (1);
UPDATE table2 SET col = val;
COMMIT;

-- Avoid: Auto-commit mode (creates implicit transactions)
INSERT INTO table1 VALUES (1);
```

### 2. Keep Transactions Short

```sql
-- Good: Quick transaction
BEGIN;
UPDATE inventory SET quantity = quantity - 1 WHERE id = 123;
COMMIT;

-- Avoid: Long-running transaction holding connection
BEGIN;
SELECT pg_sleep(60);  -- Don't do this!
COMMIT;
```

### 3. Clean Up Prepared Statements

```sql
-- Prepare statement
PREPARE my_query AS SELECT * FROM users WHERE status = $1;

-- Use it
EXECUTE my_query('active');

-- Clean up if no longer needed
DEALLOCATE my_query;
```

### 4. Handle Temporary Tables Carefully

```sql
-- If using session-scoped temp tables, use unique names
CREATE TEMP TABLE temp_data_12345 (id INT, data TEXT);

-- Or use ON COMMIT DROP for transaction-scoped tables
CREATE TEMP TABLE temp_data (id INT, data TEXT) ON COMMIT DROP;
```

### 5. Monitor Connection Utilization

Set appropriate pool sizes based on your workload:

```yaml
pool:
  # Maximum connections to backend
  max_connections: 100
  
  # Keep some connections ready
  min_idle_connections: 10
  
  # Close idle connections after this time
  max_idle_time: 10m
  
  # Recycle connections periodically
  max_lifetime: 1h
```

## Troubleshooting

### Connections Not Being Returned

**Symptom**: `WaitingClients` keeps growing

**Causes**:
- Transactions not being committed/rolled back
- Client connections dropping without cleanup
- Long-running transactions

**Solution**:
- Ensure all code paths commit or rollback transactions
- Set `statement_timeout` in PostgreSQL
- Monitor and kill long-running transactions

### Prepared Statement Errors

**Symptom**: "prepared statement XYZ already exists"

**Causes**:
- Multiple clients using same prepared statement names
- Statement names not being cleaned up

**Solution**:
- Use unique prepared statement names per client
- DEALLOCATE statements when done
- Consider using session pooling instead for heavy prepared statement usage

### Performance Degradation Over Time

**Symptom**: Queries getting slower

**Causes**:
- Too many prepared statements accumulating
- Connection memory usage growing
- Table/index bloat from temp tables

**Solution**:
- Lower `max_lifetime` to recycle connections more frequently
- Monitor prepared statement count: `SELECT count(*) FROM pg_prepared_statements`
- Use `DISCARD ALL` periodically (switches to session-like behavior)

## Comparison with Other Modes

| Feature | Session | Transaction | Statement |
|---------|---------|-------------|-----------|
| Connection Reuse | Low | High | Very High |
| Session State | Fully Preserved | Partially Preserved | Not Preserved |
| Prepared Statements | Persists | Persists | Lost |
| Temporary Tables | Safe | Requires Care | Not Available |
| Transaction Isolation | Complete | Complete | N/A |
| Best For | Few clients | Many concurrent transactions | Read-heavy workloads |
| Memory Usage | Highest | Medium | Lowest |
| Complexity | Lowest | Medium | Highest |

## Implementation Details

### Transaction Detection

Aether detects transactions by monitoring PostgreSQL protocol messages:

- **BEGIN/START TRANSACTION**: Marks connection as in-transaction
- **COMMIT**: Marks transaction complete
- **ROLLBACK**: Marks transaction complete
- **ReadyForQuery**: Contains transaction status indicator

### Connection Reset

When a connection is returned to the pool in transaction mode:

1. Check if `InTransaction` flag is set
2. If yes, send `ROLLBACK` command
3. Wait for `ReadyForQuery` response
4. Clear `InTransaction` flag
5. Update transaction counter
6. Return to idle pool

### Health Checking

Connections are validated before reuse:

1. Check connection age against `max_lifetime`
2. Verify TCP connection is alive
3. Ensure transaction state is clean
4. Validate no protocol errors occurred

## Future Enhancements

- [ ] Automatic prepared statement cleanup after N uses
- [ ] Per-user connection limits
- [ ] Transaction timeout enforcement
- [ ] Statement-level pooling mode
- [ ] Connection pool partitioning by database/user
- [ ] Query-based routing (read/write splitting)
