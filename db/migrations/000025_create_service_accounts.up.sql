BEGIN;

CREATE TABLE service_accounts (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name          VARCHAR(255) NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    api_key_prefix VARCHAR(8) NOT NULL,
    api_key_hash  VARCHAR(255) NOT NULL,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_by    UUID NOT NULL,
    last_used_at  TIMESTAMPTZ,
    expires_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

CREATE INDEX idx_service_accounts_tenant_id ON service_accounts (tenant_id);
CREATE INDEX idx_service_accounts_api_key_prefix ON service_accounts (api_key_prefix);

-- Permission grants for service accounts on collections
CREATE TABLE service_account_permissions (
    id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    service_account_id UUID NOT NULL REFERENCES service_accounts(id) ON DELETE CASCADE,
    collection_id      UUID NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    tenant_id          UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    permission         VARCHAR(50) NOT NULL DEFAULT 'viewer',
    granted_by         UUID NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(service_account_id, collection_id)
);

CREATE INDEX idx_sa_permissions_sa_id ON service_account_permissions (service_account_id);
CREATE INDEX idx_sa_permissions_collection_id ON service_account_permissions (collection_id);

COMMIT;
