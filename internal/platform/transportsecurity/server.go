package transportsecurity

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// ServerMode defines the authentication performed by a native TLS listener.
// Authorization and authority ownership remain separate application checks.
type ServerMode string

const (
	ServerDisabled ServerMode = "disabled"
	ServerTLS      ServerMode = "tls"
	ServerMTLS     ServerMode = "mtls"
)

const materialCheckInterval = time.Second
const maximumMaterialFileSize = 4 << 20

// ServerOptions contains file references only. Private key material is never
// copied into the general platform configuration or persisted by this package.
type ServerOptions struct {
	Mode            ServerMode
	CertificateFile string
	PrivateKeyFile  string
	ClientCAFile    string
}

func (options ServerOptions) Enabled() bool {
	return options.Mode == ServerTLS || options.Mode == ServerMTLS
}

// Validate checks the transport contract without reading secret material.
func (options ServerOptions) Validate() error {
	switch options.Mode {
	case ServerDisabled:
		if options.CertificateFile != "" || options.PrivateKeyFile != "" || options.ClientCAFile != "" {
			return errors.New("disabled server TLS must not reference certificate files")
		}
		return nil
	case ServerTLS, ServerMTLS:
		if options.CertificateFile == "" || options.PrivateKeyFile == "" {
			return errors.New("server TLS requires a certificate and private key file")
		}
	default:
		return fmt.Errorf("unsupported server TLS mode %q", options.Mode)
	}
	if options.Mode == ServerMTLS && options.ClientCAFile == "" {
		return errors.New("mutual server TLS requires a client CA file")
	}
	if options.Mode == ServerTLS && options.ClientCAFile != "" {
		return errors.New("a client CA file requires mutual server TLS")
	}
	return nil
}

// NewServerTLSConfig creates a fail-closed TLS configuration for a native Go
// listener. Certificate and client trust files are checked periodically and a
// complete, valid replacement is used for new handshakes without a restart.
func NewServerTLSConfig(options ServerOptions) (*tls.Config, error) {
	return newServerTLSConfig(options, time.Now, materialCheckInterval)
}

type materialVersion struct {
	certificate [sha256.Size]byte
	privateKey  [sha256.Size]byte
	clientCA    [sha256.Size]byte
}

type materialFiles struct {
	version     materialVersion
	certificate []byte
	privateKey  []byte
	clientCA    []byte
}

type serverMaterial struct {
	version     materialVersion
	certificate tls.Certificate
	leaf        *x509.Certificate
	clientCAs   *x509.CertPool
}

type serverMaterialProvider struct {
	options       ServerOptions
	now           func() time.Time
	checkInterval time.Duration

	mu         sync.Mutex
	material   *serverMaterial
	checkAfter time.Time
}

func newServerTLSConfig(options ServerOptions, now func() time.Time, checkInterval time.Duration) (*tls.Config, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	if !options.Enabled() {
		return nil, errors.New("server TLS is disabled")
	}
	if now == nil {
		now = time.Now
	}
	if checkInterval < 0 {
		return nil, errors.New("certificate check interval must not be negative")
	}

	provider := &serverMaterialProvider{
		options:       options,
		now:           now,
		checkInterval: checkInterval,
	}
	if _, err := provider.current(); err != nil {
		return nil, err
	}

	configuration := &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"h2", "http/1.1"},
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			material, err := provider.current()
			if err != nil {
				return nil, err
			}
			return &material.certificate, nil
		},
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			material, err := provider.current()
			if err != nil {
				return nil, err
			}
			return material.configuration(options.Mode), nil
		},
	}
	if options.Mode == ServerMTLS {
		configuration.ClientAuth = tls.RequireAndVerifyClientCert
		configuration.ClientCAs = provider.material.clientCAs
	}
	return configuration, nil
}

func (material *serverMaterial) configuration(mode ServerMode) *tls.Config {
	configuration := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"h2", "http/1.1"},
		Certificates: []tls.Certificate{material.certificate},
	}
	if mode == ServerMTLS {
		configuration.ClientAuth = tls.RequireAndVerifyClientCert
		configuration.ClientCAs = material.clientCAs
	} else {
		configuration.ClientAuth = tls.NoClientCert
		configuration.ClientCAs = nil
	}
	return configuration
}

