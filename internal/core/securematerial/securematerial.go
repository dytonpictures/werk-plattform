// Package securematerial defines narrow provider ports for certificates,
// non-exportable signing keys and purpose-bound secrets. It deliberately owns
// no storage, provider selection or cryptographic implementation.
package securematerial

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/dytonpictures/werk/internal/core/providerregistry"
	"github.com/dytonpictures/werk/internal/core/resource"
	"github.com/dytonpictures/werk/internal/core/tenancy"
)

var (
	// ErrInvalid indicates a malformed, mismatched or overly broad request.
	ErrInvalid = errors.New("invalid secure material contract")
	// ErrMaterialLimit indicates that material exceeded a defensive size limit.
	ErrMaterialLimit = errors.New("secure material exceeds limit")
)

const (
	CertificateServiceKey      = "core.platform.service.certificate"
	CryptographicKeyServiceKey = "core.platform.service.cryptographic-key"
	SecretServiceKey           = "core.platform.service.secret"

	ServerChainLoadCapabilityKey       = CertificateServiceKey + ".capability.server-chain.load"
	ClientTrustBundleLoadCapabilityKey = CertificateServiceKey +
		".capability.client-trust-bundle.load"
	TLSHandshakeSignCapabilityKey = CryptographicKeyServiceKey +
		".capability.tls-handshake.sign"
	TLSServerIdentityPurposeKey = "core.transport.tls-server-identity"
	MTLSClientTrustPurposeKey   = "core.transport.mtls-client-trust"
	TLSHandshakePurposeKey      = "core.transport.tls-handshake"

	MaximumMaterialHandleBytes       = 1024
	MaximumOperationIdentifierBytes  = 256
	MaximumSecretBytes               = 1 << 20
	MaximumSigningInputBytes         = 4 << 10
	MaximumSignatureBytes            = 16 << 10
	MaximumPublicKeyDERBytes         = 16 << 10
	MaximumCertificateCount          = 16
	MaximumCertificateDERBytes       = 1 << 20
	MaximumCertificateBundleDERBytes = 4 << 20
)

// OpaqueHandle is provider-local sensitive metadata. Reveal is intentionally
// explicit; ordinary formatting is redacted.
type OpaqueHandle struct {
	value string
}

func NewOpaqueHandle(value string) (OpaqueHandle, error) {
	if !validOpaqueHandle(value) {
		return OpaqueHandle{}, ErrInvalid
	}
	return OpaqueHandle{value: value}, nil
}

func (handle OpaqueHandle) Reveal() string {
	return handle.value
}

func (handle OpaqueHandle) String() string {
	return "[secure-material-handle]"
}

func (handle OpaqueHandle) GoString() string {
	return "securematerial.OpaqueHandle([redacted])"
}

func (handle OpaqueHandle) valid() bool {
	return validOpaqueHandle(handle.value)
}

// MaterialRef is an exact provider-owned material coordinate. Its version
// prevents a caller from silently following rotation.
type MaterialRef struct {
	handle  OpaqueHandle
	version uint64
}

func NewMaterialRef(handle string, version uint64) (MaterialRef, error) {
	opaque, err := NewOpaqueHandle(handle)
	if err != nil || version == 0 {
		return MaterialRef{}, ErrInvalid
	}
	return MaterialRef{handle: opaque, version: version}, nil
}

func (ref MaterialRef) Validate() error {
	if ref.version == 0 || !ref.handle.valid() {
		return ErrInvalid
	}
	return nil
}

func (ref MaterialRef) Handle() OpaqueHandle {
	return ref.handle
}

func (ref MaterialRef) Version() uint64 {
	return ref.version
}

func (ref MaterialRef) String() string {
	return "[secure-material-ref]"
}

func (ref MaterialRef) GoString() string {
	return "securematerial.MaterialRef([redacted])"
}

// OperationContext binds a provider operation to its data boundary, purpose
// and trace coordinates. TenantID must be zero only for installation-bound
// operations.
type OperationContext struct {
	Boundary      providerregistry.OperationBoundary
	TenantID      tenancy.TenantID
	PurposeKey    string
	RequestID     string
	CorrelationID string
}

