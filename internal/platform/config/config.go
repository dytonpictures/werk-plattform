package config

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dytonpictures/werk/internal/platform/transportsecurity"
)

type Config struct {
	Environment             string
	HTTPAddress             string
	HTTPServerTLS           transportsecurity.ServerOptions
	HTTPTrustedProxyCIDRs   []netip.Prefix
	DatabaseURL             string
	IdentityDatabaseURL     string
	AdminDatabaseURL        string
	BootstrapAdminPassword  string
	DevelopmentWorkPassword string
	IdentityMFAEnabled      bool
	IdentityMFAKey          []byte
	IdentityMFACurrentKeyID string
	IdentityMFAKeys         map[string][]byte
	AllowedOrigins          []string
	WorkerConcurrency       int
	BuildVersion            string
	Kafka                   KafkaConfig
}

type KafkaConfig struct {
	Enabled            bool
	Brokers            []string
	ClientID           string
	DomainEventsTopic  string
	SecurityAuditTopic string
	RuntimeLogsTopic   string
	TLS                bool
	TLSCAFile          string
	TLSCertFile        string
	TLSKeyFile         string
	TLSServerName      string
	SASLMechanism      string
	SASLUsername       string
	SASLPassword       string
	PublishTimeout     time.Duration
	AuditConcurrency   int
}

func Load() (Config, error) {
	return load(os.LookupEnv)
}

// LoadAPI loads shared platform configuration and additionally enforces the
// native HTTP server transport requirements. Non-server commands intentionally
// use Load so they never need access to a server private key.
func LoadAPI() (Config, error) {
	return loadAPI(os.LookupEnv)
}

func NewLogger(cfg Config, component ...string) *slog.Logger {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level(cfg.Environment),
	}))
	service := "platform"
	if len(component) > 0 && stableConfigKey(component[0]) {
		service = component[0]
	}
	return logger.With(
		"service", service,
		"environment", cfg.Environment,
		"build_version", cfg.BuildVersion,
	)
}

type lookupEnvironment func(string) (string, bool)

