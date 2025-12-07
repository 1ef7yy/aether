# Aether

A high-performance PostgreSQL connection pooler written in Go, inspired by PgBouncer.

## Features

- **Multiple Pooling Modes**
  - **Session pooling**: One server connection per client connection with automatic state reset
  - **Transaction pooling**: Connection returned to pool after transaction with session state preservation ⭐
  - Statement pooling: Connection returned after each statement (coming soon)

- **Advanced Session Pooling**
  - Automatic connection state reset between sessions
  - Transaction rollback and session cleanup (`DISCARD ALL`)
  - Connection health checks and lifecycle management
  - Query state tracking (transactions, temp tables, prepared statements)
  - Background maintenance and stale connection cleanup
  - Detailed pool statistics and monitoring
  - See [docs/SESSION_POOLING.md](docs/SESSION_POOLING.md) for details

- **Transaction Pooling** ⭐
  - Automatic transaction detection and management
  - Auto-rollback on connection release
  - Session state preservation (prepared statements, temp tables)
  - Higher connection reuse than session pooling
  - Transaction-level statistics tracking
  - Optimal for high-throughput transactional workloads
  - See [docs/TRANSACTION_POOLING.md](docs/TRANSACTION_POOLING.md) for details

- **Efficient Connection Management**
  - Configurable pool size and connection limits
  - Automatic connection recycling based on idle time and lifetime
  - Connection health monitoring
  - Adaptive pool sizing

- **PostgreSQL Protocol Support**
  - Native PostgreSQL wire protocol implementation
  - Authentication handling (MD5, cleartext)
  - SSL/TLS support for client and backend connections
  - Message routing between clients and backends

## Getting Started

### Prerequisites

- Go 1.24 or higher
- PostgreSQL server

### Installation

```bash
go build -o bin/aether ./cmd/aether
```

### Configuration

Create an `aether.yaml` configuration file:

```yaml
server:
  listen_addr: "0.0.0.0:6432"
  shutdown_timeout: 30s

pool:
  mode: "transaction"  # Options: session, transaction, statement
  max_connections: 100
  min_idle_connections: 10
  max_idle_time: 10m
  max_lifetime: 1h
  acquire_timeout: 30s
  
  backend:
    host: "localhost"
    port: "5432"
    database: "mydb"
    user: "myuser"
    password: "mypassword"
```

### Running

```bash
./bin/aether -config aether.yaml
```

Or with default config file location:

```bash
./bin/aether
```


## Feature Status

- [x] Query forwarding
- [x] Session pooling
- [x] Transaction pooling
- [x] SSL/TLS support
- [x] Connection health checks
- [x] Automatic connection lifecycle management
- [x] Pool statistics and monitoring
- [ ] Statement pooling
- [ ] Prepared statement pooling
- [ ] Prometheus/OpenTelemetry metrics
- [ ] Admin interface

## Documentation

- [Session Pooling Guide](docs/SESSION_POOLING.md) - Comprehensive guide to session pooling features
- [Transaction Pooling Guide](docs/TRANSACTION_POOLING.md) - Comprehensive guide to transaction pooling features
- [Configuration Examples](aether.yaml) - Sample configuration file

## Pool Mode Comparison

| Feature | Session | Transaction | Statement |
|---------|---------|-------------|-----------|
| Connection Reuse | Low | High | Very High |
| Session State | Fully Preserved | Partially Preserved | Not Preserved |
| Prepared Statements | Safe | Persists | Lost |
| Temporary Tables | Safe | Requires Care | Not Available |
| Best For | Few clients, complex sessions | Many concurrent transactions | Read-heavy workloads |
| Memory Usage | Highest | Medium | Lowest |