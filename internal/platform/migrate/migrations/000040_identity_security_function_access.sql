-- Identity runtime can execute only explicitly granted security contracts.
-- Schema usage reveals no object contents and does not widen table access.
GRANT USAGE ON SCHEMA werk_security TO werk_identity_runtime;