func load(lookup lookupEnvironment) (Config, error) {
	environment := value(lookup, "WERK_ENV", "development")
	if environment != "development" && environment != "test" && environment != "production" {
		return Config{}, fmt.Errorf("WERK_ENV must be development, test, or production")
	}

	httpAddress := value(lookup, "WERK_HTTP_ADDRESS", ":8080")
	if err := validateHTTPAddress(httpAddress); err != nil {
		return Config{}, err
	}
	httpServerTLS, err := loadHTTPServerTLS(lookup)
	if err != nil {
		return Config{}, err
	}
	httpTrustedProxyCIDRs, err := parseTrustedProxyCIDRs(value(lookup, "WERK_HTTP_TRUSTED_PROXY_CIDRS", ""))
	if err != nil {
		return Config{}, err
	}

	databaseURL, databaseURLIsSet := lookup("DATABASE_URL")
	databaseURL = strings.TrimSpace(databaseURL)
	if databaseURL == "" {
		if environment == "production" {
			return Config{}, fmt.Errorf("DATABASE_URL is required in production")
		}
		databaseURL = "postgres://werk:werk@localhost:5432/werk?sslmode=disable"
		databaseURLIsSet = false
	}
	if err := validateDatabaseURL(databaseURL); err != nil {
		return Config{}, err
	}
	if environment == "production" {
		if err := validateProductionDatabaseCredentials(databaseURL); err != nil {
			return Config{}, err
		}
	}
	if environment == "production" && !databaseURLIsSet {
		return Config{}, fmt.Errorf("DATABASE_URL is required in production")
	}

	identityDatabaseURL, identityDatabaseURLIsSet := lookup("IDENTITY_DATABASE_URL")
	identityDatabaseURL = strings.TrimSpace(identityDatabaseURL)
	if identityDatabaseURL == "" {
		if environment == "production" {
			return Config{}, fmt.Errorf("IDENTITY_DATABASE_URL is required in production")
		}
		identityDatabaseURL = "postgres://werk_identity_runtime:werk-identity-dev@localhost:5432/werk?sslmode=disable"
		identityDatabaseURLIsSet = false
	}
	if err := validateDatabaseURL(identityDatabaseURL); err != nil {
		return Config{}, fmt.Errorf("IDENTITY_DATABASE_URL: %w", err)
	}
	if environment == "production" {
		if err := validateProductionDatabaseCredentials(identityDatabaseURL); err != nil {
			return Config{}, fmt.Errorf("IDENTITY_DATABASE_URL: %w", err)
		}
	}
	if environment == "production" && !identityDatabaseURLIsSet {
		return Config{}, fmt.Errorf("IDENTITY_DATABASE_URL is required in production")
	}
	adminDatabaseURL, adminDatabaseURLIsSet := lookup("ADMIN_DATABASE_URL")
	adminDatabaseURL = strings.TrimSpace(adminDatabaseURL)
	if adminDatabaseURL == "" {
		if environment == "production" {
			return Config{}, fmt.Errorf("ADMIN_DATABASE_URL is required in production")
		}
		adminDatabaseURL = "postgres://werk_admin_runtime:werk-admin-dev@localhost:5432/werk?sslmode=disable"
		adminDatabaseURLIsSet = false
	}
	if err := validateDatabaseURL(adminDatabaseURL); err != nil {
		return Config{}, fmt.Errorf("ADMIN_DATABASE_URL: %w", err)
	}
	if environment == "production" {
		if err := validateProductionDatabaseCredentials(adminDatabaseURL); err != nil {
			return Config{}, fmt.Errorf("ADMIN_DATABASE_URL: %w", err)
		}
		if !adminDatabaseURLIsSet {
			return Config{}, fmt.Errorf("ADMIN_DATABASE_URL is required in production")
		}
	}
	bootstrapAdminPassword := value(lookup, "WERK_BOOTSTRAP_ADMIN_PASSWORD", "")
	if environment == "development" && bootstrapAdminPassword == "" {
		bootstrapAdminPassword = "werk-development"
	}
	if bootstrapAdminPassword != "" && len(bootstrapAdminPassword) < 16 {
		return Config{}, fmt.Errorf("WERK_BOOTSTRAP_ADMIN_PASSWORD must contain at least 16 characters")
	}
	developmentWorkPassword := value(lookup, "WERK_DEV_WORKER_PASSWORD", "")
	if environment == "development" && developmentWorkPassword == "" {
		developmentWorkPassword = "werk-worker-development"
	}
	if developmentWorkPassword != "" && environment != "development" {
		return Config{}, fmt.Errorf("WERK_DEV_WORKER_PASSWORD is only allowed in development")
	}
	if developmentWorkPassword != "" && len(developmentWorkPassword) < 16 {
		return Config{}, fmt.Errorf("WERK_DEV_WORKER_PASSWORD must contain at least 16 characters")
	}
	identityMFAEnabled := environment == "development"
	if configured, ok := lookup("WERK_IDENTITY_MFA_ENABLED"); ok && strings.TrimSpace(configured) != "" {
		parsed, err := strconv.ParseBool(strings.TrimSpace(configured))
		if err != nil {
			return Config{}, fmt.Errorf("WERK_IDENTITY_MFA_ENABLED must be a boolean")
		}
		identityMFAEnabled = parsed
	}
	var identityMFAKey []byte
	identityMFACurrentKeyID := ""
	var identityMFAKeys map[string][]byte
	if identityMFAEnabled {
		defaultKey := ""
		if environment == "development" {
			defaultKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY" // gitleaks:allow -- development-only key rejected in production
		}
		encodedKey, legacyKeyConfigured := lookup("WERK_IDENTITY_MFA_KEY")
		encodedKey = strings.TrimSpace(encodedKey)
		encodedKeyring, keyringConfigured := lookup("WERK_IDENTITY_MFA_KEYS")
		encodedKeyring = strings.TrimSpace(encodedKeyring)
		if keyringConfigured && encodedKeyring != "" {
			if legacyKeyConfigured && encodedKey != "" {
				return Config{}, fmt.Errorf("WERK_IDENTITY_MFA_KEY and WERK_IDENTITY_MFA_KEYS cannot be combined")
			}
			identityMFACurrentKeyID = value(lookup, "WERK_IDENTITY_MFA_CURRENT_KEY_ID", "")
			parsedKeys, parseErr := parseMFAKeyring(encodedKeyring)
			identityMFAKeys = parsedKeys
			if parseErr != nil || len(identityMFAKeys[identityMFACurrentKeyID]) != 32 {
				return Config{}, fmt.Errorf("WERK_IDENTITY_MFA_KEYS requires valid named 32-byte keys and a current key ID")
			}
			identityMFAKey = append([]byte(nil), identityMFAKeys[identityMFACurrentKeyID]...)
		} else {
			if encodedKey == "" {
				encodedKey = defaultKey
			}
			decoded, err := base64.RawStdEncoding.DecodeString(encodedKey)
			if err != nil || len(decoded) != 32 {
				return Config{}, fmt.Errorf("WERK_IDENTITY_MFA_KEY must be an unpadded base64-encoded 32-byte key when MFA is enabled")
			}
			identityMFAKey = decoded
			identityMFACurrentKeyID = "default"
			identityMFAKeys = map[string][]byte{"default": append([]byte(nil), decoded...)}
		}
	}
	if environment == "production" && !identityMFAEnabled {
		return Config{}, fmt.Errorf("WERK_IDENTITY_MFA_ENABLED must be true in production")
	}
	allowedOriginsValue := value(lookup, "WERK_ALLOWED_ORIGINS", "")
	if allowedOriginsValue == "" && environment != "production" {
		allowedOriginsValue = "http://localhost:3000,http://127.0.0.1:3000"
	}
	if allowedOriginsValue == "" {
		return Config{}, fmt.Errorf("WERK_ALLOWED_ORIGINS is required in production")
	}
	allowedOrigins := make([]string, 0, 2)
	for _, candidate := range strings.Split(allowedOriginsValue, ",") {
		origin := strings.TrimSpace(candidate)
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" ||
			(environment == "production" && parsed.Scheme != "https") {
			return Config{}, fmt.Errorf("WERK_ALLOWED_ORIGINS contains an invalid origin")
		}
		allowedOrigins = append(allowedOrigins, origin)
	}
	workerConcurrency, err := strconv.Atoi(value(lookup, "WERK_WORKER_CONCURRENCY", "4"))
	if err != nil || workerConcurrency < 1 || workerConcurrency > 128 {
		return Config{}, fmt.Errorf("WERK_WORKER_CONCURRENCY must be between 1 and 128")
	}
	kafka, err := loadKafkaConfig(lookup, environment)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Environment:             environment,
		HTTPAddress:             httpAddress,
		HTTPServerTLS:           httpServerTLS,
		HTTPTrustedProxyCIDRs:   httpTrustedProxyCIDRs,
		DatabaseURL:             databaseURL,
		IdentityDatabaseURL:     identityDatabaseURL,
		AdminDatabaseURL:        adminDatabaseURL,
		BootstrapAdminPassword:  bootstrapAdminPassword,
		DevelopmentWorkPassword: developmentWorkPassword,
		IdentityMFAEnabled:      identityMFAEnabled,
		IdentityMFAKey:          identityMFAKey,
		IdentityMFACurrentKeyID: identityMFACurrentKeyID,
		IdentityMFAKeys:         identityMFAKeys,
		AllowedOrigins:          allowedOrigins,
		WorkerConcurrency:       workerConcurrency,
		BuildVersion:            value(lookup, "WERK_BUILD_VERSION", "dev"),
		Kafka:                   kafka,
	}, nil
}

