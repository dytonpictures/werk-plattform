# WERK repository instructions

Read `AGENTS.md`, `docs/vision.md`, `docs/DATENMODELL.md`, and `docs/ROADMAP.md`
before proposing or changing code. Security invariants and the data model override
the roadmap; the roadmap overrides ad-hoc shortcuts.

- PostgreSQL is the business source of truth. Valkey is replaceable infrastructure.
- Every tenant operation needs explicit tenant context, server-side authorization,
  RLS where applicable, audit, and atomic outbox persistence for binding changes.
- Keep `work`, `admin`, and `service` accounts, APIs, sessions, and permissions separate.
- Never give AI or plugins direct database, production-secret, or implicit user access.
- Never edit an already-applied migration. Add a new migration and test it against a
  disposable PostgreSQL instance.
- Do not weaken tests, policy checks, audit, approvals, or branch protection to make a
  change pass.
- Prefer small changes. Preserve user changes and document public contracts.
- Report tests run and tests intentionally skipped. Human review owns every merge and
  release; an agent must not approve its own pull request or deploy to production.

