package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/1ef7yy/aether/internal/backend"
	"github.com/1ef7yy/aether/internal/config"
	"github.com/1ef7yy/aether/internal/pool"
	"github.com/1ef7yy/aether/internal/server"
)

func main() {
	configPath := flag.String("config", "aether.yaml", "Path to configuration file")
	flag.Parse()

	cfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	serverCfg := server.Config{
		ListenAddr: cfg.Server.ListenAddr,
		Pool: pool.Config{
			Backend: backend.Config{
				Host:     cfg.Pool.Backend.Host,
				Port:     cfg.Pool.Backend.Port,
				Database: cfg.Pool.Backend.Database,
				User:     cfg.Pool.Backend.User,
				Password: cfg.Pool.Backend.Password,
			},
			Mode:               cfg.Pool.Mode,
			MaxConnections:     cfg.Pool.MaxConnections,
			MinIdleConnections: cfg.Pool.MinIdleConnections,
			MaxIdleTime:        cfg.Pool.MaxIdleTime,
			MaxLifetime:        cfg.Pool.MaxLifetime,
			AcquireTimeout:     cfg.Pool.AcquireTimeout,
		},
	}

	srv, err := server.NewServer(serverCfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Println("Received shutdown signal")

	if err := srv.Shutdown(cfg.Server.ShutdownTimeout); err != nil {
		log.Fatalf("Error during shutdown: %v", err)
	}
}
