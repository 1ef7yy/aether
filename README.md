# Aether

A high-performance PostgreSQL connection pooler written in Go, inspired by PgBouncer.

## Features

- **Multiple Pooling Modes**
  - Session pooling: One server connection per client connection
  - Transaction pooling: Connection returned to pool after transaction
  - Statement pooling: Connection returned after each statement

- **Efficient Connection Management**
  - Configurable pool size and connection limits
  - Automatic connection recycling based on idle time and lifetime
  - Connection health monitoring

- **PostgreSQL Protocol Support**
  - Native PostgreSQL wire protocol implementation
  - Authentication handling
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


## Features

- [x] Query forwarding
- [ ] Session pooling
- [ ] Transaction pooling
- [ ] SSL
- [ ] TLS encryption
- [ ] Prepared statements
- [ ] Prometheus/Otel metrics
- [ ] Admin interface