func (operation OperationContext) Validate() error {
	if !resource.ValidKey(operation.PurposeKey) ||
		!validIdentifier(operation.RequestID) || !validIdentifier(operation.CorrelationID) {
		return ErrInvalid
	}
	switch operation.Boundary {
	case providerregistry.OperationBoundaryInstallation:
		if !operation.TenantID.IsZero() {
			return ErrInvalid
		}
	case providerregistry.OperationBoundaryTenant:
		if operation.TenantID.IsZero() {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

// CertificateRequest is validated against the method-specific certificate
// capability before an adapter is invoked.
type CertificateRequest struct {
	Resolution providerregistry.Resolution
	Material   MaterialRef
	Operation  OperationContext
}

func (request CertificateRequest) ValidateServerChain() error {
	return request.validate(ServerChainLoadCapabilityKey, TLSServerIdentityPurposeKey)
}

func (request CertificateRequest) ValidateClientTrustBundle() error {
	return request.validate(ClientTrustBundleLoadCapabilityKey, MTLSClientTrustPurposeKey)
}

func (request CertificateRequest) validate(capabilityKey, purposeKey string) error {
	return validateRequest(request.Resolution, request.Material, request.Operation,
		CertificateServiceKey, capabilityKey, purposeKey)
}

// SigningKeyRequest names a non-exportable signing key used by a TLS
// handshake. Private key bytes never form part of this contract.
type SigningKeyRequest struct {
	Resolution providerregistry.Resolution
	Material   MaterialRef
	Operation  OperationContext
}

func (request SigningKeyRequest) Validate() error {
	return validateRequest(request.Resolution, request.Material, request.Operation,
		CryptographicKeyServiceKey, TLSHandshakeSignCapabilityKey, TLSHandshakePurposeKey)
}

// SecretRequest binds secret use to the operation purpose. Its registry
// capability must be exactly "...capability.<purpose>.use"; there is no
// generic secret.read capability.
type SecretRequest struct {
	Resolution providerregistry.Resolution
	Material   MaterialRef
	Operation  OperationContext
}

func (request SecretRequest) Validate() error {
	capabilityKey, err := SecretUseCapabilityKey(request.Operation.PurposeKey)
	if err != nil {
		return err
	}
	return validateRequest(request.Resolution, request.Material, request.Operation,
		SecretServiceKey, capabilityKey, request.Operation.PurposeKey)
}

// SecretUseCapabilityKey returns the only permitted secret-capability shape.
func SecretUseCapabilityKey(purposeKey string) (string, error) {
	if !resource.ValidKey(purposeKey) {
		return "", ErrInvalid
	}
	capabilityKey := SecretServiceKey + ".capability." + purposeKey + ".use"
	if !resource.ValidKey(capabilityKey) {
		return "", ErrInvalid
	}
	return capabilityKey, nil
}

func validateRequest(
	resolution providerregistry.Resolution,
	material MaterialRef,
	operation OperationContext,
	serviceKey string,
	capabilityKey string,
	purposeKey string,
) error {
	if resolution.Validate() != nil || material.Validate() != nil || operation.Validate() != nil ||
		resolution.ServiceKey != serviceKey || resolution.CapabilityKey != capabilityKey ||
		operation.PurposeKey != purposeKey ||
		resolution.OperationBoundary != operation.Boundary ||
		resolution.OperationTenantID != operation.TenantID {
		return ErrInvalid
	}
	return nil
}

// CertificateBundle contains only public certificate DER. Construction and
// access both copy byte slices so callers cannot mutate an adapter's retained
// input or the validated bundle through aliasing.
type CertificateBundle struct {
	material MaterialRef
	chainDER [][]byte
}

func NewCertificateBundle(material MaterialRef, chainDER [][]byte) (CertificateBundle, error) {
	if material.Validate() != nil || len(chainDER) == 0 {
		return CertificateBundle{}, ErrInvalid
	}
	if len(chainDER) > MaximumCertificateCount {
		return CertificateBundle{}, ErrMaterialLimit
	}

	total := 0
	for _, certificateDER := range chainDER {
		if len(certificateDER) == 0 {
			return CertificateBundle{}, ErrInvalid
		}
		if len(certificateDER) > MaximumCertificateDERBytes ||
			len(certificateDER) > MaximumCertificateBundleDERBytes-total {
			return CertificateBundle{}, ErrMaterialLimit
		}
		total += len(certificateDER)
	}

	cloned := make([][]byte, len(chainDER))
	for index, certificateDER := range chainDER {
		if _, err := x509.ParseCertificate(certificateDER); err != nil {
			return CertificateBundle{}, ErrInvalid
		}
		cloned[index] = append([]byte(nil), certificateDER...)
	}

	return CertificateBundle{material: material, chainDER: cloned}, nil
}

func (bundle CertificateBundle) Material() MaterialRef {
	return bundle.material
}

func (bundle CertificateBundle) ChainDER() [][]byte {
	cloned := make([][]byte, len(bundle.chainDER))
	for index, certificateDER := range bundle.chainDER {
		cloned[index] = append([]byte(nil), certificateDER...)
	}
	return cloned
}

// CertificateProvider loads public certificate material only.
type CertificateProvider interface {
	LoadServerChain(context.Context, CertificateRequest) (CertificateBundle, error)
	LoadClientTrustBundle(context.Context, CertificateRequest) (CertificateBundle, error)
}

// SigningInput is a bounded, defensively copied TLS signing payload with a
// closed set of signer options. Its zero value is invalid.
type SigningInput struct {
	payload []byte
	hash    crypto.Hash
	pss     bool
}

// NewTLSHandshakeSigningInput accepts the TLS 1.2/1.3 hash families used by
// RSA-PSS, ECDSA and Ed25519. RSA-PSS is fixed to a salt equal to the hash.
func NewTLSHandshakeSigningInput(payload []byte, options crypto.SignerOpts) (SigningInput, error) {
	if len(payload) == 0 || options == nil {
		return SigningInput{}, ErrInvalid
	}
	if len(payload) > MaximumSigningInputBytes {
		return SigningInput{}, ErrMaterialLimit
	}

	var hash crypto.Hash
	pss := false
	switch typed := options.(type) {
	case crypto.Hash:
		hash = typed
	case *rsa.PSSOptions:
		if typed == nil || typed.SaltLength != rsa.PSSSaltLengthEqualsHash {
			return SigningInput{}, ErrInvalid
		}
		hash = typed.Hash
		pss = true
	default:
		return SigningInput{}, ErrInvalid
	}

	switch hash {
	case crypto.Hash(0):
		if pss {
			return SigningInput{}, ErrInvalid
		}
	case crypto.SHA256, crypto.SHA384, crypto.SHA512:
		if len(payload) != hash.Size() {
			return SigningInput{}, ErrInvalid
		}
	default:
		return SigningInput{}, ErrInvalid
	}

	return SigningInput{payload: append([]byte(nil), payload...), hash: hash, pss: pss}, nil
}

func (input SigningInput) Validate() error {
	_, err := NewTLSHandshakeSigningInput(input.payload, input.SignerOpts())
	return err
}

func (input SigningInput) Bytes() []byte {
	return append([]byte(nil), input.payload...)
}

func (input SigningInput) SignerOpts() crypto.SignerOpts {
	if input.pss {
		return &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: input.hash}
	}
	return input.hash
}

// Signature is a bounded result bound to the exact request that produced it.
// Its zero value is invalid and its bytes are defensively copied.
type Signature struct {
	request SigningKeyRequest
	input   SigningInput
	value   []byte
}

func NewSignature(request SigningKeyRequest, input SigningInput, value []byte) (Signature, error) {
	if request.Validate() != nil || input.Validate() != nil || len(value) == 0 {
		return Signature{}, ErrInvalid
	}
	if len(value) > MaximumSignatureBytes {
		return Signature{}, ErrMaterialLimit
	}
	return Signature{request: request, input: input, value: append([]byte(nil), value...)}, nil
}

func (signature Signature) Validate() error {
	if signature.request.Validate() != nil || signature.input.Validate() != nil || len(signature.value) == 0 {
		return ErrInvalid
	}
	if len(signature.value) > MaximumSignatureBytes {
		return ErrMaterialLimit
	}
	return nil
}

func (signature Signature) Material() MaterialRef {
	return signature.request.Material
}

func (signature Signature) Resolution() providerregistry.Resolution {
	return signature.request.Resolution
}

func (signature Signature) Input() SigningInput {
	return signature.input
}

func (signature Signature) Bytes() []byte {
	return append([]byte(nil), signature.value...)
}

// PublicKeyMaterial is a bounded PKIX SubjectPublicKeyInfo document bound to
// the exact request that produced it. Only TLS-capable RSA, ECDSA and Ed25519
// public keys can be constructed; private-key values are rejected before
// serialization.
type PublicKeyMaterial struct {
	request SigningKeyRequest
	der     []byte
}

func NewPublicKeyMaterial(request SigningKeyRequest, publicKey crypto.PublicKey) (PublicKeyMaterial, error) {
	if request.Validate() != nil {
		return PublicKeyMaterial{}, ErrInvalid
	}
	switch typed := publicKey.(type) {
	case *rsa.PublicKey:
		if typed == nil {
			return PublicKeyMaterial{}, ErrInvalid
		}
	case *ecdsa.PublicKey:
		if typed == nil {
			return PublicKeyMaterial{}, ErrInvalid
		}
	case ed25519.PublicKey:
		if len(typed) != ed25519.PublicKeySize {
			return PublicKeyMaterial{}, ErrInvalid
		}
	default:
		return PublicKeyMaterial{}, ErrInvalid
	}
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return PublicKeyMaterial{}, ErrInvalid
	}
	if len(der) > MaximumPublicKeyDERBytes {
		return PublicKeyMaterial{}, ErrMaterialLimit
	}
	return PublicKeyMaterial{request: request, der: append([]byte(nil), der...)}, nil
}

func (material PublicKeyMaterial) Validate() error {
	if material.request.Validate() != nil || len(material.der) == 0 {
		return ErrInvalid
	}
	if len(material.der) > MaximumPublicKeyDERBytes {
		return ErrMaterialLimit
	}
	parsed, err := x509.ParsePKIXPublicKey(material.der)
	if err != nil {
		return ErrInvalid
	}
	_, err = NewPublicKeyMaterial(material.request, parsed)
	return err
}

func (material PublicKeyMaterial) Material() MaterialRef {
	return material.request.Material
}

func (material PublicKeyMaterial) Resolution() providerregistry.Resolution {
	return material.request.Resolution
}

func (material PublicKeyMaterial) PKIXDER() []byte {
	return append([]byte(nil), material.der...)
}

func (material PublicKeyMaterial) Parse() (crypto.PublicKey, error) {
	if err := material.Validate(); err != nil {
		return nil, err
	}
	return x509.ParsePKIXPublicKey(material.der)
}

// SigningKeyProvider performs each public-key lookup and signature with the
// complete request. Consumers must obtain a fresh registry Resolution before
// each security-relevant call; no long-lived signer can outlive that check.
type SigningKeyProvider interface {
	PublicKey(context.Context, SigningKeyRequest) (PublicKeyMaterial, error)
	Sign(context.Context, SigningKeyRequest, SigningInput) (Signature, error)
}

// SecretConsumer receives secret bytes only for the duration of a provider
// call. It must not retain the slice or expose it through logs, errors or
// asynchronous work.
type SecretConsumer func([]byte) error

// SecretProvider makes material available only to an exact, purpose-bound
// callback. It never returns secret bytes as a result. Implementations must
// invalidate their transient byte view when UseSecret returns, reject a view
// larger than MaximumSecretBytes, and honor context cancellation before and
// during material access.
type SecretProvider interface {
	UseSecret(context.Context, SecretRequest, SecretConsumer) error
}

func validOpaqueHandle(value string) bool {
	if value == "" || len(value) > MaximumMaterialHandleBytes || !utf8.ValidString(value) ||
		strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func validIdentifier(value string) bool {
	if value == "" || len(value) > MaximumOperationIdentifierBytes || !utf8.ValidString(value) ||
		strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}
