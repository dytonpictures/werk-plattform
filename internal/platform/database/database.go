package database

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dytonpictures/werk/internal/core/tenancy"
)

const (
	WorkRuntimeRole     = "werk_work_runtime"
	IdentityRuntimeRole = "werk_identity_runtime"
	AdminRuntimeRole    = "werk_admin_runtime"
	ServiceRuntimeRole  = "werk_service_runtime"
	WorkerRuntimeRole   = "werk_worker_runtime"
	ownerRole           = "werk_owner"
	defaultWorkMaxConns = 16
	defaultRoleMaxConns = 8
	maximumPoolConns    = 128
)

type RuntimeOptions struct {
	ExpectedRole    string
	ApplicationName string
	MaxConnections  int32
}

type RuntimeDB struct {
	pool *pgxpool.Pool
}

type PoolSnapshot struct {
	Max          int32
	Total        int32
	Acquired     int32
	Idle         int32
	Constructing int32
	AcquireWaits int64
	AcquireTime  time.Duration
}

// IdentityDB is intentionally separate from RuntimeDB. It has no tenant-scoped
// methods and may only be constructed for the constrained identity role.
type IdentityDB struct {
	pool *pgxpool.Pool
}

// WorkDB is the tenant-bound runtime used only for interactive work requests.
// It deliberately exposes no installation-wide operation.
type WorkDB struct{ runtime *RuntimeDB }

type AdminDB struct{ runtime *RuntimeDB }

// ServiceDB is the tenant-bound runtime for authenticated platform services.
// It exposes no installation-wide operation and never infers a tenant.
type ServiceDB struct{ runtime *RuntimeDB }

// WorkerDB has explicit global operations for worker-owned infrastructure such
// as the outbox. Business data remains accessible only through tenant methods.
type WorkerDB struct {
	runtime *RuntimeDB
}

type TenantTx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type TenantOperation func(context.Context, TenantTx) error

func NewMigrationPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	return openPool(ctx, databaseURL, nil)
}

func NewRuntime(ctx context.Context, databaseURL string, options RuntimeOptions) (*RuntimeDB, error) {
	if options.ExpectedRole == "" {
		return nil, errors.New("runtime database expected role is required")
	}
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse runtime database configuration: %w", err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = "pg_catalog,werk_core"
	poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = "30s"
	poolConfig.ConnConfig.RuntimeParams["lock_timeout"] = "5s"
	poolConfig.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "30s"
	if options.ApplicationName != "" {
		poolConfig.ConnConfig.RuntimeParams["application_name"] = options.ApplicationName
	}
	if options.MaxConnections > 0 && !poolParameterConfigured(databaseURL, "pool_max_conns") {
		poolConfig.MaxConns = options.MaxConnections
	}
	if !poolParameterConfigured(databaseURL, "pool_max_conn_lifetime_jitter") {
		poolConfig.MaxConnLifetimeJitter = 5 * time.Minute
	}
	if poolConfig.MaxConns < 1 || poolConfig.MaxConns > maximumPoolConns ||
		poolConfig.MinConns < 0 || poolConfig.MinConns > poolConfig.MaxConns ||
		poolConfig.MaxConnLifetime < 5*time.Minute || poolConfig.MaxConnLifetime > 24*time.Hour ||
		poolConfig.MaxConnIdleTime < time.Minute || poolConfig.MaxConnIdleTime > 4*time.Hour ||
		poolConfig.MaxConnLifetimeJitter < 0 || poolConfig.MaxConnLifetimeJitter > time.Hour {
		return nil, errors.New("runtime database pool configuration is outside supported bounds")
	}
	poolConfig.AfterConnect = func(connectContext context.Context, connection *pgx.Conn) error {
		return verifyRuntimeConnection(connectContext, connection, options.ExpectedRole)
	}
	poolConfig.AfterRelease = scrubRuntimeConnection

	pool, err := openPool(ctx, databaseURL, poolConfig)
	if err != nil {
		return nil, err
	}
	return &RuntimeDB{pool: pool}, nil
}

