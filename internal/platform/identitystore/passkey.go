package identitystore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

const (
	maximumPasskeysPerAccount             = 10
	maximumOpenAuthenticationCeremonies   = 4096
	authenticationCeremonyCleanupBatch    = 128
	maximumConcurrentPasskeyVerifications = 16
	passkeyVerificationQueueTimeout       = 2 * time.Second
)

var passkeyVerificationLimiter = newBoundedWorkLimiter(
	maximumConcurrentPasskeyVerifications,
	passkeyVerificationQueueTimeout,
)

type passkeyUser struct {
	accountID          [16]byte
	loginName          string
	displayName        string
	actor              identity.AuthenticatedActor
	mustChangePassword bool
	sessionGeneration  int64
	credentials        []webauthn.Credential
	factorIDs          map[[32]byte]string
}

func (user *passkeyUser) WebAuthnID() []byte                         { return user.accountID[:] }
func (user *passkeyUser) WebAuthnName() string                       { return user.loginName }
func (user *passkeyUser) WebAuthnDisplayName() string                { return user.displayName }
func (user *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return user.credentials }

func (service *Service) StartPasskeyRegistration(ctx context.Context, sessionToken, currentPassword, displayName, requestID, correlationID string) (identity.PasskeyCeremony, error) {
	if !service.passkeysAvailable() || strings.TrimSpace(displayName) == "" {
		return identity.PasskeyCeremony{}, identity.ErrPasskeyInvalid
	}
	snapshot, err := service.loadSessionPasswordSnapshot(ctx, sessionToken, false)
	if err != nil || !identity.VerifyPassword(snapshot.passwordHash, currentPassword) {
		return identity.PasskeyCeremony{}, identity.ErrInvalidCredentials
	}
	ceremonyToken, ceremonyTokenHash, err := newSessionToken()
	if err != nil {
		return identity.PasskeyCeremony{}, err
	}
	challengeID, err := randomUUID()
	if err != nil {
		return identity.PasskeyCeremony{}, err
	}

	var creation any
	err = service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		accountID, _, credentialID, providerKey, passwordHash, err := lockPasskeyEnrollmentSession(ctx, tx, sessionToken, service.now())
		if err != nil || accountID != snapshot.accountID || credentialID != snapshot.credentialID ||
			providerKey != snapshot.providerKey || !samePasswordHash(passwordHash, snapshot.passwordHash) {
			return identity.ErrInvalidCredentials
		}
		if err := lockActiveProviderBinding(ctx, tx, accountID, providerKey, identity.AuthenticationMethodPassword); err != nil {
			return identity.ErrInvalidCredentials
		}
		user, err := service.loadPasskeyUser(ctx, tx, accountID, true)
		if err != nil {
			return err
		}
		if len(user.credentials) >= maximumPasskeysPerAccount {
			return identity.ErrPasskeyInvalid
		}
		options, session, err := service.webAuthn.BeginRegistration(
			user,
			webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
				RequireResidentKey: protocol.ResidentKeyRequired(),
				ResidentKey:        protocol.ResidentKeyRequirementRequired,
				UserVerification:   protocol.VerificationRequired,
			}),
			webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
		)
		if err != nil {
			return identity.ErrPasskeyInvalid
		}
		challengeHash, err := identity.HashPasskeyChallenge(session.Challenge)
		if err != nil {
			return err
		}
		sessionJSON, err := json.Marshal(session)
		if err != nil {
			return err
		}
		reference, err := service.encryptMFASecret(accountID, challengeID, string(sessionJSON))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.identity_mfa_challenges (
				id, account_id, credential_id, purpose, challenge_hash, ceremony_token_hash,
				ceremony_reference, relying_party_id, expires_at, session_generation
			) VALUES ($1::uuid, $2::uuid, $3::uuid, 'enrollment', $4, $5, $6, $7, $8, $9)
		`, challengeID, accountID, credentialID, challengeHash[:], ceremonyTokenHash[:], reference,
			service.webAuthn.Config.RPID, session.Expires, user.sessionGeneration); err != nil {
			return err
		}
		if err := service.insertSecurityAuditForTenant(ctx, tx, "identity.passkey.enrollment-started.v1", "succeeded", accountID, "", snapshot.tenantID, requestID, correlationID, `{}`); err != nil {
			return err
		}
		creation = options.Response
		return nil
	})
	if err != nil {
		return identity.PasskeyCeremony{}, err
	}
	return identity.PasskeyCeremony{PublicKey: creation, Token: ceremonyToken}, nil
}

func (service *Service) FinishPasskeyRegistration(ctx context.Context, sessionToken, ceremonyToken, displayName string, credential identity.PasskeyRegistrationCredential, requestID, correlationID string) (identity.PasskeyActivation, error) {
	if !service.passkeysAvailable() || sessionToken == "" || ceremonyToken == "" || strings.TrimSpace(displayName) == "" {
		return identity.PasskeyActivation{}, identity.ErrPasskeyInvalid
	}
	if _, err := credential.DecodeForVerification(); err != nil {
		return identity.PasskeyActivation{}, identity.ErrPasskeyInvalid
	}
	payload, err := json.Marshal(credential)
	if err != nil {
		return identity.PasskeyActivation{}, identity.ErrPasskeyInvalid
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(payload)
	if err != nil {
		return identity.PasskeyActivation{}, identity.ErrPasskeyInvalid
	}
	rotation, err := service.prepareSessionRotation()
	if err != nil {
		return identity.PasskeyActivation{}, err
	}
	ceremonyHash := sha256.Sum256([]byte(ceremonyToken))
	factorID, err := randomUUID()
	if err != nil {
		return identity.PasskeyActivation{}, err
	}

	err = service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		accountID, sessionID, credentialID, providerKey, _, err := lockPasskeyEnrollmentSession(ctx, tx, sessionToken, service.now())
		if err != nil {
			return err
		}
		challengeID, challengeAccountID, challengeCredentialID, reference, expiresAt, challengeGeneration, err := loadPasskeyCeremony(ctx, tx, ceremonyHash[:], "enrollment", service.webAuthn.Config.RPID)
		if err != nil || challengeAccountID != accountID || challengeCredentialID != credentialID || !service.now().Before(expiresAt) {
			return identity.ErrPasskeyInvalid
		}
		if err := lockActiveProviderBinding(ctx, tx, accountID, providerKey, identity.AuthenticationMethodPassword); err != nil {
			return identity.ErrInvalidCredentials
		}
		user, err := service.loadPasskeyUser(ctx, tx, accountID, true)
		if err != nil || user.sessionGeneration != challengeGeneration || len(user.credentials) >= maximumPasskeysPerAccount {
			return identity.ErrPasskeyInvalid
		}
		session, err := service.decryptPasskeySession(accountID, challengeID, reference)
		if err != nil || !bytes.Equal(session.UserID, user.WebAuthnID()) {
			return identity.ErrPasskeyInvalid
		}
		created, err := service.webAuthn.CreateCredential(user, session, parsed)
		if err != nil {
			return identity.ErrPasskeyInvalid
		}
		credentialJSON, err := json.Marshal(created)
		if err != nil {
			return err
		}
		credentialReference, err := service.encryptMFASecret(accountID, factorID, string(credentialJSON))
		if err != nil {
			return err
		}
		credentialIDHash := sha256.Sum256(created.ID)
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.identity_mfa_factors (
				id, account_id, factor_kind, status, display_name,
				credential_id_hash, credential_id, public_key, credential_reference,
				relying_party_id, sign_count, activated_at
			) VALUES ($1::uuid, $2::uuid, 'webauthn', 'active', $3, $4, $5, $6, $7, $8, $9, $10)
		`, factorID, accountID, strings.TrimSpace(displayName), credentialIDHash[:], created.ID,
			created.PublicKey, credentialReference, service.webAuthn.Config.RPID,
			created.Authenticator.SignCount, service.now()); err != nil {
			return identity.ErrPasskeyInvalid
		}
		if _, err := tx.Exec(ctx, `UPDATE werk_core.identity_mfa_challenges SET used_at = $2 WHERE id = $1::uuid AND used_at IS NULL`, challengeID, service.now()); err != nil {
			return err
		}
		if err := service.rotateAccountSessions(ctx, tx, sessionRotationSubject{
			accountID: accountID, previousSessionID: sessionID,
			audience: user.actor.Audience, assurance: identity.AssuranceMultiFactor,
			kind: identity.AuthenticationInteractive,
		}, rotation, sessionRotationMFAEnrollment, requestID, correlationID); err != nil {
			return err
		}
		return service.insertSecurityAuditForTenant(ctx, tx, "identity.passkey.enrollment-completed.v1", "succeeded", accountID, sessionID, tenantText(user.actor.TenantID), requestID, correlationID, `{}`)
	})
	if err != nil {
		return identity.PasskeyActivation{}, err
	}
	return identity.PasskeyActivation{DisplayName: strings.TrimSpace(displayName), Rotation: rotation.result}, nil
}

