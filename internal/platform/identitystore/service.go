package identitystore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

const sessionTTL = 12 * time.Hour

type Service struct {
	database          *database.IdentityDB
	now               func() time.Time
	mfaEnabled        bool
	mfaCurrentKeyID   string
	mfaKeys           map[string][]byte
	dummyPasswordHash []byte
}

type Option func(*Service) error

func WithMFA(enabled bool, encryptionKey []byte) Option {
	keys := map[string][]byte{}
	if len(encryptionKey) != 0 {
		keys["default"] = encryptionKey
	}
	return WithMFAKeyring(enabled, "default", keys)
}

func WithMFAKeyring(enabled bool, currentKeyID string, encryptionKeys map[string][]byte) Option {
	return func(service *Service) error {
		if !enabled {
			service.mfaEnabled = false
			service.mfaCurrentKeyID = ""
			service.mfaKeys = nil
			return nil
		}
		if !stableMFAKeyID(currentKeyID) || len(encryptionKeys[currentKeyID]) != 32 {
			return errors.New("MFA keyring requires a valid current 32-byte key")
		}
		keys := make(map[string][]byte, len(encryptionKeys))
		for keyID, key := range encryptionKeys {
			if !stableMFAKeyID(keyID) || len(key) != 32 {
				return errors.New("MFA keyring contains an invalid key")
			}
			keys[keyID] = append([]byte(nil), key...)
		}
		service.mfaEnabled = enabled
		service.mfaCurrentKeyID = currentKeyID
		service.mfaKeys = keys
		return nil
	}
}

func stableMFAKeyID(value string) bool {
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

type SessionView struct {
	AccountClass            identity.AccountClass            `json:"account_class"`
	Audience                identity.Audience                `json:"audience"`
	TenantID                *string                          `json:"tenant_id,omitempty"`
	Profile                 ProfileView                      `json:"profile"`
	HomePath                string                           `json:"home_path"`
	Preferences             PreferencesView                  `json:"preferences"`
	MustChangePassword      bool                             `json:"must_change_password"`
	AuthenticationAssurance identity.AuthenticationAssurance `json:"authentication_assurance"`
	MFAEnrollmentRequired   bool                             `json:"mfa_enrollment_required"`
	ExpiresAt               time.Time                        `json:"expires_at"`
}

type PreferencesView struct {
	NavigationMode string `json:"navigation_mode"`
}

// ProfileView is the authenticated account's provider-independent self view.
// Work display names remain owned by Core Party; admin display names remain
// owned by the separate platform administration subject.
type ProfileView struct {
	DisplayName string `json:"display_name"`
	LoginName   string `json:"login_name"`
}

type accountRecord struct {
	actor              identity.AuthenticatedActor
	credentialID       string
	secretHash         []byte
	rehashedSecret     []byte
	mustChangePassword bool
	sessionGeneration  int64
}

func New(database *database.IdentityDB, options ...Option) (*Service, error) {
	if database == nil {
		return nil, errors.New("identity database is required")
	}
	service := &Service{database: database, now: func() time.Time { return time.Now().UTC() }}
	dummyHash, err := identity.HashPassword("invalid-account-timing-equalizer")
	if err != nil {
		return nil, err
	}
	service.dummyPasswordHash = dummyHash
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(service); err != nil {
			return nil, err
		}
	}
	return service, nil
}

