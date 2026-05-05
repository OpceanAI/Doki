package api

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"
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
// AE10: Generates self-signed certs when no cert is provided.
func GenerateSelfSignedCert(certFile, keyFile string) error {
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   "doki",
			Organization: []string{"Doki Container Engine"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost", "doki"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create cert: %w", err)
	}

	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key),
	}), 0600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}

	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: certDER,
	}), 0644); err != nil {
		return fmt.Errorf("write cert: %w", err)
	}

	return nil
}
