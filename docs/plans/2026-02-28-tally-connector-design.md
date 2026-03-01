# SATVOS Tally Connector — Design Document

**Date**: 2026-02-28
**Status**: Approved
**Repo**: `github.com/mudsahni/satvos-tally-connector`

## Problem

SATVOS parses and validates GST invoices, but getting that data into a customer's Tally Prime requires manual XML download and import. The current Tally export (`GET /collections/:id/export/tally`) generates XML blindly — it guesses ledger names, stock item names, and tax conventions. This causes phantom ledgers, duplicate stock items, and manual cleanup on every import.

## Solution

A lightweight on-premise agent (Go binary) that runs on the customer's Windows machine. The agent bridges SATVOS Cloud and Tally Prime via bidirectional sync:

- **Outbound (SATVOS to Tally)**: Approved documents are converted to Tally vouchers and imported via Tally's XML server on localhost:9000.
- **Inbound (Tally to SATVOS)**: Tally vouchers and masters are uploaded to SATVOS for reconciliation and smart matching.
- **Smart Matching**: SATVOS uses synced Tally masters (ledgers, stock items, etc.) to build vouchers that match the customer's actual Tally setup — matching sellers by GSTIN, items by HSN, tax ledgers by rate.

## Architecture

### System Overview

Two independently deployable components:

1. **SATVOS Server** (existing, new Sync API endpoints) — stores Tally masters, does smart matching, builds voucher definitions.
2. **SATVOS Tally Connector** (new repo) — runs on customer's Windows machine, bridges SATVOS cloud to Tally Prime via localhost.

### Communication

- **Agent to SATVOS**: HTTPS polling every 30 seconds. Agent initiates all connections outbound. No inbound ports needed on the customer's network.
- **Agent to Tally**: XML over HTTP on localhost:9000. Tally's unauthenticated port never leaves localhost.

### Sync Cycle (every 30 seconds)

```
1. HEARTBEAT → POST /sync/v1/heartbeat
   Report: tally_connected, company_name, version, errors

2. PUSH MASTERS (Tally → SATVOS)
   Agent reads ledgers, stock items, godowns, units, cost centres
   → POST /sync/v1/masters
   SATVOS stores per-tenant in tally_* tables

3. PULL OUTBOUND (SATVOS → Tally)
   Agent: GET /sync/v1/outbound?cursor=X
   SATVOS returns: documents + matched voucher definitions
   Agent: converts VoucherDef JSON → Tally XML → POST localhost:9000
   Agent: POST /sync/v1/ack (results per document)

4. PUSH INBOUND (Tally → SATVOS)
   Agent reads new/modified vouchers from Tally
   → POST /sync/v1/inbound
   SATVOS stores for reconciliation
```

### Hybrid Matching Flow

The server does the brain work (matching invoices to Tally masters). The agent does the plumbing (XML conversion + local delivery).

```
Tally Masters (on customer PC)
  ↓ agent reads via XML
SATVOS stores in tally_* tables
  ↓
Smart Voucher Builder (on SATVOS server)
  - Match seller GSTIN → tally_ledgers
  - Match HSN codes → tally_stock_items
  - Match tax rates → tax ledger names
  - Use real godown, UOM from tally data
  ↓ returns VoucherDef JSON
Agent converts → Tally XML → POST localhost:9000
```

## Connector Agent Design

### Package Structure

```
satvos-tally-connector/
├── cmd/connector/main.go        # Entry point
├── internal/
│   ├── config/config.go         # Viper-based, CONNECTOR_ prefix
│   ├── cloud/
│   │   ├── client.go            # HTTPS client to SATVOS Sync API
│   │   └── types.go             # Sync API DTOs
│   ├── tally/
│   │   ├── client.go            # XML-over-HTTP client
│   │   ├── discover.go          # Auto-discover Tally port
│   │   ├── requests.go          # XML request builders
│   │   ├── responses.go         # XML response parsers
│   │   ├── import.go            # Voucher import + result parsing
│   │   └── health.go            # Tally health check
│   ├── sync/
│   │   ├── engine.go            # Main sync loop orchestrator
│   │   ├── masters.go           # Tally masters → SATVOS
│   │   ├── outbound.go          # SATVOS → Tally voucher import
│   │   ├── inbound.go           # Tally → SATVOS
│   │   └── state.go             # Sync cursors, state tracking
│   ├── convert/
│   │   ├── xml.go               # VoucherDef → Tally XML
│   │   └── template.go          # Tally XML template
│   ├── ui/
│   │   ├── server.go            # Local HTTP server :8321
│   │   ├── handlers.go          # Setup wizard + status handlers
│   │   └── static/              # Embedded HTML/CSS/JS
│   ├── service/
│   │   └── windows.go           # Windows Service lifecycle
│   └── store/
│       └── local.go             # Local state (JSON file)
├── web/                          # UI source files
├── configs/connector.example.yaml
├── scripts/install.ps1
├── Makefile
└── README.md
```

