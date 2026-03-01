BEGIN;

-- Connector agent registry
CREATE TABLE connector_agents (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID NOT NULL REFERENCES tenants(id),
    service_account_id UUID NOT NULL REFERENCES service_accounts(id),
    agent_version      TEXT NOT NULL DEFAULT '',
    tally_company      TEXT NOT NULL DEFAULT '',
    tally_port         INT NOT NULL DEFAULT 0,
    os_info            TEXT NOT NULL DEFAULT '',
    status             TEXT NOT NULL DEFAULT 'registered',
    last_heartbeat     TIMESTAMPTZ,
    registered_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, service_account_id)
);

CREATE INDEX idx_connector_agents_tenant ON connector_agents(tenant_id);

-- Tally ledgers (per-tenant)
CREATE TABLE tally_ledgers (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL REFERENCES tenants(id),
    name         TEXT NOT NULL,
    parent_group TEXT NOT NULL DEFAULT '',
    gstin        TEXT NOT NULL DEFAULT '',
    state        TEXT NOT NULL DEFAULT '',
    tax_type     TEXT NOT NULL DEFAULT '',
    tax_rate     NUMERIC NOT NULL DEFAULT 0,
    is_revenue   BOOLEAN NOT NULL DEFAULT FALSE,
    synced_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

CREATE INDEX idx_tally_ledgers_tenant ON tally_ledgers(tenant_id);
CREATE INDEX idx_tally_ledgers_gstin ON tally_ledgers(tenant_id, gstin) WHERE gstin != '';

-- Tally stock items (per-tenant)
CREATE TABLE tally_stock_items (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL REFERENCES tenants(id),
    name         TEXT NOT NULL,
    parent_group TEXT NOT NULL DEFAULT '',
    hsn_code     TEXT NOT NULL DEFAULT '',
    default_uom  TEXT NOT NULL DEFAULT '',
    synced_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

CREATE INDEX idx_tally_stock_items_tenant ON tally_stock_items(tenant_id);
CREATE INDEX idx_tally_stock_items_hsn ON tally_stock_items(tenant_id, hsn_code) WHERE hsn_code != '';

-- Tally godowns (per-tenant)
CREATE TABLE tally_godowns (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name      TEXT NOT NULL,
    parent    TEXT NOT NULL DEFAULT '',
    synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

-- Tally units of measure (per-tenant)
CREATE TABLE tally_units (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    symbol      TEXT NOT NULL,
    formal_name TEXT NOT NULL DEFAULT '',
    synced_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, symbol)
);

-- Tally cost centres (per-tenant)
CREATE TABLE tally_cost_centres (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name      TEXT NOT NULL,
    parent    TEXT NOT NULL DEFAULT '',
    synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

-- Sync event log
CREATE TABLE sync_events (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID NOT NULL REFERENCES tenants(id),
    agent_id             UUID NOT NULL REFERENCES connector_agents(id),
    document_id          UUID REFERENCES documents(id) ON DELETE SET NULL,
    direction            TEXT NOT NULL,
    status               TEXT NOT NULL,
    tally_voucher_id     TEXT NOT NULL DEFAULT '',
    tally_voucher_number TEXT NOT NULL DEFAULT '',
    error_message        TEXT NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sync_events_tenant ON sync_events(tenant_id);
CREATE INDEX idx_sync_events_agent ON sync_events(agent_id);
CREATE INDEX idx_sync_events_document ON sync_events(document_id) WHERE document_id IS NOT NULL;

-- Inbound Tally vouchers for reconciliation
CREATE TABLE tally_vouchers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    voucher_type    TEXT NOT NULL,
    voucher_number  TEXT NOT NULL DEFAULT '',
    voucher_date    DATE,
    party_name      TEXT NOT NULL DEFAULT '',
    party_gstin     TEXT NOT NULL DEFAULT '',
    amount          NUMERIC NOT NULL DEFAULT 0,
    narration       TEXT NOT NULL DEFAULT '',
    ledger_entries  JSONB NOT NULL DEFAULT '[]',
    remote_id       TEXT NOT NULL DEFAULT '',
    tally_master_id TEXT NOT NULL DEFAULT '',
    synced_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, tally_master_id)
);

CREATE INDEX idx_tally_vouchers_tenant ON tally_vouchers(tenant_id);

COMMIT;