func (service *Service) BootstrapAdmin(ctx context.Context, loginName, displayName, password string) error {
	secretHash, err := identity.HashPassword(password)
	if err != nil || strings.TrimSpace(loginName) == "" || strings.TrimSpace(displayName) == "" {
		return identity.ErrBootstrapSecretMissing
	}
	loginName = strings.ToLower(strings.TrimSpace(loginName))
	adminSubjectID, err := randomUUID()
	if err != nil {
		return err
	}
	accountID, err := randomUUID()
	if err != nil {
		return err
	}
	return service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		var consumed bool
		if err := tx.QueryRow(ctx, `
			SELECT consumed_at IS NOT NULL
			FROM werk_core.identity_bootstrap
			WHERE id = true
			FOR UPDATE
		`).Scan(&consumed); err != nil {
			return err
		}
		if consumed {
			return identity.ErrBootstrapAlreadyUsed
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.admin_subjects (id, display_name, status)
			VALUES ($1::uuid, $2, 'active')
		`, adminSubjectID, strings.TrimSpace(displayName)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.accounts (
				id, account_class, admin_subject_id, login_name, status, must_change_password
			) VALUES ($1::uuid, 'admin', $2::uuid, $3, 'active', true)
		`, accountID, adminSubjectID, loginName); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.account_credentials (
				account_id, credential_kind, secret_hash, assurance
			) VALUES ($1::uuid, 'password', $2, 'single-factor')
		`, accountID, secretHash); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.account_identity_bindings (
				account_id, provider_key, provider_subject
			) VALUES ($1::uuid, 'local', $1::uuid::text)
		`, accountID); err != nil {
			return err
		}
		assignmentID, err := randomUUID()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.role_assignments (
				id, account_id, role_id, access_plane, scope_type, valid_from
			) SELECT $1::uuid, $2::uuid, role.id, 'admin', 'installation', $3
			FROM werk_core.roles AS role
			WHERE role.role_key = 'installation-administrator'
			  AND role.access_plane = 'admin' AND role.status = 'active'
		`, assignmentID, accountID, service.now()); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE werk_core.identity_bootstrap
			SET consumed_at = $2, consumed_account_id = $1::uuid
			WHERE id = true
		`, accountID, service.now())
		return err
	})
}

func (service *Service) Login(ctx context.Context, loginName, password string) (string, string, error) {
	requestID, _ := randomUUID()
	correlationID, _ := randomUUID()
	result, err := service.LoginWithMFA(ctx, loginName, password, requestID, correlationID)
	if err != nil {
		return "", "", err
	}
	if result.MFARequired {
		return "", "", identity.ErrMFARequired
	}
	return result.SessionToken, result.Redirect, nil
}