func NewIdentity(ctx context.Context, databaseURL, applicationName string) (*IdentityDB, error) {
	runtime, err := NewRuntime(ctx, databaseURL, RuntimeOptions{
		ExpectedRole:    IdentityRuntimeRole,
		ApplicationName: applicationName,
		MaxConnections:  defaultRoleMaxConns,
	})
	if err != nil {
		return nil, err
	}
	return &IdentityDB{pool: runtime.pool}, nil
}

func NewWork(ctx context.Context, databaseURL, applicationName string) (*WorkDB, error) {
	runtime, err := NewRuntime(ctx, databaseURL, RuntimeOptions{
		ExpectedRole: WorkRuntimeRole, ApplicationName: applicationName,
		MaxConnections: defaultWorkMaxConns,
	})
	if err != nil {
		return nil, err
	}
	return &WorkDB{runtime: runtime}, nil
}

func (database *WorkDB) Close()                         { database.runtime.Close() }
func (database *WorkDB) Ping(ctx context.Context) error { return database.runtime.Ping(ctx) }
func (database *WorkDB) PoolSnapshot() PoolSnapshot     { return database.runtime.PoolSnapshot() }
func (database *WorkDB) WithinTenantRead(ctx context.Context, tenantID tenancy.TenantID, operation TenantOperation) error {
	return database.runtime.WithinTenantRead(ctx, tenantID, operation)
}

func NewAdmin(ctx context.Context, databaseURL, applicationName string) (*AdminDB, error) {
	runtime, err := NewRuntime(ctx, databaseURL, RuntimeOptions{
		ExpectedRole: AdminRuntimeRole, ApplicationName: applicationName,
		MaxConnections: defaultRoleMaxConns,
	})
	if err != nil {
		return nil, err
	}
	return &AdminDB{runtime: runtime}, nil
}

func (database *AdminDB) Close()                         { database.runtime.Close() }
func (database *AdminDB) Ping(ctx context.Context) error { return database.runtime.Ping(ctx) }
func (database *AdminDB) PoolSnapshot() PoolSnapshot     { return database.runtime.PoolSnapshot() }

// WithinInstallationRead is reserved for explicitly authorized installation
// administration queries. It never sets a tenant context and cannot mutate.
func (database *AdminDB) WithinInstallationRead(ctx context.Context, operation TenantOperation) error {
	return database.withinInstallation(ctx, pgx.ReadOnly, operation)
}

// WithinInstallationAuditRead is the narrow read-write boundary for an
// installation-wide administrative read that must record its own security
// audit entry atomically. Database grants and RLS remain the mutation boundary.
func (database *AdminDB) WithinInstallationAuditRead(ctx context.Context, operation TenantOperation) error {
	return database.withinInstallation(ctx, pgx.ReadWrite, operation)
}

func (database *AdminDB) WithinTenantRead(ctx context.Context, tenantID tenancy.TenantID, operation TenantOperation) error {
	return database.runtime.WithinTenantRead(ctx, tenantID, operation)
}

func (database *AdminDB) withinInstallation(ctx context.Context, accessMode pgx.TxAccessMode, operation TenantOperation) (err error) {
	if operation == nil {
		return errors.New("installation admin transaction requires an operation")
	}
	return withinTransaction(ctx, database.runtime.pool, accessMode, "installation admin", operation)
}
func (database *AdminDB) WithinTenantWrite(ctx context.Context, tenantID tenancy.TenantID, operation TenantOperation) error {
	return database.runtime.WithinTenantWrite(ctx, tenantID, operation)
}

func NewService(ctx context.Context, databaseURL, applicationName string) (*ServiceDB, error) {
	runtime, err := NewRuntime(ctx, databaseURL, RuntimeOptions{
		ExpectedRole:    ServiceRuntimeRole,
		ApplicationName: applicationName,
		MaxConnections:  defaultRoleMaxConns,
	})
	if err != nil {
		return nil, err
	}
	return &ServiceDB{runtime: runtime}, nil
}

func (database *ServiceDB) Close()                         { database.runtime.Close() }
func (database *ServiceDB) Ping(ctx context.Context) error { return database.runtime.Ping(ctx) }
func (database *ServiceDB) WithinTenantRead(ctx context.Context, tenantID tenancy.TenantID, operation TenantOperation) error {
	return database.runtime.WithinTenantRead(ctx, tenantID, operation)
}
func (database *ServiceDB) WithinTenantWrite(ctx context.Context, tenantID tenancy.TenantID, operation TenantOperation) error {
	return database.runtime.WithinTenantWrite(ctx, tenantID, operation)
}