func loadAPI(lookup lookupEnvironment) (Config, error) {
	configuration, err := load(lookup)
	if err != nil {
		return Config{}, err
	}
	if configuration.Environment == "production" && !configuration.HTTPServerTLS.Enabled() {
		return Config{}, fmt.Errorf("WERK_HTTP_TLS_MODE must be tls or mtls for the production API server")
	}
	return configuration, nil
}

func loadHTTPServerTLS(lookup lookupEnvironment) (transportsecurity.ServerOptions, error) {
	options := transportsecurity.ServerOptions{
		Mode:            transportsecurity.ServerMode(strings.ToLower(value(lookup, "WERK_HTTP_TLS_MODE", string(transportsecurity.ServerDisabled)))),
		CertificateFile: value(lookup, "WERK_HTTP_TLS_CERT_FILE", ""),
		PrivateKeyFile:  value(lookup, "WERK_HTTP_TLS_KEY_FILE", ""),
		ClientCAFile:    value(lookup, "WERK_HTTP_TLS_CLIENT_CA_FILE", ""),
	}
	if err := options.Validate(); err != nil {
		return transportsecurity.ServerOptions{}, fmt.Errorf("invalid HTTP server TLS configuration: %w", err)
	}
	return options, nil
}

func parseTrustedProxyCIDRs(configured string) ([]netip.Prefix, error) {
	if configured == "" {
		return nil, nil
	}
	prefixes := make([]netip.Prefix, 0, 2)
	for _, entry := range strings.Split(configured, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(entry))
		if err != nil || prefix.Bits() == 0 {
			return nil, fmt.Errorf("WERK_HTTP_TRUSTED_PROXY_CIDRS contains an invalid network")
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func loadKafkaConfig(lookup lookupEnvironment, environment string) (KafkaConfig, error) {
	enabled, err := booleanValue(lookup, "WERK_KAFKA_ENABLED", false)
	if err != nil {
		return KafkaConfig{}, err
	}
	tlsEnabled, err := booleanValue(lookup, "WERK_KAFKA_TLS", false)
	if err != nil {
		return KafkaConfig{}, err
	}
	brokersValue := value(lookup, "WERK_KAFKA_BROKERS", "127.0.0.1:59092")
	brokers := make([]string, 0, 3)
	for _, candidate := range strings.Split(brokersValue, ",") {
		broker := strings.TrimSpace(candidate)
		if broker == "" || strings.Contains(broker, "://") {
			return KafkaConfig{}, fmt.Errorf("WERK_KAFKA_BROKERS must contain host:port addresses without a URL scheme")
		}
		host, port, splitErr := net.SplitHostPort(broker)
		portNumber, portErr := strconv.Atoi(port)
		if splitErr != nil || host == "" || portErr != nil || portNumber < 1 || portNumber > 65535 {
			return KafkaConfig{}, fmt.Errorf("WERK_KAFKA_BROKERS contains an invalid broker address")
		}
		brokers = append(brokers, broker)
	}
	if len(brokers) == 0 {
		return KafkaConfig{}, fmt.Errorf("WERK_KAFKA_BROKERS must contain at least one broker")
	}

	topics := []struct {
		name  string
		value string
	}{
		{"WERK_KAFKA_DOMAIN_EVENTS_TOPIC", value(lookup, "WERK_KAFKA_DOMAIN_EVENTS_TOPIC", "platform.domain-events.v1")},
		{"WERK_KAFKA_SECURITY_AUDIT_TOPIC", value(lookup, "WERK_KAFKA_SECURITY_AUDIT_TOPIC", "platform.security-audit.v1")},
		{"WERK_KAFKA_RUNTIME_LOGS_TOPIC", value(lookup, "WERK_KAFKA_RUNTIME_LOGS_TOPIC", "platform.runtime-logs.v1")},
	}
	for _, topic := range topics {
		if !validKafkaName(topic.value, 249) {
			return KafkaConfig{}, fmt.Errorf("%s contains an invalid topic name", topic.name)
		}
	}

	clientID := value(lookup, "WERK_KAFKA_CLIENT_ID", "platform-"+environment)
	if !validKafkaName(clientID, 128) {
		return KafkaConfig{}, fmt.Errorf("WERK_KAFKA_CLIENT_ID contains an invalid client ID")
	}
	mechanism := strings.ToLower(value(lookup, "WERK_KAFKA_SASL_MECHANISM", "none"))
	if mechanism != "none" && mechanism != "plain" && mechanism != "scram-sha-256" && mechanism != "scram-sha-512" {
		return KafkaConfig{}, fmt.Errorf("WERK_KAFKA_SASL_MECHANISM must be none, plain, scram-sha-256, or scram-sha-512")
	}
	username := value(lookup, "WERK_KAFKA_SASL_USERNAME", "")
	password := value(lookup, "WERK_KAFKA_SASL_PASSWORD", "")
	if mechanism != "none" && (username == "" || password == "") {
		return KafkaConfig{}, fmt.Errorf("Kafka SASL requires WERK_KAFKA_SASL_USERNAME and WERK_KAFKA_SASL_PASSWORD")
	}
	certFile := value(lookup, "WERK_KAFKA_TLS_CERT_FILE", "")
	keyFile := value(lookup, "WERK_KAFKA_TLS_KEY_FILE", "")
	if (certFile == "") != (keyFile == "") {
		return KafkaConfig{}, fmt.Errorf("Kafka client TLS certificate and key must be configured together")
	}
	if enabled && environment == "production" {
		if !tlsEnabled {
			return KafkaConfig{}, fmt.Errorf("WERK_KAFKA_TLS must be true when Kafka is enabled in production")
		}
		if mechanism == "none" && certFile == "" {
			return KafkaConfig{}, fmt.Errorf("production Kafka requires SASL or a client TLS certificate")
		}
	}
	publishTimeoutSeconds, err := strconv.Atoi(value(lookup, "WERK_KAFKA_PUBLISH_TIMEOUT_SECONDS", "10"))
	if err != nil || publishTimeoutSeconds < 1 || publishTimeoutSeconds > 120 {
		return KafkaConfig{}, fmt.Errorf("WERK_KAFKA_PUBLISH_TIMEOUT_SECONDS must be between 1 and 120")
	}
	auditConcurrency, err := strconv.Atoi(value(lookup, "WERK_KAFKA_AUDIT_CONCURRENCY", "1"))
	if err != nil || auditConcurrency < 1 || auditConcurrency > 16 {
		return KafkaConfig{}, fmt.Errorf("WERK_KAFKA_AUDIT_CONCURRENCY must be between 1 and 16")
	}

	return KafkaConfig{
		Enabled:            enabled,
		Brokers:            brokers,
		ClientID:           clientID,
		DomainEventsTopic:  topics[0].value,
		SecurityAuditTopic: topics[1].value,
		RuntimeLogsTopic:   topics[2].value,
		TLS:                tlsEnabled,
		TLSCAFile:          value(lookup, "WERK_KAFKA_TLS_CA_FILE", ""),
		TLSCertFile:        certFile,
		TLSKeyFile:         keyFile,
		TLSServerName:      value(lookup, "WERK_KAFKA_TLS_SERVER_NAME", ""),
		SASLMechanism:      mechanism,
		SASLUsername:       username,
		SASLPassword:       password,
		PublishTimeout:     time.Duration(publishTimeoutSeconds) * time.Second,
		AuditConcurrency:   auditConcurrency,
	}, nil
}

func booleanValue(lookup lookupEnvironment, key string, fallback bool) (bool, error) {
	configured, ok := lookup(key)
	if !ok || strings.TrimSpace(configured) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(configured))
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return parsed, nil
}

func validKafkaName(candidate string, maximum int) bool {
	if candidate == "" || len(candidate) > maximum || candidate == "." || candidate == ".." {
		return false
	}
	for _, character := range candidate {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func parseMFAKeyring(value string) (map[string][]byte, error) {
	keys := make(map[string][]byte)
	for _, entry := range strings.Split(value, ",") {
		parts := strings.Split(strings.TrimSpace(entry), ":")
		if len(parts) != 2 || !stableConfigKey(parts[0]) {
			return nil, fmt.Errorf("invalid MFA keyring entry")
		}
		decoded, err := base64.RawStdEncoding.DecodeString(parts[1])
		if err != nil || len(decoded) != 32 {
			return nil, fmt.Errorf("invalid MFA keyring key")
		}
		if _, exists := keys[parts[0]]; exists {
			return nil, fmt.Errorf("duplicate MFA keyring key")
		}
		keys[parts[0]] = decoded
	}
	return keys, nil
}

func stableConfigKey(value string) bool {
	if value == "" || len(value) > 32 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
			continue
		}
		return false
	}
	return true
}

func value(lookup lookupEnvironment, key, fallback string) string {
	if configured, ok := lookup(key); ok && strings.TrimSpace(configured) != "" {
		return strings.TrimSpace(configured)
	}
	return fallback
}

func validateHTTPAddress(address string) error {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("WERK_HTTP_ADDRESS must be a host:port address: %w", err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("WERK_HTTP_ADDRESS contains an invalid port")
	}
	return nil
}

func validateDatabaseURL(databaseURL string) error {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return fmt.Errorf("DATABASE_URL is invalid: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return fmt.Errorf("DATABASE_URL must use the postgres or postgresql scheme")
	}
	if strings.Trim(parsed.Path, "/") == "" {
		return fmt.Errorf("DATABASE_URL must name a database")
	}
	if parsed.Hostname() == "" && parsed.Query().Get("host") == "" {
		return fmt.Errorf("DATABASE_URL must name a server or socket")
	}
	return nil
}

func validateProductionDatabaseCredentials(databaseURL string) error {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return fmt.Errorf("DATABASE_URL is invalid: %w", err)
	}
	if parsed.User == nil || parsed.User.Username() == "" {
		return fmt.Errorf("DATABASE_URL must contain an explicit production database role")
	}
	password, hasPassword := parsed.User.Password()
	if !hasPassword || password == "" {
		return fmt.Errorf("DATABASE_URL must contain an explicit production database password")
	}
	if parsed.User.Username() == "werk" {
		return fmt.Errorf("the PostgreSQL bootstrap role must not be used by a production service")
	}
	sslMode := strings.ToLower(strings.TrimSpace(parsed.Query().Get("sslmode")))
	if sslMode != "verify-full" {
		return fmt.Errorf("production database URLs must use sslmode=verify-full")
	}
	developmentPasswords := map[string]struct{}{
		"werk":              {},
		"werk-migrator-dev": {},
		"werk-work-dev":     {},
		"werk-identity-dev": {},
		"werk-admin-dev":    {},
		"werk-service-dev":  {},
		"werk-worker-dev":   {},
		"werk-backup-dev":   {},
	}
	if _, isDevelopmentPassword := developmentPasswords[password]; isDevelopmentPassword {
		return fmt.Errorf("development database credentials are forbidden in production")
	}
	return nil
}

func level(environment string) slog.Level {
	if environment == "development" {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}