func (provider *serverMaterialProvider) current() (*serverMaterial, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()

	now := provider.now()
	if provider.material != nil && now.Before(provider.checkAfter) {
		if err := validateLeafTime(provider.material.leaf, now); err != nil {
			return nil, err
		}
		return provider.material, nil
	}

	files, err := readMaterial(provider.options)
	if err != nil {
		return nil, err
	}
	if provider.material != nil && provider.material.version == files.version {
		if err := validateLeafTime(provider.material.leaf, now); err != nil {
			return nil, err
		}
		provider.checkAfter = now.Add(provider.checkInterval)
		return provider.material, nil
	}

	loaded, err := loadServerMaterial(provider.options, files, now)
	if err != nil {
		// Do not advance checkAfter. A partially replaced or otherwise invalid
		// certificate set is retried on the next handshake and never activated.
		return nil, err
	}
	provider.material = loaded
	provider.checkAfter = now.Add(provider.checkInterval)
	return loaded, nil
}

func readMaterial(options ServerOptions) (materialFiles, error) {
	certificate, err := readRegularFile(options.CertificateFile)
	if err != nil {
		return materialFiles{}, fmt.Errorf("read server certificate: %w", err)
	}
	privateKey, err := readRegularFile(options.PrivateKeyFile)
	if err != nil {
		return materialFiles{}, fmt.Errorf("read server private key: %w", err)
	}
	files := materialFiles{
		certificate: certificate,
		privateKey:  privateKey,
		version: materialVersion{
			certificate: sha256.Sum256(certificate),
			privateKey:  sha256.Sum256(privateKey),
		},
	}
	if options.Mode == ServerMTLS {
		files.clientCA, err = readRegularFile(options.ClientCAFile)
		if err != nil {
			return materialFiles{}, fmt.Errorf("read client CA bundle: %w", err)
		}
		files.version.clientCA = sha256.Sum256(files.clientCA)
	}
	return files, nil
}

func readRegularFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	information, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !information.Mode().IsRegular() {
		return nil, errors.New("certificate material must be a regular file")
	}
	contents, err := io.ReadAll(io.LimitReader(file, maximumMaterialFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > maximumMaterialFileSize {
		return nil, fmt.Errorf("certificate material exceeds %d bytes", maximumMaterialFileSize)
	}
	return contents, nil
}

func loadServerMaterial(options ServerOptions, files materialFiles, now time.Time) (*serverMaterial, error) {
	certificate, err := tls.X509KeyPair(files.certificate, files.privateKey)
	if err != nil {
		return nil, fmt.Errorf("load server certificate pair: %w", err)
	}
	if len(certificate.Certificate) == 0 {
		return nil, errors.New("server certificate chain is empty")
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parse server leaf certificate: %w", err)
	}
	if leaf.IsCA {
		return nil, errors.New("server leaf certificate must not be a certificate authority")
	}
	if !allowsServerAuthentication(leaf) {
		return nil, errors.New("server leaf certificate is not valid for server authentication")
	}
	if err := validateLeafTime(leaf, now); err != nil {
		return nil, err
	}
	certificate.Leaf = leaf

	material := &serverMaterial{version: files.version, certificate: certificate, leaf: leaf}
	if options.Mode == ServerMTLS {
		material.clientCAs, err = loadCAPool(files.clientCA, now)
		if err != nil {
			return nil, err
		}
	}
	return material, nil
}

func allowsServerAuthentication(certificate *x509.Certificate) bool {
	if len(certificate.ExtKeyUsage) == 0 {
		return true
	}
	for _, usage := range certificate.ExtKeyUsage {
		if usage == x509.ExtKeyUsageAny || usage == x509.ExtKeyUsageServerAuth {
			return true
		}
	}
	return false
}

func validateLeafTime(certificate *x509.Certificate, now time.Time) error {
	if now.Before(certificate.NotBefore) {
		return fmt.Errorf("server certificate is not valid before %s", certificate.NotBefore.UTC().Format(time.RFC3339))
	}
	if !now.Before(certificate.NotAfter) {
		return fmt.Errorf("server certificate expired at %s", certificate.NotAfter.UTC().Format(time.RFC3339))
	}
	return nil
}

func loadCAPool(contents []byte, now time.Time) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	validAuthorities := 0
	for remaining := contents; len(remaining) > 0; {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}
		remaining = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		certificate, parseErr := x509.ParseCertificate(block.Bytes)
		if parseErr != nil || !certificate.IsCA || now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
			continue
		}
		pool.AddCert(certificate)
		validAuthorities++
	}
	if validAuthorities == 0 {
		return nil, errors.New("client CA bundle contains no currently valid certificate authority")
	}
	return pool, nil
}
