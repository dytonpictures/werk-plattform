INSERT INTO werk_core.permissions (
    id, permission_key, display_name, owning_module, access_plane, risk_level
) VALUES (
    '0196f000-0000-7000-8000-000000000906',
    'core.documents.document.list',
    'Eigene Dokumente auflisten',
    'core.documents',
    'work',
    'medium'
);

INSERT INTO werk_core.permission_resource_types (permission_id, resource_kind)
SELECT permission.id, 'core.documents.collection'
FROM werk_core.permissions AS permission
WHERE permission.permission_key = 'core.documents.document.list';

INSERT INTO werk_core.permission_processing_policies (
    permission_id, resource_kind, processing_required,
    activity_key, purpose_key, legal_basis_ref
)
SELECT permission.id, 'core.documents.collection', true,
       'core.documents.document-management',
       'core.documents.document-use',
       'operator.processing-register.documents'
FROM werk_core.permissions AS permission
WHERE permission.permission_key = 'core.documents.document.list';

COMMENT ON COLUMN werk_core.documents.created_by_account_id IS
    'The first public work read slice uses this field as a deliberately narrow created-by-me visibility rule. Shared visibility requires a later explicit document-local contract.';

CREATE INDEX idx_documents_tenant_creator_updated
    ON werk_core.documents (tenant_id, created_by_account_id, updated_at DESC, id DESC);
