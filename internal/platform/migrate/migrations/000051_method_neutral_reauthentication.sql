-- Action-bound tickets record the concrete method used, while their
-- consumption remains method-neutral and bound to session, generation,
-- permission and resource.
ALTER TABLE werk_core.identity_reauthentication_tickets
    DROP CONSTRAINT identity_reauthentication_tickets_authentication_method_check,
    ADD CONSTRAINT identity_reauthentication_tickets_authentication_method_check
        CHECK (authentication_method IN ('password', 'totp'));