### Configuration

Only required config is the API key. Everything else auto-discovers or uses defaults.

```yaml
satvos:
  base_url: "https://api.satvos.com"
  api_key: "sk_..."

tally:
  host: "localhost"
  port: 0          # 0 = auto-discover
  company: ""      # empty = auto-detect

sync:
  interval_seconds: 30
  batch_size: 50
  retry_attempts: 3

ui:
  port: 8321
```

Env var prefix: `CONNECTOR_` (e.g., `CONNECTOR_SATVOS_API_KEY`).

### Local State

Stored at `%APPDATA%/satvos-connector/state.json`:
- Sync cursors (outbound last-seen ID, inbound last-sync timestamp)
- Discovered Tally port and company name
- Pending retry queue for failed ACKs

### Auto-Discovery

Agent scans localhost:9000-9010, sends lightweight XML info request to each port, picks the one that responds with valid Tally data. Caches discovered port in local state.

### Local Web UI

- `GET /` — Status dashboard (connection status, sync stats, last sync time)
- `GET /setup` — Setup wizard (paste API key, confirm Tally found)
- `POST /setup/apikey` — Save API key, trigger initial connection
- `GET /api/status` — JSON status for dashboard polling
- `GET /api/logs` — Recent sync log entries
- `POST /api/sync` — Trigger immediate sync cycle

Served via `embed.FS` — entire UI baked into the binary.

## SATVOS Server-Side Changes

### New Sync API Endpoints

All under `/api/v1/sync/` with service account auth:

```
POST /sync/v1/register      # Agent registration
POST /sync/v1/heartbeat     # Liveness + status
POST /sync/v1/masters       # Upload Tally masters
GET  /sync/v1/outbound      # Pull documents for export
POST /sync/v1/ack           # Report import results
POST /sync/v1/inbound       # Upload Tally vouchers
GET  /sync/v1/config        # Agent config from server
```

### New Database Tables

```sql
-- Agent registry
CREATE TABLE connector_agents (
    id                 UUID PRIMARY KEY,
    tenant_id          UUID NOT NULL REFERENCES tenants(id),
    service_account_id UUID NOT NULL REFERENCES service_accounts(id),
    agent_version      TEXT NOT NULL,
    tally_company      TEXT,
    tally_port         INT,
    os_info            TEXT,
    status             TEXT NOT NULL DEFAULT 'registered',
    last_heartbeat     TIMESTAMPTZ,
    registered_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Tally masters (per-tenant)
CREATE TABLE tally_ledgers (
    id           UUID PRIMARY KEY,
    tenant_id    UUID NOT NULL REFERENCES tenants(id),
    name         TEXT NOT NULL,
    parent_group TEXT NOT NULL,
    gstin        TEXT,
    state        TEXT,
    tax_type     TEXT,
    tax_rate     NUMERIC,
    is_revenue   BOOLEAN DEFAULT FALSE,
    synced_at    TIMESTAMPTZ NOT NULL,
    UNIQUE(tenant_id, name)
);

CREATE TABLE tally_stock_items (
    id           UUID PRIMARY KEY,
    tenant_id    UUID NOT NULL REFERENCES tenants(id),
    name         TEXT NOT NULL,
    parent_group TEXT,
    hsn_code     TEXT,
    default_uom  TEXT,
    synced_at    TIMESTAMPTZ NOT NULL,
    UNIQUE(tenant_id, name)
);

CREATE TABLE tally_godowns (
    id        UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name      TEXT NOT NULL,
    parent    TEXT,
    synced_at TIMESTAMPTZ NOT NULL,
    UNIQUE(tenant_id, name)
);

CREATE TABLE tally_units (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    symbol      TEXT NOT NULL,
    formal_name TEXT,
    synced_at   TIMESTAMPTZ NOT NULL,
    UNIQUE(tenant_id, symbol)
);

CREATE TABLE tally_cost_centres (
    id        UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name      TEXT NOT NULL,
    parent    TEXT,
    synced_at TIMESTAMPTZ NOT NULL,
    UNIQUE(tenant_id, name)
);

-- Sync tracking
CREATE TABLE sync_events (
    id                   UUID PRIMARY KEY,
    tenant_id            UUID NOT NULL REFERENCES tenants(id),
    agent_id             UUID NOT NULL REFERENCES connector_agents(id),
    document_id          UUID REFERENCES documents(id),
    direction            TEXT NOT NULL,
    status               TEXT NOT NULL,
    tally_voucher_id     TEXT,
    tally_voucher_number TEXT,
    error_message        TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Inbound Tally vouchers
CREATE TABLE tally_vouchers (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    voucher_type    TEXT NOT NULL,
    voucher_number  TEXT,
    voucher_date    DATE,
    party_name      TEXT,
    party_gstin     TEXT,
    amount          NUMERIC,
    narration       TEXT,
    ledger_entries  JSONB,
    remote_id       TEXT,
    tally_master_id TEXT,
    synced_at       TIMESTAMPTZ NOT NULL,
    UNIQUE(tenant_id, tally_master_id)
);
```