func NewWorker(ctx context.Context, databaseURL, applicationName string) (*WorkerDB, error) {
	runtime, err := NewRuntime(ctx, databaseURL, RuntimeOptions{
		ExpectedRole:    WorkerRuntimeRole,
		ApplicationName: applicationName,
		MaxConnections:  defaultWorkMaxConns,
	})
	if err != nil {
		return nil, err
	}
	return &WorkerDB{runtime: runtime}, nil
}

func (database *WorkerDB) Close()                         { database.runtime.Close() }
func (database *WorkerDB) Ping(ctx context.Context) error { return database.runtime.Ping(ctx) }
func (database *WorkerDB) WithinTenantWrite(ctx context.Context, tenantID tenancy.TenantID, operation TenantOperation) error {
	return database.runtime.WithinTenantWrite(ctx, tenantID, operation)
}
func (database *WorkerDB) WithinGlobalWrite(ctx context.Context, operation TenantOperation) error {
	return database.withinGlobal(ctx, pgx.ReadWrite, operation)
}

// ListenAsyncWork dedicates one pooled worker connection to a payloadless
// acceleration signal. PostgreSQL queues remain authoritative; callers must
// retain polling because notifications are intentionally transient.
func (database *WorkerDB) ListenAsyncWork(ctx context.Context, wakeups chan<- struct{}) error {
	if database == nil || database.runtime == nil || wakeups == nil {
		return errors.New("async work listener is not configured")
	}
	connection, err := database.runtime.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire async work listener: %w", err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, `LISTEN werk_async_work`); err != nil {
		return fmt.Errorf("listen for async work: %w", err)
	}
	for {
		if _, err := connection.Conn().WaitForNotification(ctx); err != nil {
			return fmt.Errorf("wait for async work: %w", err)
		}
		select {
		case wakeups <- struct{}{}:
		default:
		}
	}
}

func (database *WorkerDB) withinGlobal(ctx context.Context, accessMode pgx.TxAccessMode, operation TenantOperation) (err error) {
	if operation == nil {
		return errors.New("global worker transaction requires an operation")
	}
	return withinTransaction(ctx, database.runtime.pool, accessMode, "global worker", operation)
}

func (database *IdentityDB) Close() {
	database.pool.Close()
}

func (database *IdentityDB) Ping(ctx context.Context) error {
	return database.pool.Ping(ctx)
}

func (database *IdentityDB) PoolSnapshot() PoolSnapshot {
	return poolSnapshot(database.pool)
}

func (database *IdentityDB) WithinWrite(ctx context.Context, operation TenantOperation) (err error) {
	return database.within(ctx, pgx.ReadWrite, operation)
}

func (database *IdentityDB) WithinRead(ctx context.Context, operation TenantOperation) (err error) {
	return database.within(ctx, pgx.ReadOnly, operation)
}

func (database *IdentityDB) within(ctx context.Context, accessMode pgx.TxAccessMode, operation TenantOperation) (err error) {
	if operation == nil {
		return errors.New("identity transaction requires an operation")
	}
	return withinTransaction(ctx, database.pool, accessMode, "identity", operation)
}

func (database *RuntimeDB) Close() {
	database.pool.Close()
}

func (database *RuntimeDB) Ping(ctx context.Context) error {
	return database.pool.Ping(ctx)
}

func (database *RuntimeDB) PoolSnapshot() PoolSnapshot {
	if database == nil {
		return PoolSnapshot{}
	}
	return poolSnapshot(database.pool)
}

func (database *RuntimeDB) WithinTenantRead(ctx context.Context, tenantID tenancy.TenantID, operation TenantOperation) error {
	return database.withinTenant(ctx, tenantID, pgx.ReadOnly, operation)
}

func (database *RuntimeDB) WithinTenantWrite(ctx context.Context, tenantID tenancy.TenantID, operation TenantOperation) error {
	return database.withinTenant(ctx, tenantID, pgx.ReadWrite, operation)
}

