package transportsecurity

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServerOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		options ServerOptions
		valid   bool
	}{
		{name: "disabled", options: ServerOptions{Mode: ServerDisabled}, valid: true},
		{name: "tls", options: ServerOptions{Mode: ServerTLS, CertificateFile: "server.pem", PrivateKeyFile: "server-key.pem"}, valid: true},
		{name: "mutual", options: ServerOptions{Mode: ServerMTLS, CertificateFile: "server.pem", PrivateKeyFile: "server-key.pem", ClientCAFile: "clients.pem"}, valid: true},
		{name: "unknown", options: ServerOptions{Mode: "optional"}},
		{name: "disabled with material", options: ServerOptions{Mode: ServerDisabled, CertificateFile: "server.pem"}},
		{name: "tls without key", options: ServerOptions{Mode: ServerTLS, CertificateFile: "server.pem"}},
		{name: "tls with client CA", options: ServerOptions{Mode: ServerTLS, CertificateFile: "server.pem", PrivateKeyFile: "server-key.pem", ClientCAFile: "clients.pem"}},
		{name: "mutual without client CA", options: ServerOptions{Mode: ServerMTLS, CertificateFile: "server.pem", PrivateKeyFile: "server-key.pem"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.options.Validate()
			if test.valid && err != nil {
				t.Fatalf("valid options rejected: %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("invalid options accepted")
			}
		})
	}
}

func TestServerTLSAuthenticatesServer(t *testing.T) {
	files := newTestPKI(t, time.Now())
	configuration, err := NewServerTLSConfig(ServerOptions{
		Mode: ServerTLS, CertificateFile: files.serverCertificate, PrivateKeyFile: files.serverKey,
	})
	if err != nil {
		t.Fatalf("create server TLS configuration: %v", err)
	}
	if configuration.MinVersion != tls.VersionTLS12 {
		t.Fatalf("minimum TLS version = %d, want TLS 1.2", configuration.MinVersion)
	}

	clientErr, serverErr := tlsHandshake(configuration, &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    files.roots,
		ServerName: "server.platform.test",
	})
	if clientErr != nil || serverErr != nil {
		t.Fatalf("TLS handshake failed: client=%v server=%v", clientErr, serverErr)
	}
}

func TestNativeHTTPServerServesHTTPSAndHTTP2(t *testing.T) {
	files := newTestPKI(t, time.Now())
	configuration, err := NewServerTLSConfig(ServerOptions{
		Mode: ServerTLS, CertificateFile: files.serverCertificate, PrivateKeyFile: files.serverKey,
	})
	if err != nil {
		t.Fatalf("create server TLS configuration: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{
		TLSConfig: configuration,
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte("secure"))
		}),
	}
	serverResults := make(chan error, 1)
	go func() { serverResults <- server.ServeTLS(listener, "", "") }()

	transport := &http.Transport{
		ForceAttemptHTTP2: true,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    files.roots,
			ServerName: "server.platform.test",
		},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, listener.Addr().String())
		},
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	response, err := client.Get("https://server.platform.test/")
	if err != nil {
		t.Fatalf("call native HTTPS server: %v", err)
	}
	contents, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK || string(contents) != "secure" {
		t.Fatalf("unexpected HTTPS response: status=%d body=%q error=%v", response.StatusCode, contents, readErr)
	}
	if response.ProtoMajor != 2 {
		t.Fatalf("HTTP protocol = %s, want HTTP/2", response.Proto)
	}

	transport.CloseIdleConnections()
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serverResults; err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("native HTTPS server stopped unexpectedly: %v", err)
	}
}

func TestServerMTLSRequiresTrustedClient(t *testing.T) {
	files := newTestPKI(t, time.Now())
	configuration, err := NewServerTLSConfig(ServerOptions{
		Mode:            ServerMTLS,
		CertificateFile: files.serverCertificate,
		PrivateKeyFile:  files.serverKey,
		ClientCAFile:    files.caCertificate,
	})
	if err != nil {
		t.Fatalf("create mutual TLS configuration: %v", err)
	}

	trustedClient := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      files.roots,
		ServerName:   "server.platform.test",
		Certificates: []tls.Certificate{files.clientCertificatePair},
	}
	clientErr, serverErr := tlsHandshake(configuration, trustedClient)
	if clientErr != nil || serverErr != nil {
		t.Fatalf("trusted mutual TLS handshake failed: client=%v server=%v", clientErr, serverErr)
	}

	untrustedClient := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    files.roots,
		ServerName: "server.platform.test",
	}
	clientErr, serverErr = tlsHandshake(configuration, untrustedClient)
	if clientErr == nil && serverErr == nil {
		t.Fatal("mutual TLS accepted a client without a certificate")
	}
}