func (service *Service) StartPasskeyLogin(ctx context.Context, requestID, correlationID string) (identity.PasskeyCeremony, error) {
	if !service.passkeysAvailable() {
		return identity.PasskeyCeremony{}, identity.ErrInvalidCredentials
	}
	ceremonyToken, ceremonyTokenHash, err := newSessionToken()
	if err != nil {
		return identity.PasskeyCeremony{}, err
	}
	challengeID, err := randomUUID()
	if err != nil {
		return identity.PasskeyCeremony{}, err
	}
	options, session, err := service.webAuthn.BeginDiscoverableLogin(
		webauthn.WithUserVerification(protocol.VerificationRequired),
	)
	if err != nil || session == nil || options == nil {
		return identity.PasskeyCeremony{}, identity.ErrInvalidCredentials
	}
	challengeHash, err := identity.HashPasskeyChallenge(session.Challenge)
	if err != nil {
		return identity.PasskeyCeremony{}, err
	}
	sessionJSON, err := json.Marshal(session)
	if err != nil {
		return identity.PasskeyCeremony{}, err
	}
	reference, err := service.encryptMFASecret("passkey-login", challengeID, string(sessionJSON))
	if err != nil {
		return identity.PasskeyCeremony{}, err
	}
	now := service.now()
	expiresAt := session.Expires
	if expiresAt.IsZero() || expiresAt.After(now.Add(mfaChallengeTTL)) {
		expiresAt = now.Add(mfaChallengeTTL)
	}
	err = service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		// Serialize cleanup and the installation-wide open-ceremony cap across
		// every API process sharing the authoritative Identity database.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(6288511189404317025)`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			WITH removable AS (
				SELECT id FROM werk_core.identity_authentication_ceremonies
				WHERE expires_at <= $1 OR used_at IS NOT NULL
				ORDER BY expires_at
				LIMIT $2
				FOR UPDATE SKIP LOCKED
			)
			DELETE FROM werk_core.identity_authentication_ceremonies AS ceremony
			USING removable WHERE ceremony.id = removable.id
		`, now, authenticationCeremonyCleanupBatch); err != nil {
			return err
		}
		var open int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM werk_core.identity_authentication_ceremonies
			WHERE used_at IS NULL AND expires_at > $1
		`, now).Scan(&open); err != nil || open >= maximumOpenAuthenticationCeremonies {
			return identity.ErrInvalidCredentials
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.identity_authentication_ceremonies (
				id, ceremony_kind, token_hash, challenge_hash, session_reference,
				relying_party_id, created_at, expires_at
			) VALUES ($1::uuid, 'passkey-login', $2, $3, $4, $5, $6, $7)
		`, challengeID, ceremonyTokenHash[:], challengeHash[:], reference,
			service.webAuthn.Config.RPID, now, expiresAt); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return identity.PasskeyCeremony{}, identity.ErrInvalidCredentials
	}
	return identity.PasskeyCeremony{PublicKey: options.Response, Token: ceremonyToken}, nil
}

func (service *Service) FinishPasskeyLogin(ctx context.Context, ceremonyToken string, credential identity.PasskeyAuthenticationCredential, requestID, correlationID string) (identity.LoginResult, error) {
	if !service.passkeysAvailable() || ceremonyToken == "" {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	release, allowed := passkeyVerificationLimiter.acquire(ctx)
	if !allowed {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	defer release()
	decoded, err := credential.DecodeForVerification()
	if err != nil {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	payload, err := json.Marshal(credential)
	if err != nil {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(payload)
	if err != nil {
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	ceremonyHash := sha256.Sum256([]byte(ceremonyToken))
	sessionToken, sessionHash, err := newSessionToken()
	if err != nil {
		return identity.LoginResult{}, err
	}
	sessionID, err := randomUUID()
	if err != nil {
		return identity.LoginResult{}, err
	}
	redirect := "/app"
	auditAccountID := ""
	auditTenantID := ""
	verificationFailed := false
	err = service.database.WithinWrite(ctx, func(ctx context.Context, tx database.TenantTx) error {
		challengeID, reference, expiresAt, err := loadAnonymousPasskeyCeremony(ctx, tx, ceremonyHash[:], service.webAuthn.Config.RPID)
		if err != nil || !service.now().Before(expiresAt) {
			return identity.ErrInvalidCredentials
		}
		session, err := service.decryptPasskeySession("passkey-login", challengeID, reference)
		if err != nil {
			return identity.ErrInvalidCredentials
		}
		var resolvedUser *passkeyUser
		resolved, verified, verifyErr := service.webAuthn.ValidatePasskeyLogin(
			func(rawID, userHandle []byte) (webauthn.User, error) {
				user, lookupErr := service.loadDiscoverablePasskeyUser(ctx, tx, rawID, userHandle)
				if lookupErr == nil {
					resolvedUser = user
					auditAccountID = formatUUID(user.accountID)
					auditTenantID = tenantText(user.actor.TenantID)
				}
				return user, lookupErr
			}, session, parsed,
		)
		if verifyErr != nil || resolved == nil || verified == nil || resolvedUser == nil ||
			verified.Authenticator.CloneWarning || !bytes.Equal(verified.ID, decoded.CredentialID) {
			verificationFailed = true
			_, updateErr := tx.Exec(ctx, `UPDATE werk_core.identity_authentication_ceremonies SET used_at = $2 WHERE id = $1::uuid AND used_at IS NULL`, challengeID, service.now())
			return updateErr
		}
		user := resolvedUser
		accountID := formatUUID(user.accountID)
		if err := lockActiveProviderBinding(ctx, tx, accountID, localProviderKey, identity.AuthenticationMethodPasskey); err != nil {
			verificationFailed = true
			_, updateErr := tx.Exec(ctx, `UPDATE werk_core.identity_authentication_ceremonies SET used_at = $2 WHERE id = $1::uuid AND used_at IS NULL`, challengeID, service.now())
			return updateErr
		}
		factorID, ok := user.factorIDs[sha256.Sum256(verified.ID)]
		if !ok {
			return identity.ErrInvalidCredentials
		}
		credentialJSON, err := json.Marshal(verified)
		if err != nil {
			return err
		}
		credentialReference, err := service.encryptMFASecret(accountID, factorID, string(credentialJSON))
		if err != nil {
			return err
		}
		command, err := tx.Exec(ctx, `
			UPDATE werk_core.identity_mfa_factors
			SET credential_reference = $3, public_key = $4, sign_count = $5, last_used_at = $6
			WHERE id = $1::uuid AND account_id = $2::uuid AND factor_kind = 'webauthn' AND status = 'active'
		`, factorID, accountID, credentialReference, verified.PublicKey, verified.Authenticator.SignCount, service.now())
		if err != nil || command.RowsAffected() != 1 {
			return identity.ErrInvalidCredentials
		}
		if _, err := tx.Exec(ctx, `UPDATE werk_core.identity_authentication_ceremonies SET used_at = $2 WHERE id = $1::uuid AND used_at IS NULL`, challengeID, service.now()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO werk_core.sessions (
				id, account_id, token_hash, audience, tenant_id, expires_at,
				authentication_assurance, authentication_kind, session_generation
			) VALUES ($1::uuid, $2::uuid, $3, $4, $5::uuid, $6, 'multi-factor', 'interactive', $7)
		`, sessionID, accountID, sessionHash[:], user.actor.Audience, nullableTenant(user.actor.TenantID), service.now().Add(service.sessionLifetime(user.actor.Audience)), user.sessionGeneration); err != nil {
			return err
		}
		if user.mustChangePassword {
			redirect = "/change-password"
		} else if user.actor.AccountClass == identity.AccountClassAdmin {
			redirect = "/admin"
		}
		return service.insertSecurityAuditForTenant(ctx, tx, "identity.passkey.authentication-succeeded.v1", "succeeded", accountID, sessionID, tenantText(user.actor.TenantID), requestID, correlationID, `{"authentication_assurance":"multi-factor"}`)
	})
	if err != nil {
		if auditAccountID != "" {
			_ = service.auditSecurityEvent(ctx, "identity.passkey.authentication-denied.v1", "denied", auditAccountID, auditTenantID, requestID, correlationID, `{"reason":"verification-failed"}`)
		}
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	if verificationFailed {
		if auditAccountID != "" {
			_ = service.auditSecurityEvent(ctx, "identity.passkey.authentication-denied.v1", "denied", auditAccountID, auditTenantID, requestID, correlationID, `{"reason":"verification-failed"}`)
		}
		return identity.LoginResult{}, identity.ErrInvalidCredentials
	}
	return identity.LoginResult{SessionToken: sessionToken, Redirect: redirect}, nil
}