func (database *RuntimeDB) withinTenant(ctx context.Context, tenantID tenancy.TenantID, accessMode pgx.TxAccessMode, operation TenantOperation) (err error) {
	if tenantID.IsZero() {
		return errors.New("tenant transaction requires a tenant ID")
	}
	if operation == nil {
		return errors.New("tenant transaction requires an operation")
	}
	return withinTransaction(ctx, database.pool, accessMode, "tenant", func(ctx context.Context, transaction TenantTx) error {
		var configuredTenant string
		if err := transaction.QueryRow(ctx, `
			SELECT pg_catalog.set_config('werk.tenant_id', $1::uuid::text, true)
		`, tenantID.String()).Scan(&configuredTenant); err != nil {
			return fmt.Errorf("set transaction tenant context: %w", err)
		}
		if configuredTenant != tenantID.String() {
			return errors.New("database did not accept the requested tenant context")
		}
		return operation(ctx, transaction)
	})
}

func withinTransaction(ctx context.Context, pool *pgxpool.Pool, accessMode pgx.TxAccessMode, label string, operation TenantOperation) (err error) {
	transaction, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: accessMode})
	if err != nil {
		return fmt.Errorf("begin %s transaction: %w", label, err)
	}
	completed := false
	defer func() {
		if !completed {
			rollbackContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = transaction.Rollback(rollbackContext)
		}
		if recovered := recover(); recovered != nil {
			panic(recovered)
		}
	}()
	if err := operation(ctx, transaction); err != nil {
		return err
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit %s transaction: %w", label, err)
	}
	completed = true
	return nil
}

func openPool(ctx context.Context, databaseURL string, poolConfig *pgxpool.Config) (*pgxpool.Pool, error) {
	var pool *pgxpool.Pool
	var err error
	if poolConfig == nil {
		pool, err = pgxpool.New(ctx, databaseURL)
	} else {
		pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
	}
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	checkContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(checkContext); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return pool, nil
}

func verifyRuntimeConnection(ctx context.Context, connection *pgx.Conn, expectedRole string) error {
	var currentUser string
	var superuser bool
	var bypassRLS bool
	var ownerMember bool
	var ownsWerkObjects bool
	if err := connection.QueryRow(ctx, `
		SELECT
			current_user,
			role.rolsuper,
			role.rolbypassrls,
			pg_catalog.pg_has_role(current_user, 'werk_owner', 'MEMBER'),
			EXISTS (
				SELECT 1
				FROM pg_catalog.pg_class AS relation
				JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
				WHERE namespace.nspname IN ('werk_core', 'werk_security')
				  AND relation.relowner = role.oid
			)
		FROM pg_catalog.pg_roles AS role
		WHERE role.rolname = current_user
	`).Scan(&currentUser, &superuser, &bypassRLS, &ownerMember, &ownsWerkObjects); err != nil {
		return fmt.Errorf("inspect runtime database role: %w", err)
	}
	if currentUser != expectedRole {
		return fmt.Errorf("runtime database connected as %q, expected %q", currentUser, expectedRole)
	}
	if superuser || bypassRLS || ownerMember || ownsWerkObjects {
		return fmt.Errorf("runtime database role %q violates non-owner RLS requirements", currentUser)
	}
	return nil
}

func scrubRuntimeConnection(connection *pgx.Conn) bool {
	if connection.PgConn().TxStatus() != 'I' {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var value string
	if err := connection.QueryRow(ctx, `
		SELECT pg_catalog.set_config('werk.tenant_id', '', false)
	`).Scan(&value); err != nil {
		return false
	}
	return value == ""
}

func poolParameterConfigured(databaseURL, key string) bool {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return false
	}
	_, configured := parsed.Query()[key]
	return configured
}

func poolSnapshot(pool *pgxpool.Pool) PoolSnapshot {
	if pool == nil {
		return PoolSnapshot{}
	}
	stats := pool.Stat()
	return PoolSnapshot{
		Max: stats.MaxConns(), Total: stats.TotalConns(),
		Acquired: stats.AcquiredConns(), Idle: stats.IdleConns(),
		Constructing: stats.ConstructingConns(),
		AcquireWaits: stats.EmptyAcquireCount(), AcquireTime: stats.AcquireDuration(),
	}
}
