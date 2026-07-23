package securematerial

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/dytonpictures/werk/internal/core/providerregistry"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

func TestMaterialRefRequiresOpaqueHandleAndExactVersion(t *testing.T) {
	valid, err := NewMaterialRef(strings.Repeat("h", MaximumMaterialHandleBytes), 1)
	if err != nil {
		t.Fatalf("construct maximum-sized handle: %v", err)
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("maximum-sized handle rejected: %v", err)
	}
	if valid.Handle().Reveal() != strings.Repeat("h", MaximumMaterialHandleBytes) || valid.Version() != 1 {
		t.Fatal("material reference lost its exact coordinate")
	}
	if strings.Contains(valid.String(), "hhh") || strings.Contains(valid.GoString(), "hhh") ||
		strings.Contains(valid.Handle().String(), "hhh") || strings.Contains(valid.Handle().GoString(), "hhh") {
		t.Fatal("material reference formatting exposed its handle")
	}
	for _, formatted := range []string{
		fmt.Sprintf("%v", valid.Handle()),
		fmt.Sprintf("%+v", valid.Handle()),
		fmt.Sprintf("%#v", valid.Handle()),
		fmt.Sprintf("%q", valid.Handle()),
		fmt.Sprintf("%v", valid),
		fmt.Sprintf("%+v", valid),
		fmt.Sprintf("%#v", valid),
		fmt.Sprintf("%q", valid),
	} {
		if strings.Contains(formatted, "hhh") {
			t.Fatalf("automatic formatting exposed material handle: %q", formatted)
		}
	}
	encoded, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("marshal material reference: %v", err)
	}
	if strings.Contains(string(encoded), "hhh") {
		t.Fatalf("JSON exposed material handle: %s", encoded)
	}

	tests := []struct {
		handle  string
		version uint64
	}{
		{},
		{handle: "material", version: 0},
		{handle: " material", version: 1},
		{handle: "material\n", version: 1},
		{handle: strings.Repeat("h", MaximumMaterialHandleBytes+1), version: 1},
		{handle: string([]byte{0xff}), version: 1},
	}
	for _, candidate := range tests {
		if _, err := NewMaterialRef(candidate.handle, candidate.version); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid material reference accepted: %#v", candidate)
		}
	}
}

func TestOperationContextValidatesBoundaryTenantPurposeAndIdentifiers(t *testing.T) {
	installation := validOperation()
	if err := installation.Validate(); err != nil {
		t.Fatalf("installation operation rejected: %v", err)
	}

	tenantID := tenancy.TenantID{9}
	tenant := installation
	tenant.Boundary = providerregistry.OperationBoundaryTenant
	tenant.TenantID = tenantID
	if err := tenant.Validate(); err != nil {
		t.Fatalf("tenant operation rejected: %v", err)
	}

	maximumIdentifiers := installation
	maximumIdentifiers.RequestID = strings.Repeat("r", MaximumOperationIdentifierBytes)
	maximumIdentifiers.CorrelationID = strings.Repeat("c", MaximumOperationIdentifierBytes)
	if err := maximumIdentifiers.Validate(); err != nil {
		t.Fatalf("maximum-sized identifiers rejected: %v", err)
	}

	tests := map[string]func(*OperationContext){
		"invalid boundary":      func(value *OperationContext) { value.Boundary = "global" },
		"installation tenant":   func(value *OperationContext) { value.TenantID = tenantID },
		"tenant without tenant": func(value *OperationContext) { value.Boundary = providerregistry.OperationBoundaryTenant },
		"invalid purpose":       func(value *OperationContext) { value.PurposeKey = "TLS Handshake" },
		"empty request id":      func(value *OperationContext) { value.RequestID = "" },
		"blank correlation id":  func(value *OperationContext) { value.CorrelationID = " " },
		"request id max plus one": func(value *OperationContext) {
			value.RequestID = strings.Repeat("r", MaximumOperationIdentifierBytes+1)
		},
		"identifier control": func(value *OperationContext) { value.CorrelationID = "trace\nforged" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := installation
			mutate(&candidate)
			if !errors.Is(candidate.Validate(), ErrInvalid) {
				t.Fatal("invalid operation context accepted")
			}
		})
	}
}

