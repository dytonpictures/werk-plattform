// Package providertransport provides a fail-closed HTTP egress boundary for
// external identity and directory providers. It deliberately owns URL, DNS,
// address, redirect, proxy, TLS, timeout, and response-size policy so protocol
// adapters cannot accidentally fall back to net/http defaults.
package providertransport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidConfiguration = errors.New("invalid provider transport configuration")
	ErrURLRejected          = errors.New("provider URL rejected")
	ErrAddressBlocked       = errors.New("provider address blocked")
	ErrRedirectRejected     = errors.New("provider redirect rejected")
	ErrResponseBodyTooLarge = errors.New("provider response body too large")
)

const (
	DefaultDialTimeout           = 5 * time.Second
	DefaultTLSHandshakeTimeout   = 5 * time.Second
	DefaultResponseHeaderTimeout = 10 * time.Second
	DefaultRequestTimeout        = 30 * time.Second
	DefaultIdleConnectionTimeout = 30 * time.Second
	DefaultResponseBodyLimit     = int64(1 << 20)
	MaximumResponseBodyLimit     = int64(16 << 20)

	maximumDialTimeout           = 30 * time.Second
	maximumTLSHandshakeTimeout   = 30 * time.Second
	maximumResponseHeaderTimeout = 30 * time.Second
	maximumRequestTimeout        = 2 * time.Minute
	maximumIdleConnectionTimeout = 2 * time.Minute
	maximumRedirects             = 10
	maximumResponseHeaderBytes   = int64(1 << 20)
)

// Resolver is intentionally narrower than net.Resolver and is invoked for
// every new network dial. Returning netip.Addr values lets the policy validate
// the exact address that is subsequently passed to the dialer.
type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

// ContextDialer is implemented by net.Dialer. Implementations receive only a
// numeric address that has already passed the address policy.
type ContextDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

type Timeouts struct {
	Dial           time.Duration
	TLSHandshake   time.Duration
	ResponseHeader time.Duration
	Request        time.Duration
	IdleConnection time.Duration
}

// Config requires callers to name every usable URL scheme. CIDRAllowlist is an
// explicit exception list for addresses that are blocked by default, such as a
// deliberately operated private-network identity provider. Public global
// unicast addresses do not require an allowlist entry.
type Config struct {
	AllowedSchemes    []string
	CIDRAllowlist     []netip.Prefix
	MaxRedirects      int
	ResponseBodyLimit int64
	Timeouts          Timeouts
	TLSConfig         *tls.Config
	Resolver          Resolver
	Dialer            ContextDialer
}

// Client keeps the underlying http.Client and transport private so callers
// cannot replace the proxy, dialer, TLS, or redirect policy after validation.
type Client struct {
	httpClient *http.Client
	policy     *egressPolicy
}

type egressPolicy struct {
	allowedSchemes map[string]struct{}
	allowlist      []netip.Prefix
	resolver       Resolver
	dialer         ContextDialer
}

// NewClient constructs a bounded client. Zero timeout and response-limit
// values select safe defaults; negative or excessively large values fail.
func NewClient(config Config) (*Client, error) {
	policy, err := newEgressPolicy(config)
	if err != nil {
		return nil, err
	}
	timeouts, err := normalizeTimeouts(config.Timeouts)
	if err != nil {
		return nil, err
	}
	bodyLimit := config.ResponseBodyLimit
	if bodyLimit == 0 {
		bodyLimit = DefaultResponseBodyLimit
	}
	if bodyLimit < 1 || bodyLimit > MaximumResponseBodyLimit {
		return nil, ErrInvalidConfiguration
	}
	if config.MaxRedirects < 0 || config.MaxRedirects > maximumRedirects {
		return nil, ErrInvalidConfiguration
	}
	tlsConfig, err := secureTLSConfig(config.TLSConfig)
	if err != nil {
		return nil, err
	}

	base := &http.Transport{
		Proxy:                  nil,
		DialContext:            policy.dialContext,
		ForceAttemptHTTP2:      true,
		TLSClientConfig:        tlsConfig,
		TLSHandshakeTimeout:    timeouts.TLSHandshake,
		ResponseHeaderTimeout:  timeouts.ResponseHeader,
		ExpectContinueTimeout:  time.Second,
		IdleConnTimeout:        timeouts.IdleConnection,
		MaxIdleConns:           32,
		MaxIdleConnsPerHost:    4,
		MaxConnsPerHost:        16,
		MaxResponseHeaderBytes: maximumResponseHeaderBytes,
		WriteBufferSize:        32 << 10,
		ReadBufferSize:         32 << 10,
	}
	guarded := &guardedRoundTripper{policy: policy, base: base, bodyLimit: bodyLimit}
	client := &Client{policy: policy}
	client.httpClient = &http.Client{
		Transport: guarded,
		Timeout:   timeouts.Request,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if config.MaxRedirects == 0 || len(via) > config.MaxRedirects {
				return ErrRedirectRejected
			}
			if request == nil || request.URL == nil {
				return ErrRedirectRejected
			}
			if err := policy.validateURL(request.URL); err != nil {
				return errors.Join(ErrRedirectRejected, err)
			}
			if forbiddenHostOverride(request) {
				return ErrRedirectRejected
			}
			if len(via) > 0 && strings.EqualFold(via[len(via)-1].URL.Scheme, "https") &&
				!strings.EqualFold(request.URL.Scheme, "https") {
				return ErrRedirectRejected
			}
			return nil
		},
	}
	return client, nil
}

