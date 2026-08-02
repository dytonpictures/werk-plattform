CREATE OR REPLACE FUNCTION werk_core.notify_async_runtime_work()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog
AS $$
BEGIN
    PERFORM pg_catalog.pg_notify('werk_async_work', '');
    RETURN NULL;
END;
$$;

REVOKE ALL ON FUNCTION werk_core.notify_async_runtime_work() FROM PUBLIC;

CREATE TRIGGER outbox_events_async_runtime_wakeup
AFTER INSERT ON werk_core.outbox_events
FOR EACH STATEMENT
EXECUTE FUNCTION werk_core.notify_async_runtime_work();

CREATE TRIGGER security_audit_export_async_runtime_wakeup
AFTER INSERT ON werk_core.security_audit_export_queue
FOR EACH STATEMENT
EXECUTE FUNCTION werk_core.notify_async_runtime_work();