func TestCertificateRequestsRequireExactServiceCapabilityAndOperation(t *testing.T) {
	server := CertificateRequest{
		Resolution: validResolution(CertificateServiceKey, ServerChainLoadCapabilityKey),
		Material:   validMaterial(),
		Operation:  validOperation(),
	}
	if err := server.ValidateServerChain(); err != nil {
		t.Fatalf("server-chain request rejected: %v", err)
	}
	if !errors.Is(server.ValidateClientTrustBundle(), ErrInvalid) {
		t.Fatal("server-chain capability accepted as client trust bundle")
	}

	clientTrust := server
	clientTrust.Resolution = validResolution(CertificateServiceKey, ClientTrustBundleLoadCapabilityKey)
	clientTrust.Operation.PurposeKey = MTLSClientTrustPurposeKey
	if err := clientTrust.ValidateClientTrustBundle(); err != nil {
		t.Fatalf("client-trust request rejected: %v", err)
	}

	wrongService := server
	wrongService.Resolution.ServiceKey = CryptographicKeyServiceKey
	if !errors.Is(wrongService.ValidateServerChain(), ErrInvalid) {
		t.Fatal("wrong certificate service accepted")
	}

	wrongCapability := server
	wrongCapability.Resolution.CapabilityKey = CertificateServiceKey + ".capability.server-chain.write"
	if !errors.Is(wrongCapability.ValidateServerChain(), ErrInvalid) {
		t.Fatal("wrong certificate capability accepted")
	}

	wrongPurpose := server
	wrongPurpose.Operation.PurposeKey = MTLSClientTrustPurposeKey
	if !errors.Is(wrongPurpose.ValidateServerChain(), ErrInvalid) {
		t.Fatal("server chain accepted for a different purpose")
	}
}

func TestSigningKeyRequestRequiresTLSHandshakeCapability(t *testing.T) {
	request := SigningKeyRequest{
		Resolution: validResolution(CryptographicKeyServiceKey, TLSHandshakeSignCapabilityKey),
		Material:   validMaterial(),
		Operation:  validOperation(),
	}
	request.Operation.PurposeKey = TLSHandshakePurposeKey
	if err := request.Validate(); err != nil {
		t.Fatalf("signing request rejected: %v", err)
	}

	request.Resolution.CapabilityKey = CryptographicKeyServiceKey + ".capability.document.sign"
	if !errors.Is(request.Validate(), ErrInvalid) {
		t.Fatal("non-TLS signing capability accepted")
	}

	request = SigningKeyRequest{
		Resolution: validResolution(CryptographicKeyServiceKey, TLSHandshakeSignCapabilityKey),
		Material:   validMaterial(),
		Operation:  validOperation(),
	}
	if !errors.Is(request.Validate(), ErrInvalid) {
		t.Fatal("TLS signing key accepted for a different purpose")
	}
}

func TestRequestsRejectTenantCoordinateMismatch(t *testing.T) {
	tenantID := tenancy.TenantID{9}
	otherTenantID := tenancy.TenantID{10}
	resolution := validResolution(CertificateServiceKey, ServerChainLoadCapabilityKey)
	resolution.OperationBoundary = providerregistry.OperationBoundaryTenant
	resolution.OperationTenantID = tenantID
	operation := validOperation()
	operation.Boundary = providerregistry.OperationBoundaryTenant
	operation.TenantID = otherTenantID

	request := CertificateRequest{Resolution: resolution, Material: validMaterial(), Operation: operation}
	if !errors.Is(request.ValidateServerChain(), ErrInvalid) {
		t.Fatal("different operation and resolution tenants accepted")
	}

	operation.TenantID = tenantID
	request.Operation = operation
	if err := request.ValidateServerChain(); err != nil {
		t.Fatalf("matching tenant coordinates rejected: %v", err)
	}
}

