// Package certs manages the platform CA used to issue client certificates
// for MQTT-device authentication (device X.509). A self-signed CA is
// generated on first start and persisted to a data directory.
package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultTTL is the validity of issued device certificates.
const DefaultTTL = 365 * 24 * time.Hour

// Manager signs device certificates and verifies peer certificates against
// the persisted platform CA.
type Manager struct {
	dir     string
	caCert  *x509.Certificate
	caKey   *ecdsa.PrivateKey
	caPEM   []byte
}

// LoadOrCreate opens the CA from dir (creating it on first run). The CA
// private key stays on disk with 0600 permissions and never leaves the core.
func LoadOrCreate(dir string) (*Manager, error) {
	if dir == "" {
		dir = "./certs"
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	certPath := filepath.Join(dir, "ca.crt")
	keyPath := filepath.Join(dir, "ca.key")

	if _, err := os.Stat(certPath); err == nil {
		// reuse existing CA
		pair, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("load existing CA: %w", err)
		}
		leaf, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil {
			return nil, err
		}
		key, ok := pair.PrivateKey.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("ca key must be ECDSA")
		}
		raw, _ := os.ReadFile(certPath)
		return &Manager{dir: dir, caCert: leaf, caKey: key, caPEM: raw}, nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "aiiot-platform-ca"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, err
	}
	return &Manager{dir: dir, caCert: leaf, caKey: key, caPEM: certPEM}, nil
}

// CAPEM returns the CA certificate in PEM form (for gateways to trust).
func (m *Manager) CAPEM() []byte { return m.caPEM }

// Pool returns a CertPool containing the platform CA.
func (m *Manager) Pool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(m.caPEM)
	return pool
}

// DeviceCN builds the client-certificate CommonName for a device: dev-<id>.
func DeviceCN(deviceID uint) string { return fmt.Sprintf("dev-%d", deviceID) }

// DeviceIDFromCN parses the device id out of a DeviceCN, returning false when
// the CN is not a platform-issued device CN.
func DeviceIDFromCN(cn string) (uint, bool) {
	if !strings.HasPrefix(cn, "dev-") {
		return 0, false
	}
	var id uint
	if _, err := fmt.Sscanf(cn, "dev-%d", &id); err != nil || id == 0 {
		return 0, false
	}
	return id, true
}

// SignDevice issues a client certificate for a device. The private key is NOT
// retained by the platform: it is returned once and must be handed to the
// device over a trusted channel.
func (m *Manager) SignDevice(deviceID uint) (certPEM, keyPEM []byte, serial string, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, "", err
	}
	serialBig, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, "", err
	}
	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber: serialBig,
		Subject:      pkix.Name{CommonName: DeviceCN(deviceID)},
		NotBefore:    now.Add(-10 * time.Minute),
		NotAfter:     now.Add(DefaultTTL),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, m.caCert, &key.PublicKey, m.caKey)
	if err != nil {
		return nil, nil, "", err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, "", err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, serialBig.Text(16), nil
}

// VerifyPeer checks a PEM client certificate against the platform CA
// (signature, validity window, client-auth usage).
func (m *Manager) VerifyPeer(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("no PEM block in certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	_, err = cert.Verify(x509.VerifyOptions{
		Roots:     m.Pool(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		return nil, err
	}
	return cert, nil
}