// ValidateURL parses and validates a provider URL without performing DNS or
// network I/O. Numeric hosts are additionally checked against the IP policy.
func (client *Client) ValidateURL(rawURL string) (*url.URL, error) {
	if client == nil || client.policy == nil || rawURL == "" || strings.TrimSpace(rawURL) != rawURL {
		return nil, ErrURLRejected
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || client.policy.validateURL(parsed) != nil {
		return nil, ErrURLRejected
	}
	return parsed, nil
}

// Do executes one request through the immutable egress policy.
func (client *Client) Do(request *http.Request) (*http.Response, error) {
	if client == nil || client.httpClient == nil || request == nil {
		return nil, ErrInvalidConfiguration
	}
	return client.httpClient.Do(request)
}

// HTTPClient returns a shallow client clone for libraries, such as OIDC and
// OAuth2 implementations, that require a concrete *http.Client. The clone
// starts with the same private guarded RoundTripper, timeout, and redirect
// function, while changes to its public fields cannot mutate the canonical
// client held here. The connection pool is intentionally shared.
//
// A concrete http.Client is mutable by design. Trusted composition code must
// pass this clone directly to the protocol library and must not replace its
// Transport or CheckRedirect fields; doing so can weaken that clone's policy,
// although it still cannot weaken the canonical client or another clone.
func (client *Client) HTTPClient() *http.Client {
	if client == nil || client.httpClient == nil {
		return nil
	}
	clone := *client.httpClient
	return &clone
}

// CloseIdleConnections releases pooled connections. DNS and address policy are
// evaluated again when a later request creates a new connection.
func (client *Client) CloseIdleConnections() {
	if client != nil && client.httpClient != nil {
		client.httpClient.CloseIdleConnections()
	}
}

type guardedRoundTripper struct {
	policy    *egressPolicy
	base      http.RoundTripper
	bodyLimit int64
}

func (transport *guardedRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if transport == nil || transport.policy == nil || transport.base == nil || request == nil || request.URL == nil {
		return nil, ErrInvalidConfiguration
	}
	if forbiddenHostOverride(request) {
		return nil, ErrURLRejected
	}
	if err := transport.policy.validateURL(request.URL); err != nil {
		return nil, err
	}
	response, err := transport.base.RoundTrip(request)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, err
	}
	if response == nil || response.Body == nil {
		return nil, errors.New("provider returned an invalid HTTP response")
	}
	if response.ContentLength > transport.bodyLimit {
		_ = response.Body.Close()
		return nil, ErrResponseBodyTooLarge
	}
	response.Body = &hardLimitReadCloser{inner: response.Body, remaining: transport.bodyLimit}
	return response, nil
}

func (transport *guardedRoundTripper) CloseIdleConnections() {
	if transport == nil || transport.base == nil {
		return
	}
	if closer, ok := transport.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func newEgressPolicy(config Config) (*egressPolicy, error) {
	if len(config.AllowedSchemes) == 0 {
		return nil, ErrInvalidConfiguration
	}
	allowedSchemes := make(map[string]struct{}, len(config.AllowedSchemes))
	for _, configured := range config.AllowedSchemes {
		scheme := strings.ToLower(configured)
		if configured == "" || strings.TrimSpace(configured) != configured || !validScheme(scheme) ||
			(scheme != "http" && scheme != "https") {
			return nil, ErrInvalidConfiguration
		}
		allowedSchemes[scheme] = struct{}{}
	}

	allowlist := make([]netip.Prefix, 0, len(config.CIDRAllowlist))
	for _, prefix := range config.CIDRAllowlist {
		normalized, ok := normalizePrefix(prefix)
		if !ok {
			return nil, ErrInvalidConfiguration
		}
		allowlist = append(allowlist, normalized)
	}
	resolver := config.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dialer := config.Dialer
	if dialer == nil {
		dialTimeout := config.Timeouts.Dial
		if dialTimeout == 0 {
			dialTimeout = DefaultDialTimeout
		}
		if dialTimeout < 0 || dialTimeout > maximumDialTimeout {
			return nil, ErrInvalidConfiguration
		}
		dialer = &net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}
	}
	return &egressPolicy{
		allowedSchemes: allowedSchemes,
		allowlist:      allowlist,
		resolver:       resolver,
		dialer:         dialer,
	}, nil
}