func (service *Service) LoginWithMFA(ctx context.Context, loginName, password, requestID, correlationID string) (identity.LoginResult, error) {
	request := identity.LoginRequest{LoginName: loginName, Password: password}
	if err := identity.ValidateLoginRequest(request); err != nil {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	normalizedLogin := strings.ToLower(strings.TrimSpace(loginName))
	throttleKey := loginThrottleKey(normalizedLogin)
	throttled, throttleErr := service.loginThrottled(ctx, throttleKey)
	if throttleErr != nil {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	if throttled {
		// Perform the same expensive verifier even while throttled so the
		// response does not become an account-existence oracle.
		_ = identity.VerifyPassword(service.dummyPasswordHash, password)
		_ = service.auditSecurityEvent(ctx, "identity.login.throttled.v1", "denied", "", "", requestID, correlationID, `{"reason":"rate-limited"}`)
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	var record accountRecord
	err := service.database.WithinRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		loaded, err := loadAccountByLogin(ctx, tx, normalizedLogin, service.now())
		record = loaded
		return err
	})
	hashToVerify := record.secretHash
	if err != nil {
		hashToVerify = service.dummyPasswordHash
	}
	passwordValid := identity.VerifyPassword(hashToVerify, password)
	if err != nil || !passwordValid {
		failureKey := throttleKey
		if err != nil {
			failureKey = unknownLoginThrottleKey()
		}
		if service.recordLoginFailure(ctx, failureKey) != nil {
			return identity.LoginResult{}, identity.ErrInvalidCredentials
		}
		accountID := ""
		tenantID := ""
		if err == nil {
			accountID = formatUUID(record.actor.AccountID)
			if record.actor.TenantID != nil {
				tenantID = formatUUID(*record.actor.TenantID)
			}
		}
		_ = service.auditSecurityEvent(ctx, "identity.login.denied.v1", "denied", accountID, tenantID, requestID, correlationID, `{"reason":"invalid-credentials"}`)
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	if service.clearLoginFailures(ctx, throttleKey) != nil {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	if identity.PasswordNeedsRehash(record.secretHash) {
		record.rehashedSecret, err = identity.HashPassword(password)
		if err != nil {
			return identity.LoginResult{}, identity.ErrInvalidCredentials
		}
	}
	if service.mfaEnabled && record.actor.AccountClass == identity.AccountClassAdmin {
		return service.beginAdminMFA(ctx, record, requestID, correlationID)
	}
	return service.issueLoginSession(ctx, record, identity.AssuranceSingleFactor, requestID, correlationID)
}

func (service *Service) issueLoginSession(ctx context.Context, record accountRecord, assurance identity.AuthenticationAssurance, requestID, correlationID string) (identity.LoginResult, error) {
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	sessionID, err := randomUUID()
	if err != nil {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	expiresAt := service.now().Add(sessionTTL)
	err = service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		var currentGeneration int64
		if err := tx.QueryRow(ctx, `
			SELECT session_generation
			FROM werk_core.accounts
			WHERE id = $1::uuid AND status = 'active'
			FOR UPDATE
		`, formatUUID(record.actor.AccountID)).Scan(&currentGeneration); err != nil || currentGeneration != record.sessionGeneration {
			return identity.ErrInvalidCredentials
		}
		if len(record.rehashedSecret) != 0 {
			command, err := tx.Exec(ctx, `
				UPDATE werk_core.account_credentials
				SET secret_hash = $2, changed_at = $3
				WHERE id = $1::uuid AND credential_kind = 'password' AND status = 'active'
			`, record.credentialID, record.rehashedSecret, service.now())
			if err != nil || command.RowsAffected() != 1 {
				return identity.ErrInvalidCredentials
			}
		}
		command, err := tx.Exec(ctx, `
			INSERT INTO werk_core.sessions (
				id, account_id, token_hash, audience, tenant_id, expires_at,
				authentication_assurance, authentication_kind, session_generation
			)
			SELECT $1::uuid, id, $2, $3, tenant_id, $4, $6, $7, session_generation
			FROM werk_core.accounts
			WHERE id = $5::uuid AND status = 'active' AND session_generation = $8
		`, sessionID, tokenHash[:], record.actor.Audience, expiresAt, formatUUID(record.actor.AccountID), assurance, record.actor.Kind, record.sessionGeneration)
		if err != nil || command.RowsAffected() != 1 {
			return identity.ErrInvalidCredentials
		}
		tenantID := ""
		if record.actor.TenantID != nil {
			tenantID = formatUUID(*record.actor.TenantID)
		}
		return service.insertSecurityAuditForTenant(
			ctx, tx, "identity.login.succeeded.v1", "succeeded",
			formatUUID(record.actor.AccountID), sessionID, tenantID, requestID, correlationID,
			`{"authentication_assurance":"`+string(assurance)+`","audience":"`+string(record.actor.Audience)+`"}`,
		)
	})
	if err != nil {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	redirect := "/app"
	if record.mustChangePassword {
		redirect = "/change-password"
	} else if record.actor.AccountClass == identity.AccountClassAdmin {
		redirect = "/admin"
	}
	return identity.LoginResult{SessionToken: token, Redirect: redirect}, nil
}

func (service *Service) Session(ctx context.Context, token string) (any, error) {
	if token == "" {
		return nil, identity.ErrSessionInvalid
	}
	tokenHash := sha256.Sum256([]byte(token))
	var view SessionView
	var accountClass string
	var audience string
	var authenticationKind string
	var tenantID pgtype.UUID
	var sessionID [16]byte
	var accountID [16]byte
	err := service.database.WithinRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		return tx.QueryRow(ctx, `
			SELECT session.id, account.id, account.account_class, session.audience,
			       session.authentication_kind, session.tenant_id,
			       account.login_name,
			       COALESCE(admin_subject.display_name, party.display_name),
			       account.must_change_password, session.authentication_assurance, session.expires_at,
			       COALESCE(preference.navigation_mode, 'bar')
			FROM werk_core.sessions AS session
			JOIN werk_core.accounts AS account ON account.id = session.account_id
			LEFT JOIN werk_core.admin_subjects AS admin_subject ON admin_subject.id = account.admin_subject_id
			LEFT JOIN werk_core.parties AS party
			  ON party.tenant_id = account.tenant_id AND party.id = account.person_party_id
			LEFT JOIN werk_core.account_ui_preferences AS preference ON preference.account_id = account.id
			WHERE session.token_hash = $1 AND session.revoked_at IS NULL
			  AND session.expires_at > $2 AND account.status = 'active'
			  AND session.session_generation = account.session_generation
		`, tokenHash[:], service.now()).Scan(
			&sessionID, &accountID, &accountClass, &audience, &authenticationKind, &tenantID, &view.Profile.LoginName,
			&view.Profile.DisplayName, &view.MustChangePassword, &view.AuthenticationAssurance, &view.ExpiresAt,
			&view.Preferences.NavigationMode,
		)
	})
	if err != nil {
		return nil, identity.ErrSessionInvalid
	}
	view.AccountClass = identity.AccountClass(accountClass)
	view.Audience = identity.Audience(audience)
	actor := identity.AuthenticatedActor{
		AccountID: identity.AccountID(accountID), AccountClass: view.AccountClass,
		Audience: view.Audience, Kind: identity.AuthenticationKind(authenticationKind),
		Assurance: view.AuthenticationAssurance,
	}
	view.HomePath = "/app"
	if view.AccountClass == identity.AccountClassAdmin {
		view.HomePath = "/admin"
		view.MFAEnrollmentRequired = service.mfaEnabled && view.AuthenticationAssurance != identity.AssuranceMultiFactor
	}
	if tenantID.Valid {
		value := formatUUID(tenantID.Bytes)
		view.TenantID = &value
		tenant := tenancy.TenantID(tenantID.Bytes)
		actor.TenantID = &tenant
	}
	if _, err := identity.ValidateSessionRecord(identity.SessionRecord{
		ID: identity.SessionID(sessionID), Account: actor, Audience: view.Audience,
		TenantID: actor.TenantID, ExpiresAt: view.ExpiresAt,
	}, service.now()); err != nil {
		return nil, identity.ErrSessionInvalid
	}
	return view, nil
}

func (service *Service) UpdateNavigationPreference(ctx context.Context, token, mode, requestID, correlationID string) error {
	if token == "" || (mode != "bar" && mode != "collapsed") {
		return identity.ErrSessionInvalid
	}
	tokenHash := sha256.Sum256([]byte(token))
	auditID, err := randomUUID()
	if err != nil {
		return err
	}
	return service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		var accountID [16]byte
		var sessionID [16]byte
		var tenantID pgtype.UUID
		if err := tx.QueryRow(ctx, `
			SELECT account.id, session.id, session.tenant_id
			FROM werk_core.sessions AS session
			JOIN werk_core.accounts AS account ON account.id = session.account_id
			WHERE session.token_hash = $1 AND session.revoked_at IS NULL
			  AND session.expires_at > $2 AND account.status = 'active'
			  AND session.session_generation = account.session_generation
		`, tokenHash[:], service.now()).Scan(&accountID, &sessionID, &tenantID); err != nil {
			return identity.ErrSessionInvalid
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.account_ui_preferences (account_id, navigation_mode, updated_at)
			VALUES ($1::uuid, $2, $3)
			ON CONFLICT (account_id) DO UPDATE
			SET navigation_mode = EXCLUDED.navigation_mode, updated_at = EXCLUDED.updated_at
		`, formatUUID(accountID), mode, service.now()); err != nil {
			return err
		}
		var tenant any
		if tenantID.Valid {
			tenant = formatUUID(tenantID.Bytes)
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id, occurred_at, event_type, outcome, account_id, session_id,
				tenant_id, request_id, correlation_id, details
			) VALUES (
				$1::uuid, $2, 'identity.profile-preference.updated.v1', 'succeeded',
				$3::uuid, $4::uuid, $5::uuid, $6::uuid, $7::uuid,
				jsonb_build_object('navigation_mode', $8::text)
			)
		`, auditID, service.now(), formatUUID(accountID), formatUUID(sessionID), tenant, requestID, correlationID, mode)
		return err
	})
}

