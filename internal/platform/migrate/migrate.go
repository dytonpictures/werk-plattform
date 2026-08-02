package migrate

import (
	"context"
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const migrationAdvisoryLockKey int64 = 0x5745524b4d494752

const (
	migrationLoginRole = "werk_migrator"
	migrationOwnerRole = "werk_owner"
)

type migration struct {
	name     string
	contents string
	checksum [32]byte
}

func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer connection.Release()
	if err := assumeMigrationOwner(ctx, connection); err != nil {
		return err
	}
	defer func() {
		resetContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = connection.Exec(resetContext, `RESET ROLE`)
	}()

	if _, err := connection.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationAdvisoryLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		unlockContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = connection.Exec(unlockContext, `SELECT pg_advisory_unlock($1)`, migrationAdvisoryLockKey)
	}()

	if _, err := connection.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS werk_core`); err != nil {
		return fmt.Errorf("create core schema: %w", err)
	}
	if _, err := connection.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS werk_core.schema_migrations (
			name text PRIMARY KEY,
			checksum text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		if err := applyOne(ctx, connection, migration); err != nil {
			return err
		}
	}
	return nil
}

func assumeMigrationOwner(ctx context.Context, connection *pgxpool.Conn) error {
	var sessionUser string
	var currentUser string
	var superuser bool
	var bypassRLS bool
	if err := connection.QueryRow(ctx, `
		SELECT session_user, current_user, role.rolsuper, role.rolbypassrls
		FROM pg_catalog.pg_roles AS role
		WHERE role.rolname = session_user
	`).Scan(&sessionUser, &currentUser, &superuser, &bypassRLS); err != nil {
		return fmt.Errorf("inspect migration login role: %w", err)
	}
	if sessionUser != migrationLoginRole || currentUser != migrationLoginRole {
		return fmt.Errorf("migration must connect directly as %s", migrationLoginRole)
	}
	if superuser || bypassRLS {
		return fmt.Errorf("migration login must not be superuser or bypass row security")
	}
	if _, err := connection.Exec(ctx, `SET ROLE werk_owner`); err != nil {
		return fmt.Errorf("assume migration owner role: %w", err)
	}

	var owner string
	var ownerCanLogin bool
	var ownerSuperuser bool
	var ownerBypassRLS bool
	if err := connection.QueryRow(ctx, `
		SELECT current_user, role.rolcanlogin, role.rolsuper, role.rolbypassrls
		FROM pg_catalog.pg_roles AS role
		WHERE role.rolname = current_user
	`).Scan(&owner, &ownerCanLogin, &ownerSuperuser, &ownerBypassRLS); err != nil {
		return fmt.Errorf("inspect migration owner role: %w", err)
	}
	if owner != migrationOwnerRole || ownerCanLogin || ownerSuperuser || ownerBypassRLS {
		return fmt.Errorf("migration owner role does not satisfy the required security attributes")
	}
	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	migrations := make([]migration, 0, len(entries))
	versions := make(map[uint64]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		versionText, _, found := strings.Cut(entry.Name(), "_")
		version, versionErr := strconv.ParseUint(versionText, 10, 64)
		if !found || len(versionText) != 6 || versionErr != nil || version == 0 {
			return nil, fmt.Errorf("migration %s must start with a six-digit positive version", entry.Name())
		}
		if existing, duplicate := versions[version]; duplicate {
			return nil, fmt.Errorf("migration version %06d is used by both %s and %s", version, existing, entry.Name())
		}
		versions[version] = entry.Name()
		contents, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		migrations = append(migrations, migration{name: entry.Name(), contents: string(contents), checksum: sha256.Sum256(contents)})
	}
	sort.Slice(migrations, func(left, right int) bool { return migrations[left].name < migrations[right].name })
	return migrations, nil
}

func applyOne(ctx context.Context, connection *pgxpool.Conn, migration migration) error {
	transaction, err := connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", migration.name, err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	var existingChecksum string
	err = transaction.QueryRow(ctx, `SELECT checksum FROM werk_core.schema_migrations WHERE name = $1`, migration.name).Scan(&existingChecksum)
	if err == nil {
		if existingChecksum != fmt.Sprintf("%x", migration.checksum) {
			return fmt.Errorf("migration %s checksum changed after application", migration.name)
		}
		return transaction.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read migration %s: %w", migration.name, err)
	}
	if _, err := transaction.Exec(ctx, migration.contents); err != nil {
		return fmt.Errorf("execute migration %s: %w", migration.name, err)
	}
	if _, err := transaction.Exec(ctx, `INSERT INTO werk_core.schema_migrations (name, checksum) VALUES ($1, $2)`, migration.name, fmt.Sprintf("%x", migration.checksum)); err != nil {
		return fmt.Errorf("record migration %s: %w", migration.name, err)
	}
	return transaction.Commit(ctx)
}