func (policy *egressPolicy) validateURL(candidate *url.URL) error {
	if policy == nil || candidate == nil || candidate.Scheme == "" || candidate.Host == "" ||
		candidate.Opaque != "" || candidate.User != nil || candidate.Fragment != "" || candidate.RawFragment != "" {
		return ErrURLRejected
	}
	scheme := strings.ToLower(candidate.Scheme)
	if scheme != candidate.Scheme {
		return ErrURLRejected
	}
	if _, allowed := policy.allowedSchemes[scheme]; !allowed {
		return ErrURLRejected
	}
	host := candidate.Hostname()
	if host == "" || strings.TrimSpace(host) != host {
		return ErrURLRejected
	}
	if port := candidate.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return ErrURLRejected
		}
	}
	if address, err := netip.ParseAddr(host); err == nil {
		if err := policy.validateIP(address); err != nil {
			return errors.Join(ErrURLRejected, err)
		}
	}
	return nil
}

func (policy *egressPolicy) validateIP(address netip.Addr) error {
	if !address.IsValid() {
		return ErrAddressBlocked
	}
	address = address.Unmap()
	if address.Zone() != "" {
		address = address.WithZone("")
	}
	for _, prefix := range policy.allowlist {
		if prefix.Contains(address) {
			return nil
		}
	}
	if address.IsUnspecified() || address.IsMulticast() || address.IsLoopback() ||
		address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsPrivate() ||
		!address.IsGlobalUnicast() {
		return ErrAddressBlocked
	}
	return nil
}

func (policy *egressPolicy) dialContext(ctx context.Context, network, target string) (net.Conn, error) {
	if policy == nil || policy.resolver == nil || policy.dialer == nil ||
		(network != "tcp" && network != "tcp4" && network != "tcp6") {
		return nil, ErrInvalidConfiguration
	}
	host, port, err := net.SplitHostPort(target)
	if err != nil || host == "" || port == "" {
		return nil, ErrURLRejected
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, ErrURLRejected
	}

	addresses := make([]netip.Addr, 0, 4)
	if literal, parseErr := netip.ParseAddr(host); parseErr == nil {
		addresses = append(addresses, literal)
	} else {
		addresses, err = policy.resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("provider DNS lookup failed: %w", err)
		}
	}
	if len(addresses) == 0 {
		return nil, ErrAddressBlocked
	}

	var dialErrors []error
	permitted := 0
	seen := make(map[netip.Addr]struct{}, len(addresses))
	for _, address := range addresses {
		if !address.IsValid() {
			continue
		}
		identity := address.Unmap()
		if _, duplicate := seen[identity]; duplicate {
			continue
		}
		seen[identity] = struct{}{}
		if err := policy.validateIP(address); err != nil {
			continue
		}
		permitted++
		directTarget := net.JoinHostPort(address.String(), port)
		connection, err := policy.dialer.DialContext(ctx, network, directTarget)
		if err == nil {
			return connection, nil
		}
		dialErrors = append(dialErrors, err)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	if permitted == 0 {
		return nil, ErrAddressBlocked
	}
	return nil, fmt.Errorf("provider dial failed: %w", errors.Join(dialErrors...))
}

func secureTLSConfig(config *tls.Config) (*tls.Config, error) {
	if config == nil {
		return &tls.Config{MinVersion: tls.VersionTLS12}, nil
	}
	if config.InsecureSkipVerify || config.ServerName != "" ||
		(config.MinVersion != 0 && config.MinVersion < tls.VersionTLS12) {
		return nil, ErrInvalidConfiguration
	}
	clone := config.Clone()
	clone.InsecureSkipVerify = false
	clone.ServerName = ""
	if config.RootCAs != nil {
		clone.RootCAs = config.RootCAs.Clone()
	}
	if clone.MinVersion == 0 {
		clone.MinVersion = tls.VersionTLS12
	}
	if clone.MaxVersion != 0 && clone.MaxVersion < clone.MinVersion {
		return nil, ErrInvalidConfiguration
	}
	return clone, nil
}