func (service *Service) Logout(ctx context.Context, token string) error {
	requestID, _ := randomUUID()
	correlationID, _ := randomUUID()
	return service.LogoutWithAudit(ctx, token, requestID, correlationID)
}

func (service *Service) LogoutWithAudit(ctx context.Context, token, requestID, correlationID string) error {
	if token == "" {
		return identity.ErrSessionInvalid
	}
	tokenHash := sha256.Sum256([]byte(token))
	return service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		var accountID [16]byte
		var sessionID [16]byte
		var tenantID pgtype.UUID
		if err := tx.QueryRow(ctx, `
			SELECT account.id
			FROM werk_core.sessions AS session
			JOIN werk_core.accounts AS account ON account.id = session.account_id
			WHERE session.token_hash = $1 AND session.revoked_at IS NULL
			  AND session.expires_at > $2
			  AND session.session_generation = account.session_generation
		`, tokenHash[:], service.now()).Scan(&accountID); err != nil {
			return identity.ErrSessionInvalid
		}
		var sessionGeneration int64
		if err := tx.QueryRow(ctx, `
			SELECT session_generation
			FROM werk_core.accounts
			WHERE id = $1::uuid AND status = 'active'
			FOR UPDATE
		`, formatUUID(accountID)).Scan(&sessionGeneration); err != nil {
			return identity.ErrSessionInvalid
		}
		if err := tx.QueryRow(ctx, `
			SELECT session.id, session.tenant_id
			FROM werk_core.sessions AS session
			JOIN werk_core.accounts AS account ON account.id = session.account_id
			WHERE session.token_hash = $1 AND session.revoked_at IS NULL
			  AND session.expires_at > $2
			  AND session.session_generation = account.session_generation
			  AND account.id = $3::uuid AND session.session_generation = $4
			FOR UPDATE OF session
		`, tokenHash[:], service.now(), formatUUID(accountID), sessionGeneration).Scan(&sessionID, &tenantID); err != nil {
			return identity.ErrSessionInvalid
		}
		command, err := tx.Exec(ctx, `
			UPDATE werk_core.sessions SET revoked_at = $2
			WHERE id = $1::uuid AND revoked_at IS NULL
		`, formatUUID(sessionID), service.now())
		if err != nil || command.RowsAffected() != 1 {
			return identity.ErrSessionInvalid
		}
		tenant := ""
		if tenantID.Valid {
			tenant = formatUUID(tenantID.Bytes)
		}
		return service.insertSecurityAuditForTenant(ctx, tx, "identity.logout.succeeded.v1", "succeeded", formatUUID(accountID), formatUUID(sessionID), tenant, requestID, correlationID, `{}`)
	})
}