func (service *Service) passkeysAvailable() bool {
	return service.mfaEnabled && service.webAuthn != nil && len(service.mfaKeys) != 0
}

func (service *Service) loadPasskeyUser(ctx context.Context, tx database.TenantTx, accountID string, forUpdate bool) (*passkeyUser, error) {
	query := `
		SELECT account.id, account.login_name, account.account_class, account.tenant_id,
		       account.must_change_password, account.session_generation,
		       COALESCE(admin_subject.display_name, party.display_name)
		FROM werk_core.accounts AS account
		LEFT JOIN werk_core.admin_subjects AS admin_subject ON admin_subject.id = account.admin_subject_id
		LEFT JOIN werk_core.parties AS party ON party.tenant_id = account.tenant_id AND party.id = account.person_party_id
		WHERE account.id = $1::uuid AND account.status = 'active'`
	if forUpdate {
		query += ` FOR UPDATE OF account`
	}
	user := &passkeyUser{factorIDs: make(map[[32]byte]string)}
	var accountClass string
	var tenantID pgtype.UUID
	if err := tx.QueryRow(ctx, query, accountID).Scan(&user.accountID, &user.loginName, &accountClass, &tenantID, &user.mustChangePassword, &user.sessionGeneration, &user.displayName); err != nil {
		return nil, identity.ErrInvalidCredentials
	}
	user.actor = identity.AuthenticatedActor{
		AccountID: identity.AccountID(user.accountID), AccountClass: identity.AccountClass(accountClass),
		Kind: identity.AuthenticationInteractive, Assurance: identity.AssuranceMultiFactor,
	}
	switch user.actor.AccountClass {
	case identity.AccountClassAdmin:
		if tenantID.Valid {
			return nil, identity.ErrInvalidCredentials
		}
		user.actor.Audience = identity.AudienceAdmin
	case identity.AccountClassWork:
		if !tenantID.Valid {
			return nil, identity.ErrInvalidCredentials
		}
		tenant := tenancy.TenantID(tenantID.Bytes)
		user.actor.TenantID = &tenant
		user.actor.Audience = identity.AudienceWork
	default:
		return nil, identity.ErrInvalidCredentials
	}
	credentialQuery := `
		SELECT id::text, credential_id, credential_id_hash, public_key, credential_reference
		FROM werk_core.identity_mfa_factors
		WHERE account_id = $1::uuid AND factor_kind = 'webauthn' AND status = 'active'
		  AND relying_party_id = $2`
	if forUpdate {
		credentialQuery += ` FOR UPDATE`
	}
	rows, err := tx.Query(ctx, credentialQuery, accountID, service.webAuthn.Config.RPID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var factorID, reference string
		var rawID, storedHash, publicKey []byte
		if err := rows.Scan(&factorID, &rawID, &storedHash, &publicKey, &reference); err != nil {
			return nil, err
		}
		plaintext, err := service.decryptMFASecret(accountID, factorID, reference)
		if err != nil {
			return nil, identity.ErrInvalidCredentials
		}
		var credential webauthn.Credential
		if err := json.Unmarshal([]byte(plaintext), &credential); err != nil || !bytes.Equal(rawID, credential.ID) || !bytes.Equal(publicKey, credential.PublicKey) {
			return nil, identity.ErrInvalidCredentials
		}
		digest := sha256.Sum256(credential.ID)
		if !bytes.Equal(storedHash, digest[:]) {
			return nil, identity.ErrInvalidCredentials
		}
		user.credentials = append(user.credentials, credential)
		user.factorIDs[digest] = factorID
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return user, nil
}

func lockPasskeyEnrollmentSession(ctx context.Context, tx database.TenantTx, token string, now time.Time) (
	accountID string,
	sessionID string,
	credentialID string,
	providerKey string,
	passwordHash []byte,
	err error,
) {
	if token == "" {
		return "", "", "", "", nil, identity.ErrSessionInvalid
	}
	tokenHash := sha256.Sum256([]byte(token))
	err = tx.QueryRow(ctx, `
		SELECT account.id::text, session.id::text, credential.id::text,
		       credential.provider_key, credential.secret_hash
		FROM werk_core.sessions AS session
		JOIN werk_core.accounts AS account ON account.id = session.account_id
		JOIN werk_core.account_credentials AS credential
		  ON credential.account_id = account.id AND credential.credential_kind = 'password'
		 AND credential.status = 'active' AND (credential.expires_at IS NULL OR credential.expires_at > $2)
		WHERE session.token_hash = $1 AND session.revoked_at IS NULL AND session.expires_at > $2
		  AND account.status = 'active' AND session.session_generation = account.session_generation
		  AND ((account.account_class = 'admin' AND session.audience = 'admin')
		    OR (account.account_class = 'work' AND session.audience = 'work' AND session.tenant_id = account.tenant_id))
		FOR UPDATE OF account, session, credential
	`, tokenHash[:], now).Scan(&accountID, &sessionID, &credentialID, &providerKey, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", "", nil, identity.ErrSessionInvalid
	}
	return accountID, sessionID, credentialID, providerKey, passwordHash, err
}

func loadPasskeyCeremony(ctx context.Context, tx database.TenantTx, tokenHash []byte, purpose, relyingPartyID string) (
	challengeID string,
	accountID string,
	credentialID string,
	reference string,
	expiresAt time.Time,
	generation int64,
	err error,
) {
	err = tx.QueryRow(ctx, `
		SELECT id::text, account_id::text, credential_id::text,
		       ceremony_reference, expires_at, session_generation
		FROM werk_core.identity_mfa_challenges
		WHERE ceremony_token_hash = $1 AND purpose = $2 AND relying_party_id = $3 AND used_at IS NULL
		FOR UPDATE
	`, tokenHash, purpose, relyingPartyID).Scan(
		&challengeID, &accountID, &credentialID, &reference, &expiresAt, &generation,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		err = identity.ErrPasskeyInvalid
	}
	return
}

func loadPasskeyLoginCeremony(ctx context.Context, tx database.TenantTx, tokenHash []byte, relyingPartyID string) (challengeID, accountID, reference string, expiresAt time.Time, generation int64, err error) {
	err = tx.QueryRow(ctx, `
		SELECT id::text, account_id::text, ceremony_reference, expires_at, session_generation
		FROM werk_core.identity_mfa_challenges
		WHERE ceremony_token_hash = $1 AND purpose = 'authentication' AND relying_party_id = $2 AND used_at IS NULL
		FOR UPDATE
	`, tokenHash, relyingPartyID).Scan(&challengeID, &accountID, &reference, &expiresAt, &generation)
	if errors.Is(err, pgx.ErrNoRows) {
		err = identity.ErrInvalidCredentials
	}
	return
}

func loadAnonymousPasskeyCeremony(ctx context.Context, tx database.TenantTx, tokenHash []byte, relyingPartyID string) (challengeID, reference string, expiresAt time.Time, err error) {
	err = tx.QueryRow(ctx, `
		SELECT id::text, session_reference, expires_at
		FROM werk_core.identity_authentication_ceremonies
		WHERE token_hash = $1 AND ceremony_kind = 'passkey-login'
		  AND relying_party_id = $2 AND used_at IS NULL
		FOR UPDATE
	`, tokenHash, relyingPartyID).Scan(&challengeID, &reference, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = identity.ErrInvalidCredentials
	}
	return
}

func (service *Service) loadDiscoverablePasskeyUser(ctx context.Context, tx database.TenantTx, rawID, userHandle []byte) (*passkeyUser, error) {
	if len(userHandle) != identity.PasskeyUserHandleSize || len(rawID) < identity.PasskeyCredentialIDMinSize ||
		len(rawID) > identity.PasskeyCredentialIDMaxSize {
		return nil, identity.ErrInvalidCredentials
	}
	var accountBytes [16]byte
	copy(accountBytes[:], userHandle)
	accountID := formatUUID(accountBytes)
	credentialHash := sha256.Sum256(rawID)
	var matchedAccount string
	if err := tx.QueryRow(ctx, `
		SELECT account.id::text
		FROM werk_core.identity_mfa_factors AS factor
		JOIN werk_core.accounts AS account ON account.id = factor.account_id
		LEFT JOIN werk_core.tenants AS tenant ON tenant.id = account.tenant_id
		WHERE factor.account_id = $1::uuid
		  AND factor.credential_id_hash = $2
		  AND factor.factor_kind = 'webauthn' AND factor.status = 'active'
		  AND factor.relying_party_id = $3
		  AND account.status = 'active'
		  AND (account.tenant_id IS NULL OR tenant.status = 'active')
	`, accountID, credentialHash[:], service.webAuthn.Config.RPID).Scan(&matchedAccount); err != nil || matchedAccount != accountID {
		return nil, identity.ErrInvalidCredentials
	}
	user, err := service.loadPasskeyUser(ctx, tx, accountID, true)
	if err != nil || len(user.credentials) == 0 {
		return nil, identity.ErrInvalidCredentials
	}
	return user, nil
}

func lookupPasskeyLoginCeremonyAccount(ctx context.Context, tx database.TenantTx, tokenHash []byte, relyingPartyID string) (string, error) {
	var accountID string
	err := tx.QueryRow(ctx, `
		SELECT account_id::text
		FROM werk_core.identity_mfa_challenges
		WHERE ceremony_token_hash = $1 AND purpose = 'authentication'
		  AND relying_party_id = $2 AND used_at IS NULL
	`, tokenHash, relyingPartyID).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", identity.ErrInvalidCredentials
	}
	return accountID, err
}

func (service *Service) decryptPasskeySession(accountID, challengeID, reference string) (webauthn.SessionData, error) {
	plaintext, err := service.decryptMFASecret(accountID, challengeID, reference)
	if err != nil {
		return webauthn.SessionData{}, identity.ErrPasskeyInvalid
	}
	var session webauthn.SessionData
	if err := json.Unmarshal([]byte(plaintext), &session); err != nil {
		return webauthn.SessionData{}, identity.ErrPasskeyInvalid
	}
	return session, nil
}

func tenantText(tenantID *tenancy.TenantID) string {
	if tenantID == nil {
		return ""
	}
	return tenantID.String()
}

func nullableTenant(tenantID *tenancy.TenantID) any {
	if tenantID == nil {
		return nil
	}
	return tenantID.String()
}
