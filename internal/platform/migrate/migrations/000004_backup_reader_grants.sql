GRANT USAGE ON SCHEMA werk_core, werk_security TO werk_backup_reader;

GRANT SELECT ON ALL TABLES IN SCHEMA werk_core, werk_security
    TO werk_backup_reader;
GRANT SELECT ON ALL SEQUENCES IN SCHEMA werk_core, werk_security
    TO werk_backup_reader;

ALTER DEFAULT PRIVILEGES FOR ROLE werk_owner IN SCHEMA werk_core
    GRANT SELECT ON TABLES TO werk_backup_reader;
ALTER DEFAULT PRIVILEGES FOR ROLE werk_owner IN SCHEMA werk_core
    GRANT SELECT ON SEQUENCES TO werk_backup_reader;
ALTER DEFAULT PRIVILEGES FOR ROLE werk_owner IN SCHEMA werk_security
    GRANT SELECT ON TABLES TO werk_backup_reader;
ALTER DEFAULT PRIVILEGES FOR ROLE werk_owner IN SCHEMA werk_security
    GRANT SELECT ON SEQUENCES TO werk_backup_reader;
