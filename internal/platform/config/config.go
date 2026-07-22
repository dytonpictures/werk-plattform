package config

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Environment             string
	HTTPAddress             string
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
}

func Load() (Config, error) {
	return load(os.LookupEnv)
}

func NewLogger(cfg Config) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level(cfg.Environment),
	}))
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

	return Config{
		Environment:             environment,
		HTTPAddress:             httpAddress,
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
	}, nil
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
	if sslMode != "require" && sslMode != "verify-ca" && sslMode != "verify-full" {
		return fmt.Errorf("production database URLs must require TLS")
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