func (service *Service) ChangePassword(ctx context.Context, token, currentPassword, newPassword string) (identity.SessionRotation, error) {
	requestID, _ := randomUUID()
	correlationID, _ := randomUUID()
	return service.ChangePasswordWithAudit(ctx, token, currentPassword, newPassword, requestID, correlationID)
}

func (service *Service) ChangePasswordWithAudit(ctx context.Context, token, currentPassword, newPassword, requestID, correlationID string) (identity.SessionRotation, error) {
	passwordSnapshot, err := service.loadSessionPasswordSnapshot(ctx, token, false)
	if err != nil {
		return identity.SessionRotation{}, err
	}
	if !identity.VerifyPassword(passwordSnapshot.passwordHash, currentPassword) {
		_ = service.auditSecurityEvent(ctx, "identity.password.change-denied.v1", "denied", passwordSnapshot.accountID, passwordSnapshot.tenantID, requestID, correlationID, `{"reason":"invalid-current-password"}`)
		return identity.SessionRotation{}, identity.ErrInvalidCredentials
	}
	newHash, err := identity.HashPassword(newPassword)
	if err != nil {
		return identity.SessionRotation{}, identity.ErrInvalidCredentials
	}
	rotation, err := service.prepareSessionRotation()
	if err != nil {
		return identity.SessionRotation{}, err
	}
	tokenHash := sha256.Sum256([]byte(token))
	err = service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		var accountID [16]byte
		var sessionID [16]byte
		var tenantID pgtype.UUID
		var currentHash []byte
		var audience identity.Audience
		var assurance identity.AuthenticationAssurance
		var authenticationKind identity.AuthenticationKind
		var sourceExpiresAt time.Time
		var sessionGeneration int64
		if err := tx.QueryRow(ctx, `
			SELECT session_generation
			FROM werk_core.accounts
			WHERE id = $1::uuid AND status = 'active'
			FOR UPDATE
		`, passwordSnapshot.accountID).Scan(&sessionGeneration); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return identity.ErrInvalidCredentials
			}
			return err
		}
		if err := tx.QueryRow(ctx, `
			SELECT account.id, session.id, session.tenant_id, credential.secret_hash,
			       session.audience, session.authentication_assurance, session.authentication_kind,
			       session.expires_at
			FROM werk_core.sessions AS session
			JOIN werk_core.accounts AS account ON account.id = session.account_id
			JOIN werk_core.account_credentials AS credential
			  ON credential.account_id = account.id
			 AND credential.credential_kind = 'password'
			 AND credential.status = 'active'
			 AND (credential.expires_at IS NULL OR credential.expires_at > $2)
			WHERE session.token_hash = $1 AND session.revoked_at IS NULL
			  AND session.expires_at > $2 AND account.status = 'active'
			  AND session.session_generation = account.session_generation
			  AND account.id = $3::uuid AND session.session_generation = $4
			FOR UPDATE OF session, credential
		`, tokenHash[:], service.now(), passwordSnapshot.accountID, sessionGeneration).Scan(
			&accountID, &sessionID, &tenantID, &currentHash,
			&audience, &assurance, &authenticationKind, &sourceExpiresAt,
		); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return identity.ErrInvalidCredentials
			}
			return err
		}
		if formatUUID(accountID) != passwordSnapshot.accountID || !samePasswordHash(currentHash, passwordSnapshot.passwordHash) {
			return identity.ErrInvalidCredentials
		}
		if err := rotation.limitExpiresAt(sourceExpiresAt); err != nil {
			return identity.ErrSessionInvalid
		}
		command, err := tx.Exec(ctx, `
			UPDATE werk_core.account_credentials SET secret_hash = $2, changed_at = $3
			WHERE account_id = $1::uuid AND credential_kind = 'password' AND status = 'active'
		`, formatUUID(accountID), newHash, rotation.createdAt)
		if err != nil {
			return err
		}
		if command.RowsAffected() != 1 {
			return identity.ErrInvalidCredentials
		}
		command, err = tx.Exec(ctx, `
			UPDATE werk_core.accounts
			SET must_change_password = false, updated_at = $2, version = version + 1
			WHERE id = $1::uuid
		`, formatUUID(accountID), rotation.createdAt)
		if err != nil {
			return err
		}
		if command.RowsAffected() != 1 {
			return identity.ErrInvalidCredentials
		}
		if _, err := tx.Exec(ctx, `
			UPDATE werk_core.identity_mfa_challenges
			SET used_at = $2
			WHERE account_id = $1::uuid AND used_at IS NULL
		`, formatUUID(accountID), rotation.createdAt); err != nil {
			return err
		}
		tenant := ""
		if tenantID.Valid {
			tenant = formatUUID(tenantID.Bytes)
		}
		if err := service.rotateAccountSessions(ctx, tx, sessionRotationSubject{
			accountID: formatUUID(accountID), previousSessionID: formatUUID(sessionID), tenantID: tenant,
			audience: audience, assurance: assurance, kind: authenticationKind,
		}, rotation, sessionRotationPasswordChange, requestID, correlationID); err != nil {
			return err
		}
		return service.insertSecurityAuditForTenant(ctx, tx, "identity.password.changed.v1", "succeeded", formatUUID(accountID), formatUUID(sessionID), tenant, requestID, correlationID, `{}`)
	})
	if err != nil {
		return identity.SessionRotation{}, err
	}
	return rotation.result, nil
}