func TestRequestValidationRejectsMalformedResolution(t *testing.T) {
	request := CertificateRequest{
		Resolution: validResolution(CertificateServiceKey, ServerChainLoadCapabilityKey),
		Material:   validMaterial(),
		Operation:  validOperation(),
	}
	request.Resolution.ProviderRevision = 0
	if !errors.Is(request.ValidateServerChain(), ErrInvalid) {
		t.Fatal("malformed provider resolution accepted")
	}
}

func TestSecretRequestBindsCapabilityToPurpose(t *testing.T) {
	operation := validOperation()
	operation.PurposeKey = "transport.postgresql.client-auth"
	capabilityKey, err := SecretUseCapabilityKey(operation.PurposeKey)
	if err != nil {
		t.Fatalf("derive secret capability: %v", err)
	}
	want := SecretServiceKey + ".capability.transport.postgresql.client-auth.use"
	if capabilityKey != want {
		t.Fatalf("capability = %q, want %q", capabilityKey, want)
	}

	request := SecretRequest{
		Resolution: validResolution(SecretServiceKey, capabilityKey),
		Material:   validMaterial(),
		Operation:  operation,
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("purpose-bound secret request rejected: %v", err)
	}

	request.Resolution.CapabilityKey = SecretServiceKey + ".capability.secret.read"
	if !errors.Is(request.Validate(), ErrInvalid) {
		t.Fatal("generic secret.read capability accepted")
	}

	request.Resolution.CapabilityKey = SecretServiceKey + ".capability.other-purpose.use"
	if !errors.Is(request.Validate(), ErrInvalid) {
		t.Fatal("secret capability for a different purpose accepted")
	}

	if _, err := SecretUseCapabilityKey(strings.Repeat("purpose", 30)); !errors.Is(err, ErrInvalid) {
		t.Fatal("derived capability exceeding the stable-key limit accepted")
	}
}

