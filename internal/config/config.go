package config

import (
	"fmt"
	"os"
	"time"

	"github.com/1ef7yy/aether/internal/pool"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server ServerConfig `yaml:"server"`
	Pool   PoolConfig   `yaml:"pool"`
}

type ServerConfig struct {
	ListenAddr      string        `yaml:"listen_addr"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type PoolConfig struct {
	Backend            BackendConfig `yaml:"backend"`
	Mode               pool.PoolMode `yaml:"mode"`
	MaxConnections     int           `yaml:"max_connections"`
	MinIdleConnections int           `yaml:"min_idle_connections"`
	MaxIdleTime        time.Duration `yaml:"max_idle_time"`
	MaxLifetime        time.Duration `yaml:"max_lifetime"`
	AcquireTimeout     time.Duration `yaml:"acquire_timeout"`
}

type BackendConfig struct {
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	Database string `yaml:"database"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := config.setDefaults(); err != nil {
		return nil, err
	}

	return &config, nil
}

func (c *Config) setDefaults() error {
	if c.Server.ListenAddr == "" {
		c.Server.ListenAddr = "0.0.0.0:6432"
	}
	if c.Server.ShutdownTimeout == 0 {
		c.Server.ShutdownTimeout = 30 * time.Second
	}

	if c.Pool.Mode == "" {
		c.Pool.Mode = pool.PoolModeSession
	}
	if c.Pool.MaxConnections == 0 {
		c.Pool.MaxConnections = 100
	}
	if c.Pool.MinIdleConnections == 0 {
		c.Pool.MinIdleConnections = 10
	}
	if c.Pool.MaxIdleTime == 0 {
		c.Pool.MaxIdleTime = 10 * time.Minute
	}
	if c.Pool.MaxLifetime == 0 {
		c.Pool.MaxLifetime = 1 * time.Hour
	}
	if c.Pool.AcquireTimeout == 0 {
		c.Pool.AcquireTimeout = 30 * time.Second
	}

	if c.Pool.Backend.Host == "" {
		return fmt.Errorf("pool.backend.host is required")
	}
	if c.Pool.Backend.Port == "" {
		c.Pool.Backend.Port = "5432"
	}
	if c.Pool.Backend.Database == "" {
		return fmt.Errorf("pool.backend.database is required")
	}
	if c.Pool.Backend.User == "" {
		return fmt.Errorf("pool.backend.user is required")
	}

	return nil
}

func Default() *Config {
	config := &Config{
		Server: ServerConfig{
			ListenAddr:      "0.0.0.0:6432",
			ShutdownTimeout: 30 * time.Second,
		},
		Pool: PoolConfig{
			Mode:               pool.PoolModeSession,
			MaxConnections:     100,
			MinIdleConnections: 10,
			MaxIdleTime:        10 * time.Minute,
			MaxLifetime:        1 * time.Hour,
			AcquireTimeout:     30 * time.Second,
		},
	}
	return config
}