func TestServerTLSReloadsOnlyCompleteValidMaterial(t *testing.T) {
	now := time.Now().UTC()
	files := newTestPKI(t, now)
	configuration, err := newServerTLSConfig(ServerOptions{
		Mode: ServerTLS, CertificateFile: files.serverCertificate, PrivateKeyFile: files.serverKey,
	}, func() time.Time { return now }, 0)
	if err != nil {
		t.Fatalf("create server TLS configuration: %v", err)
	}

	initial, err := configuration.GetConfigForClient(nil)
	if err != nil {
		t.Fatalf("get initial certificate: %v", err)
	}
	if initial.Certificates[0].Leaf.SerialNumber.Int64() != 2 {
		t.Fatalf("initial serial = %s, want 2", initial.Certificates[0].Leaf.SerialNumber)
	}

	if err := os.WriteFile(files.serverCertificate, []byte("incomplete rotation"), 0o600); err != nil {
		t.Fatal(err)
	}
	changedAt := now.Add(2 * time.Second)
	if err := os.Chtimes(files.serverCertificate, changedAt, changedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := configuration.GetConfigForClient(nil); err == nil {
		t.Fatal("invalid replacement certificate was activated")
	}

	serverCertificate, serverKey, _ := issueCertificate(t, files.caCertificateValue, files.caKey, 4, now, false, "server.platform.test")
	writePEM(t, files.serverCertificate, "CERTIFICATE", serverCertificate.Raw)
	writePrivateKey(t, files.serverKey, serverKey)
	changedAt = now.Add(4 * time.Second)
	if err := os.Chtimes(files.serverCertificate, changedAt, changedAt); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(files.serverKey, changedAt, changedAt); err != nil {
		t.Fatal(err)
	}
	reloaded, err := configuration.GetConfigForClient(nil)
	if err != nil {
		t.Fatalf("load complete replacement certificate: %v", err)
	}
	if reloaded.Certificates[0].Leaf.SerialNumber.Int64() != 4 {
		t.Fatalf("reloaded serial = %s, want 4", reloaded.Certificates[0].Leaf.SerialNumber)
	}
}

func TestServerMTLSReloadsClientTrustBundle(t *testing.T) {
	now := time.Now().UTC()
	files := newTestPKI(t, now)
	configuration, err := newServerTLSConfig(ServerOptions{
		Mode:            ServerMTLS,
		CertificateFile: files.serverCertificate,
		PrivateKeyFile:  files.serverKey,
		ClientCAFile:    files.caCertificate,
	}, func() time.Time { return now }, 0)
	if err != nil {
		t.Fatalf("create mutual TLS configuration: %v", err)
	}

	newAuthority, newAuthorityKey := issueCA(t, now)
	_, _, newClient := issueCertificate(t, newAuthority, newAuthorityKey, 5, now, true, "new-client.platform.test")
	writePEM(t, files.caCertificate, "CERTIFICATE", newAuthority.Raw)
	clientErr, serverErr := tlsHandshake(configuration, &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      files.roots,
		ServerName:   "server.platform.test",
		Certificates: []tls.Certificate{newClient},
	})
	if clientErr != nil || serverErr != nil {
		t.Fatalf("rotated client trust bundle was not used: client=%v server=%v", clientErr, serverErr)
	}

	clientErr, serverErr = tlsHandshake(configuration, &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      files.roots,
		ServerName:   "server.platform.test",
		Certificates: []tls.Certificate{files.clientCertificatePair},
	})
	if clientErr == nil && serverErr == nil {
		t.Fatal("client signed by the removed authority was still trusted")
	}
}

func TestServerTLSRejectsExpiredCertificateAtStartup(t *testing.T) {
	now := time.Now().UTC()
	files := newTestPKI(t, now.Add(-48*time.Hour))
	_, err := newServerTLSConfig(ServerOptions{
		Mode: ServerTLS, CertificateFile: files.serverCertificate, PrivateKeyFile: files.serverKey,
	}, func() time.Time { return now }, 0)
	if err == nil {
		t.Fatal("expired server certificate was accepted")
	}
}