func TestCertificateBundleEnforcesLimitsAndDefensiveCopies(t *testing.T) {
	material := validMaterial()
	first := validCertificateDER(t)
	second := validCertificateDER(t)
	input := [][]byte{first, second}
	bundle, err := NewCertificateBundle(material, input)
	if err != nil {
		t.Fatalf("construct certificate bundle: %v", err)
	}

	first[0] = 9
	input[1] = []byte{9}
	got := bundle.ChainDER()
	if got[0][0] == 9 || len(got[1]) != len(second) {
		t.Fatal("bundle retained aliases to constructor input")
	}
	got[0][0] = 8
	got[1] = []byte{8}
	again := bundle.ChainDER()
	if again[0][0] == 8 || len(again[1]) != len(second) {
		t.Fatal("bundle accessor exposed mutable internal slices")
	}
	if bundle.Material() != material {
		t.Fatal("bundle lost exact material reference")
	}

	atCountLimit := make([][]byte, MaximumCertificateCount)
	for index := range atCountLimit {
		atCountLimit[index] = validCertificateDER(t)
	}
	if _, err := NewCertificateBundle(material, atCountLimit); err != nil {
		t.Fatalf("certificate count boundary rejected: %v", err)
	}
	if _, err := NewCertificateBundle(material, append(atCountLimit, first)); !errors.Is(err, ErrMaterialLimit) {
		t.Fatal("certificate count max+1 accepted")
	}

	if _, err := NewCertificateBundle(material, [][]byte{make([]byte, MaximumCertificateDERBytes+1)}); !errors.Is(err, ErrMaterialLimit) {
		t.Fatal("per-certificate DER max+1 accepted")
	}

	totalOverLimit := [][]byte{
		make([]byte, MaximumCertificateDERBytes),
		make([]byte, MaximumCertificateDERBytes),
		make([]byte, MaximumCertificateDERBytes),
		make([]byte, MaximumCertificateDERBytes),
		[]byte{1},
	}
	if _, err := NewCertificateBundle(material, totalOverLimit); !errors.Is(err, ErrMaterialLimit) {
		t.Fatal("total DER max+1 accepted")
	}

	if _, err := NewCertificateBundle(material, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("empty certificate bundle accepted")
	}
	if _, err := NewCertificateBundle(material, [][]byte{{}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("empty DER certificate accepted")
	}
	if _, err := NewCertificateBundle(material, [][]byte{{1, 2, 3}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("arbitrary bytes accepted as certificate DER")
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	if _, err := NewCertificateBundle(material, [][]byte{privateDER}); !errors.Is(err, ErrInvalid) {
		t.Fatal("private key DER accepted as certificate DER")
	}
}

func TestSigningInputAndSignatureAreBoundedAndDefensivelyCopied(t *testing.T) {
	digest := make([]byte, crypto.SHA256.Size())
	input, err := NewTLSHandshakeSigningInput(digest, crypto.SHA256)
	if err != nil {
		t.Fatalf("construct signing input: %v", err)
	}
	digest[0] = 9
	got := input.Bytes()
	if got[0] != 0 {
		t.Fatal("signing input retained caller-owned bytes")
	}
	got[0] = 8
	if input.Bytes()[0] != 0 {
		t.Fatal("signing input exposed mutable bytes")
	}
	if err := input.Validate(); err != nil {
		t.Fatalf("valid signing input rejected: %v", err)
	}

	pss, err := NewTLSHandshakeSigningInput(make([]byte, crypto.SHA384.Size()), &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash,
		Hash:       crypto.SHA384,
	})
	if err != nil || pss.Validate() != nil {
		t.Fatalf("valid RSA-PSS input rejected: %v", err)
	}

	invalid := []struct {
		payload []byte
		opts    crypto.SignerOpts
	}{
		{nil, crypto.SHA256},
		{make([]byte, crypto.SHA256.Size()-1), crypto.SHA256},
		{make([]byte, crypto.SHA256.Size()), crypto.SHA1},
		{make([]byte, crypto.SHA256.Size()), &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthAuto, Hash: crypto.SHA256}},
		{make([]byte, crypto.SHA256.Size()), panickingSignerOpts{}},
	}
	for index, candidate := range invalid {
		if _, err := NewTLSHandshakeSigningInput(candidate.payload, candidate.opts); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid signing input %d accepted", index)
		}
	}
	if _, err := NewTLSHandshakeSigningInput(
		make([]byte, MaximumSigningInputBytes+1), crypto.Hash(0),
	); !errors.Is(err, ErrMaterialLimit) {
		t.Fatal("oversized signing input did not return the material-limit error")
	}

	request := SigningKeyRequest{
		Resolution: validResolution(CryptographicKeyServiceKey, TLSHandshakeSignCapabilityKey),
		Material:   validMaterial(),
		Operation:  validOperation(),
	}
	request.Operation.PurposeKey = TLSHandshakePurposeKey
	value := []byte{1, 2, 3}
	signature, err := NewSignature(request, input, value)
	if err != nil {
		t.Fatalf("construct signature: %v", err)
	}
	value[0] = 9
	if signature.Bytes()[0] != 1 || signature.Material() != request.Material ||
		signature.Resolution() != request.Resolution {
		t.Fatal("signature lost its exact request binding or retained caller bytes")
	}
	if gotInput := signature.Input(); string(gotInput.Bytes()) != string(input.Bytes()) ||
		gotInput.SignerOpts().HashFunc() != input.SignerOpts().HashFunc() {
		t.Fatal("signature lost its exact signing-input binding")
	}
	otherInput, err := NewTLSHandshakeSigningInput(make([]byte, crypto.SHA384.Size()), crypto.SHA384)
	if err != nil {
		t.Fatalf("construct other signing input: %v", err)
	}
	if string(signature.Input().Bytes()) == string(otherInput.Bytes()) &&
		signature.Input().SignerOpts().HashFunc() == otherInput.SignerOpts().HashFunc() {
		t.Fatal("different signing inputs became indistinguishable")
	}
	if _, err := NewSignature(
		request, input, make([]byte, MaximumSignatureBytes+1),
	); !errors.Is(err, ErrMaterialLimit) {
		t.Fatal("oversized signature did not return the material-limit error")
	}
}