### Smart Voucher Builder

`service/voucher_builder.go` — matching priority:

1. **Party Ledger**: GSTIN exact match → normalized name match → flag unmatched
2. **Tax Ledgers**: tax_type + tax_rate exact match → convention-based name
3. **Purchase Ledger**: rate match in Purchase Accounts group → convention-based
4. **Stock Items**: HSN exact match → flag unmatched (use description)
5. **Godown**: first godown from tally_godowns or default
6. **UOM**: symbol match in tally_units or default

Returns `VoucherDef` JSON with match confidence per entity.

### New Server-Side Files

```
handler/sync_handler.go
service/sync_service.go
service/voucher_builder.go
port/tally_master_repository.go
port/sync_repository.go
port/tally_voucher_repository.go
repository/postgres/tally_master_repo.go
repository/postgres/sync_repo.go
repository/postgres/tally_voucher_repo.go
db/migrations/000026_create_tally_connector_tables.{up,down}.sql
```

## Security

| Concern | Solution |
|---------|----------|
| Agent auth | Existing `sk_` service account keys. Auto-create `tally_connector` SA per tenant |
| TLS | All agent-to-SATVOS calls over HTTPS. Tally is localhost only |
| API key storage | `%APPDATA%/satvos-connector/config.yaml`, current-user permissions only |
| No secrets in logs | API key masked. Tally XML data not logged above DEBUG |
| Rate limiting | Server enforces per-agent rate limits on sync endpoints |
| Duplicate prevention | REMOTEID (tenantID-docID) ensures idempotent Tally imports |

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Tally not running | Skip sync cycle, report in heartbeat, retry next cycle |
| Tally import error | Log error, ACK as failed, continue with next document |
| SATVOS unreachable | Exponential backoff (5s to 60s). Queue pending ACKs locally |
| Invalid API key | Stop sync, show error on local web UI |
| Network timeout | 30s for Tally, 15s for SATVOS. Retry on timeout |
| Partial master sync | Idempotent UPSERT. Next cycle completes |

## Testing Strategy

### Connector Tests

- `tally/client_test.go` — XML request building, response parsing (httptest mock)
- `tally/discover_test.go` — Port scanning logic
- `cloud/client_test.go` — SATVOS API client with httptest
- `sync/engine_test.go` — Sync loop orchestration
- `convert/xml_test.go` — VoucherDef to Tally XML conversion
- `config/config_test.go` — Config loading, defaults, validation
- Integration test with mock Tally + mock SATVOS servers

### Server-Side Tests

- `handler/sync_handler_test.go` — Sync API endpoint tests
- `service/voucher_builder_test.go` — Smart matching logic (critical)
- `service/sync_service_test.go` — Sync service logic

## Installation UX

1. Download `satvos-connector.exe` from SATVOS dashboard
2. Run installer — registers as Windows Service
3. Opens `http://localhost:8321/setup` in browser
4. Paste API key → Verify → Auto-discover Tally → Done
5. Status dashboard at `http://localhost:8321/` shows sync status

## Future Extensions

- Dashboard mapping preview UI (show matched entities before sync)
- User-defined mappings for unmatched entities
- Auto-creation of missing Tally ledgers and stock items
- Multi-company support (one agent, multiple Tally companies)
- System tray icon for desktop mode
- Auto-update mechanism
