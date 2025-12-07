package server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

func (c *Config) LoadTLSConfig() (*tls.Config, error) {
	if !c.TLS.Enabled {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if c.TLS.MinVersion == "1.3" {
		tlsConfig.MinVersion = tls.VersionTLS13
	}

	if c.TLS.CertFile != "" && c.TLS.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(c.TLS.CertFile, c.TLS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if c.TLS.CAFile != "" {
		caCert, err := os.ReadFile(c.TLS.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.ClientCAs = caCertPool
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	} else {
		tlsConfig.ClientAuth = tls.NoClientCert
	}

	return tlsConfig, nil
}