func loadAccountByLogin(ctx context.Context, tx database.TenantTx, loginName string, now time.Time) (accountRecord, error) {
	var record accountRecord
	var accountID [16]byte
	var accountClass string
	var status string
	var assurance string
	var tenantID pgtype.UUID
	err := tx.QueryRow(ctx, `
		SELECT account.id, account.account_class, account.status, account.tenant_id,
		       account.must_change_password, credential.id::text,
		       credential.assurance, credential.secret_hash, account.session_generation
		FROM werk_core.accounts AS account
		JOIN werk_core.account_credentials AS credential ON credential.account_id = account.id
		WHERE account.login_name = $1 AND credential.credential_kind = 'password'
		  AND credential.status = 'active'
		  AND (credential.expires_at IS NULL OR credential.expires_at > $2)
	`, loginName, now).Scan(&accountID, &accountClass, &status, &tenantID, &record.mustChangePassword, &record.credentialID, &assurance, &record.secretHash, &record.sessionGeneration)
	if err != nil || status != "active" {
		return accountRecord{}, identity.ErrInvalidCredentials
	}
	record.actor = identity.AuthenticatedActor{
		AccountID: identity.AccountID(accountID), AccountClass: identity.AccountClass(accountClass),
		Kind: identity.AuthenticationInteractive, Assurance: identity.AuthenticationAssurance(assurance),
	}
	switch record.actor.AccountClass {
	case identity.AccountClassWork:
		record.actor.Audience = identity.AudienceWork
		if !tenantID.Valid {
			return accountRecord{}, identity.ErrInvalidCredentials
		}
		value := tenancy.TenantID(tenantID.Bytes)
		record.actor.TenantID = &value
	case identity.AccountClassAdmin:
		record.actor.Audience = identity.AudienceAdmin
		if tenantID.Valid {
			return accountRecord{}, identity.ErrInvalidCredentials
		}
	default:
		return accountRecord{}, identity.ErrInvalidCredentials
	}
	return record, nil
}

func newSessionToken() (string, [32]byte, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", [32]byte{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(value)
	return token, sha256.Sum256([]byte(token)), nil
}

func randomUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return formatUUID(value), nil
}

func formatUUID(value [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