func TestServerTLSRejectsClientOnlyLeafAtStartup(t *testing.T) {
	now := time.Now().UTC()
	files := newTestPKI(t, now)
	clientLeaf, clientKey, _ := issueCertificate(t, files.caCertificateValue, files.caKey, 6, now, true, "client-only.platform.test")
	writePEM(t, files.serverCertificate, "CERTIFICATE", clientLeaf.Raw)
	writePrivateKey(t, files.serverKey, clientKey)
	if _, err := newServerTLSConfig(ServerOptions{
		Mode: ServerTLS, CertificateFile: files.serverCertificate, PrivateKeyFile: files.serverKey,
	}, func() time.Time { return now }, 0); err == nil {
		t.Fatal("client-only leaf certificate was accepted as a server certificate")
	}
}

type testPKI struct {
	caCertificate         string
	serverCertificate     string
	serverKey             string
	roots                 *x509.CertPool
	caCertificateValue    *x509.Certificate
	caKey                 ed25519.PrivateKey
	clientCertificatePair tls.Certificate
}

func newTestPKI(t *testing.T, validityStart time.Time) testPKI {
	t.Helper()
	directory := t.TempDir()
	caCertificate, caKey := issueCA(t, validityStart)
	serverCertificate, serverKey, serverPair := issueCertificate(t, caCertificate, caKey, 2, validityStart, false, "server.platform.test")
	clientCertificate, clientKey, clientPair := issueCertificate(t, caCertificate, caKey, 3, validityStart, true, "client.platform.test")

	caPath := filepath.Join(directory, "ca.pem")
	serverPath := filepath.Join(directory, "server.pem")
	serverKeyPath := filepath.Join(directory, "server-key.pem")
	writePEM(t, caPath, "CERTIFICATE", caCertificate.Raw)
	writePEM(t, serverPath, "CERTIFICATE", serverCertificate.Raw)
	writePrivateKey(t, serverKeyPath, serverKey)

	roots := x509.NewCertPool()
	roots.AddCert(caCertificate)
	_ = clientCertificate
	_ = clientKey
	_ = serverPair
	return testPKI{
		caCertificate:         caPath,
		serverCertificate:     serverPath,
		serverKey:             serverKeyPath,
		roots:                 roots,
		caCertificateValue:    caCertificate,
		caKey:                 caKey,
		clientCertificatePair: clientPair,
	}
}

func issueCA(t *testing.T, validityStart time.Time) (*x509.Certificate, ed25519.PrivateKey) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Platform Test CA"},
		NotBefore:             validityStart.Add(-time.Hour),
		NotAfter:              validityStart.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, privateKey.Public(), privateKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return certificate, privateKey
}

func issueCertificate(t *testing.T, authority *x509.Certificate, authorityKey ed25519.PrivateKey, serial int64, validityStart time.Time, client bool, dnsName string) (*x509.Certificate, ed25519.PrivateKey, tls.Certificate) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	extendedUsage := x509.ExtKeyUsageServerAuth
	if client {
		extendedUsage = x509.ExtKeyUsageClientAuth
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    validityStart.Add(-time.Hour),
		NotAfter:     validityStart.Add(12 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{extendedUsage},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, authority, privateKey.Public(), authorityKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return certificate, privateKey, pair
}

func writePEM(t *testing.T, path, blockType string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: contents}), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writePrivateKey(t *testing.T, path string, key ed25519.PrivateKey) {
	t.Helper()
	contents, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	writePEM(t, path, "PRIVATE KEY", contents)
}

func tlsHandshake(serverConfiguration, clientConfiguration *tls.Config) (error, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err, err
	}
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	serverResults := make(chan error, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverResults <- acceptErr
			return
		}
		defer connection.Close()
		serverResults <- tls.Server(connection, serverConfiguration).HandshakeContext(ctx)
	}()

	rawClient, err := (&net.Dialer{}).DialContext(ctx, "tcp", listener.Addr().String())
	if err != nil {
		return err, <-serverResults
	}
	clientConnection := tls.Client(rawClient, clientConfiguration)
	clientErr := clientConnection.HandshakeContext(ctx)
	_ = rawClient.Close()
	return clientErr, <-serverResults
}
