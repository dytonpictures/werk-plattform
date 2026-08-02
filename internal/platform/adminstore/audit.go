package adminstore

import (
	"context"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dytonpictures/werk/internal/core/identity"
	"github.com/dytonpictures/werk/internal/core/tenancy"
	"github.com/dytonpictures/werk/internal/platform/database"
)

const (
	defaultSecurityAuditLimit = 50
	maxSecurityAuditLimit     = 100
)

var securityAuditEventTypePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*(?:[.][a-z][a-z0-9-]*)+[.]v[1-9][0-9]*$`)

type SecurityAuditCursor struct {
	OccurredAt time.Time `json:"occurred_at"`
	ID         string    `json:"id"`
}

type SecurityAuditQuery struct {
	TenantID  string
	EventType string
	Outcome   string
	Limit     int
	Cursor    *SecurityAuditCursor
}

type SecurityAuditEventView struct {
	ID                string    `json:"id"`
	OccurredAt        time.Time `json:"occurred_at"`
	EventType         string    `json:"event_type"`
	Outcome           string    `json:"outcome"`
	ActorAccountID    string    `json:"actor_account_id,omitempty"`
	ActorAccountClass string    `json:"actor_account_class,omitempty"`
	TenantID          string    `json:"tenant_id,omitempty"`
	TenantName        string    `json:"tenant_name,omitempty"`
	RequestID         string    `json:"request_id"`
	CorrelationID     string    `json:"correlation_id"`
}

type SecurityAuditPage struct {
	Items      []SecurityAuditEventView
	NextCursor *SecurityAuditCursor
}

func (service *Service) ListSecurityAuditEvents(ctx context.Context, query SecurityAuditQuery, actor identity.AuthenticatedActor, requestID, correlationID string) (SecurityAuditPage, error) {
	normalized, tenantFilter, err := normalizeSecurityAuditQuery(query)
	if err != nil {
		return SecurityAuditPage{}, err
	}
	auditID, err := randomUUID()
	if err != nil {
		return SecurityAuditPage{}, err
	}

	page := SecurityAuditPage{Items: make([]SecurityAuditEventView, 0, normalized.Limit)}
	err = service.database.WithinInstallationAuditRead(ctx, func(ctx context.Context, tx database.TenantTx) error {
		var cursorTime any
		var cursorID any
		if normalized.Cursor != nil {
			cursorTime = normalized.Cursor.OccurredAt
			cursorID = normalized.Cursor.ID
		}
		rows, err := tx.Query(ctx, `
			SELECT audit.id::text, audit.occurred_at, audit.event_type, audit.outcome,
			       COALESCE(audit.account_id::text, ''),
			       COALESCE(werk_security.security_audit_account_class(audit.account_id), ''),
			       COALESCE(audit.tenant_id::text, ''),
			       COALESCE(tenant.name, ''), audit.request_id::text, audit.correlation_id::text
			FROM werk_core.security_audit_events AS audit
			LEFT JOIN werk_core.tenants AS tenant ON tenant.id = audit.tenant_id
			WHERE ($1::uuid IS NULL OR audit.tenant_id = $1::uuid)
			  AND ($2::text = '' OR audit.event_type = $2::text)
			  AND ($3::text = '' OR audit.outcome = $3::text)
			  AND ($4::timestamptz IS NULL OR (audit.occurred_at, audit.id) < ($4::timestamptz, $5::uuid))
			ORDER BY audit.occurred_at DESC, audit.id DESC
			LIMIT $6
		`, tenantFilter, normalized.EventType, normalized.Outcome, cursorTime, cursorID, normalized.Limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item SecurityAuditEventView
			if err := rows.Scan(
				&item.ID, &item.OccurredAt, &item.EventType, &item.Outcome,
				&item.ActorAccountID, &item.ActorAccountClass, &item.TenantID, &item.TenantName,
				&item.RequestID, &item.CorrelationID,
			); err != nil {
				return err
			}
			page.Items = append(page.Items, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(page.Items) > normalized.Limit {
			page.Items = page.Items[:normalized.Limit]
			last := page.Items[len(page.Items)-1]
			page.NextCursor = &SecurityAuditCursor{OccurredAt: last.OccurredAt, ID: last.ID}
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO werk_core.security_audit_events (
				id, occurred_at, event_type, outcome, account_id, tenant_id,
				request_id, correlation_id, details
			) VALUES (
				$1::uuid, $2, 'core.audit.security-events-listed.v1', 'succeeded',
				$3::uuid, NULL, $4::uuid, $5::uuid,
				jsonb_strip_nulls(jsonb_build_object(
					'result_count', $6::integer,
					'tenant_filter', $7::text,
					'event_type_filter', NULLIF($8::text, ''),
					'outcome_filter', NULLIF($9::text, ''),
					'cursor_used', $10::boolean
				))
			)
		`, auditID, service.now(), formatUUID(actor.AccountID), requestID, correlationID,
			len(page.Items), tenantFilter, normalized.EventType, normalized.Outcome, normalized.Cursor != nil)
		return err
	})
	if err != nil {
		return SecurityAuditPage{}, err
	}
	return page, nil
}

func normalizeSecurityAuditQuery(query SecurityAuditQuery) (SecurityAuditQuery, any, error) {
	query.TenantID = strings.TrimSpace(query.TenantID)
	query.EventType = strings.TrimSpace(query.EventType)
	query.Outcome = strings.TrimSpace(query.Outcome)
	if query.Limit == 0 {
		query.Limit = defaultSecurityAuditLimit
	}
	if query.Limit < 1 || query.Limit > maxSecurityAuditLimit {
		return SecurityAuditQuery{}, nil, fmt.Errorf("%w: limit must be between 1 and 100", ErrInvalidAuditQuery)
	}
	var tenantFilter any
	if query.TenantID != "" {
		tenantID, err := tenancy.ParseTenantID(query.TenantID)
		if err != nil {
			return SecurityAuditQuery{}, nil, fmt.Errorf("%w: tenant filter", ErrInvalidAuditQuery)
		}
		query.TenantID = tenantID.String()
		tenantFilter = query.TenantID
	}
	if len(query.EventType) > 256 || (query.EventType != "" && !securityAuditEventTypePattern.MatchString(query.EventType)) {
		return SecurityAuditQuery{}, nil, fmt.Errorf("%w: event type filter", ErrInvalidAuditQuery)
	}
	if query.Outcome != "" && query.Outcome != "succeeded" && query.Outcome != "denied" && query.Outcome != "failed" {
		return SecurityAuditQuery{}, nil, fmt.Errorf("%w: outcome filter", ErrInvalidAuditQuery)
	}
	if query.Cursor != nil {
		if query.Cursor.OccurredAt.IsZero() || !validCanonicalUUID(query.Cursor.ID) {
			return SecurityAuditQuery{}, nil, fmt.Errorf("%w: cursor", ErrInvalidAuditQuery)
		}
	}
	return query, tenantFilter, nil
}

func validCanonicalUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil && len(raw) == 16
}
