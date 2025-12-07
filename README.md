# Aether

A high-performance PostgreSQL connection pooler written in Go, inspired by PgBouncer.

## Features

- **Multiple Pooling Modes**
  - **Session pooling**: One server connection per client connection with automatic state reset
  - Transaction pooling: Connection returned to pool after transaction
  - Statement pooling: Connection returned after each statement

- **Advanced Session Pooling** ⭐
  - Automatic connection state reset between sessions
  - Transaction rollback and session cleanup (`DISCARD ALL`)
  - Connection health checks and lifecycle management
  - Query state tracking (transactions, temp tables, prepared statements)
  - Background maintenance and stale connection cleanup
  - Detailed pool statistics and monitoring
  - See [SESSION_POOLING.md](SESSION_POOLING.md) for details

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
  dsn: "postgres://username:password@localhost:5432/database?sslmode=disable"
  mode: "session"
  max_connections: 100
  min_idle_connections: 10
  max_idle_time: 10m
  max_lifetime: 1h
  acquire_timeout: 30s
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
- [x] **Session pooling** ⭐ (with automatic state reset)
- [ ] Transaction pooling
- [x] SSL/TLS support
- [x] Connection health checks
- [x] Automatic connection lifecycle management
- [x] Pool statistics and monitoring
- [ ] Prepared statement pooling
- [ ] Prometheus/OpenTelemetry metrics
- [ ] Admin interface

## Documentation

- [Session Pooling Guide](SESSION_POOLING.md) - Comprehensive guide to session pooling features
- [Configuration Examples](aether.yaml) - Sample configuration file