func normalizeTimeouts(configured Timeouts) (Timeouts, error) {
	var result Timeouts
	var err error
	if result.Dial, err = boundedDuration(configured.Dial, DefaultDialTimeout, maximumDialTimeout); err != nil {
		return Timeouts{}, err
	}
	if result.TLSHandshake, err = boundedDuration(configured.TLSHandshake, DefaultTLSHandshakeTimeout, maximumTLSHandshakeTimeout); err != nil {
		return Timeouts{}, err
	}
	if result.ResponseHeader, err = boundedDuration(configured.ResponseHeader, DefaultResponseHeaderTimeout, maximumResponseHeaderTimeout); err != nil {
		return Timeouts{}, err
	}
	if result.Request, err = boundedDuration(configured.Request, DefaultRequestTimeout, maximumRequestTimeout); err != nil {
		return Timeouts{}, err
	}
	if result.IdleConnection, err = boundedDuration(configured.IdleConnection, DefaultIdleConnectionTimeout, maximumIdleConnectionTimeout); err != nil {
		return Timeouts{}, err
	}
	return result, nil
}

func boundedDuration(value, fallback, maximum time.Duration) (time.Duration, error) {
	if value == 0 {
		return fallback, nil
	}
	if value < 0 || value > maximum {
		return 0, ErrInvalidConfiguration
	}
	return value, nil
}

func normalizePrefix(prefix netip.Prefix) (netip.Prefix, bool) {
	if !prefix.IsValid() || prefix.Addr().Zone() != "" {
		return netip.Prefix{}, false
	}
	if prefix.Addr().Is4In6() {
		if prefix.Bits() < 96 {
			return netip.Prefix{}, false
		}
		return netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96).Masked(), true
	}
	return prefix.Masked(), true
}

func validScheme(scheme string) bool {
	for index, character := range scheme {
		if (character >= 'a' && character <= 'z') ||
			(index > 0 && character >= '0' && character <= '9') ||
			(index > 0 && (character == '+' || character == '-' || character == '.')) {
			continue
		}
		return false
	}
	return scheme != ""
}

func forbiddenHostOverride(request *http.Request) bool {
	return request != nil && request.Host != "" && request.URL != nil &&
		!strings.EqualFold(request.Host, request.URL.Host)
}

type hardLimitReadCloser struct {
	inner     io.ReadCloser
	remaining int64
}

func (reader *hardLimitReadCloser) Read(buffer []byte) (int, error) {
	if reader == nil || reader.inner == nil {
		return 0, io.ErrClosedPipe
	}
	if len(buffer) == 0 {
		return 0, nil
	}
	if reader.remaining == 0 {
		var probe [1]byte
		count, err := reader.inner.Read(probe[:])
		if count > 0 {
			return 0, ErrResponseBodyTooLarge
		}
		return 0, err
	}
	allowed := int64(len(buffer))
	if allowed > reader.remaining+1 {
		allowed = reader.remaining + 1
	}
	count, err := reader.inner.Read(buffer[:allowed])
	if int64(count) > reader.remaining {
		visible := int(reader.remaining)
		reader.remaining = 0
		return visible, ErrResponseBodyTooLarge
	}
	reader.remaining -= int64(count)
	return count, err
}

func (reader *hardLimitReadCloser) Close() error {
	if reader == nil || reader.inner == nil {
		return nil
	}
	return reader.inner.Close()
}

// ReadResponseBody reads and closes a response with an endpoint-specific limit.
// The client-wide hard limit remains active underneath this helper. No partial
// body is returned when either limit is exceeded.
func ReadResponseBody(response *http.Response, limit int64) (body []byte, err error) {
	if response == nil || response.Body == nil {
		return nil, ErrInvalidConfiguration
	}
	defer func() {
		if closeErr := response.Body.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	if limit < 1 || limit >= math.MaxInt64 || limit > MaximumResponseBodyLimit {
		return nil, ErrInvalidConfiguration
	}
	if response.ContentLength > limit {
		return nil, ErrResponseBodyTooLarge
	}
	limited := io.LimitReader(response.Body, limit+1)
	body, err = io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, ErrResponseBodyTooLarge
	}
	return body, nil
}