func TestPublicKeyMaterialRejectsPrivateKeysAndBindsRequest(t *testing.T) {
	request := SigningKeyRequest{
		Resolution: validResolution(CryptographicKeyServiceKey, TLSHandshakeSignCapabilityKey),
		Material:   validMaterial(),
		Operation:  validOperation(),
	}
	request.Operation.PurposeKey = TLSHandshakePurposeKey
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	material, err := NewPublicKeyMaterial(request, publicKey)
	if err != nil {
		t.Fatalf("construct public-key material: %v", err)
	}
	if err := material.Validate(); err != nil {
		t.Fatalf("valid public-key material rejected: %v", err)
	}
	parsed, err := material.Parse()
	if err != nil {
		t.Fatalf("parse public-key material: %v", err)
	}
	if _, ok := parsed.(ed25519.PublicKey); !ok {
		t.Fatalf("parsed key type = %T, want ed25519.PublicKey", parsed)
	}
	der := material.PKIXDER()
	der[0] ^= 0xff
	if err := material.Validate(); err != nil || material.Material() != request.Material ||
		material.Resolution() != request.Resolution {
		t.Fatal("public-key material exposed mutable bytes or lost its request binding")
	}
	if _, err := NewPublicKeyMaterial(request, privateKey); !errors.Is(err, ErrInvalid) {
		t.Fatal("private key accepted as public-key material")
	}
	if _, err := NewPublicKeyMaterial(request, struct{}{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown public-key type accepted")
	}
}

func TestPortsRequireRequestBoundOperations(t *testing.T) {
	var _ CertificateProvider = certificateProviderStub{}
	var _ SigningKeyProvider = signingKeyProviderStub{}
	var _ SecretProvider = secretProviderStub{}
}

type certificateProviderStub struct{}

func (certificateProviderStub) LoadServerChain(context.Context, CertificateRequest) (CertificateBundle, error) {
	return CertificateBundle{}, nil
}

func (certificateProviderStub) LoadClientTrustBundle(context.Context, CertificateRequest) (CertificateBundle, error) {
	return CertificateBundle{}, nil
}

type signingKeyProviderStub struct{}

func (signingKeyProviderStub) PublicKey(context.Context, SigningKeyRequest) (PublicKeyMaterial, error) {
	return PublicKeyMaterial{}, nil
}

func (signingKeyProviderStub) Sign(context.Context, SigningKeyRequest, SigningInput) (Signature, error) {
	return Signature{}, nil
}

type secretProviderStub struct{}

func (secretProviderStub) UseSecret(context.Context, SecretRequest, SecretConsumer) error {
	return nil
}

type panickingSignerOpts struct{}

func (panickingSignerOpts) HashFunc() crypto.Hash {
	panic("unknown signer options must be rejected before method dispatch")
}

func validMaterial() MaterialRef {
	material, err := NewMaterialRef("provider-owned/material/handle", 2)
	if err != nil {
		panic(err)
	}
	return material
}

func validCertificateDER(t *testing.T) []byte {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate certificate key: %v", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: "securematerial.test"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return certificateDER
}

func validOperation() OperationContext {
	return OperationContext{
		Boundary:      providerregistry.OperationBoundaryInstallation,
		PurposeKey:    TLSServerIdentityPurposeKey,
		RequestID:     "request-1",
		CorrelationID: "correlation-1",
	}
}

func validResolution(serviceKey, capabilityKey string) providerregistry.Resolution {
	return providerregistry.Resolution{
		ProviderID:              providerregistry.ProviderID{1},
		ProviderKey:             serviceKey + ".provider.local",
		AdapterKey:              "internal.securematerial.local",
		ProviderConfigScope:     providerregistry.ConfigScopeInstallation,
		ProviderRevision:        3,
		BindingRevision:         4,
		RegistryContractVersion: providerregistry.ContractVersionV1,
		ServiceKey:              serviceKey,
		ServiceVersion:          1,
		CapabilityKey:           capabilityKey,
		CapabilityVersion:       1,
		OperationBoundary:       providerregistry.OperationBoundaryInstallation,
	}
}
