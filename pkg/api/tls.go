package api

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
)

type TLSConfig struct {
	Enabled  bool
	CertFile string
	KeyFile  string
	CAFile   string
	Verify   bool
	MinTLS   uint16
}

// DefaultTLSConfig returns a default TLS configuration.
func DefaultTLSConfig() *TLSConfig {
	return &TLSConfig{
		Enabled: false,
		MinTLS:  tls.VersionTLS12,
	}
}

// NewTLSConfig creates a tls.Config from the given settings.
func NewTLSConfig(cfg *TLSConfig) (*tls.Config, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	tlsCfg := &tls.Config{
		MinVersion: cfg.MinTLS,
	}

	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load TLS cert/key: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	if cfg.Verify || cfg.CAFile != "" {
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
		caPool := x509.NewCertPool()

		if cfg.CAFile != "" {
			caCert, err := os.ReadFile(cfg.CAFile)
			if err != nil {
				return nil, fmt.Errorf("read CA cert: %w", err)
			}
			if !caPool.AppendCertsFromPEM(caCert) {
				return nil, fmt.Errorf("failed to parse CA certificate")
			}
		}
		tlsCfg.ClientCAs = caPool
	}

	return tlsCfg, nil
}

// TLSListener wraps a regular listener with TLS.
func TLSListener(l net.Listener, tlsCfg *tls.Config) net.Listener {
	if tlsCfg == nil {
		return l
	}
	return tls.NewListener(l, tlsCfg)
}

// GenerateSelfSignedCert generates a self-signed certificate for testing.
func GenerateSelfSignedCert(certFile, keyFile string) error {
	// Use openssl if available, otherwise generate with crypto/tls.
	// For production, use proper certificates.
	return fmt.Errorf("self-signed cert generation not implemented - use openssl: openssl req -x509 -newkey rsa:4096 -keyout %s -out %s -days 365 -nodes -subj '/CN=doki'", keyFile, certFile)
}
