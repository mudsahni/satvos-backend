# Tally Connector Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a bidirectional sync system between SATVOS Cloud and customer Tally Prime instances via an on-premise agent, enabling smart voucher import using real Tally masters.

**Architecture:** Two independently deployable components — (1) new Sync API endpoints on the existing SATVOS server that store Tally masters, do smart matching, and build voucher definitions; (2) a new Go binary (`satvos-tally-connector`) that runs on the customer's Windows machine, polls SATVOS via HTTPS, reads/writes Tally via XML on localhost:9000.

**Tech Stack:** Go 1.24, PostgreSQL 16, sqlx, Gin, Viper, text/template (Tally XML), embed.FS (local UI), golang.org/x/sys/windows/svc (Windows Service)

**Design Doc:** `docs/plans/2026-02-28-tally-connector-design.md`

---

## Part 1 — SATVOS Server-Side Changes

All server changes live in the existing `satvos` repo. Follow existing patterns exactly.

---

### Task 1: Database Migration

**Files:**
- Create: `db/migrations/000027_create_tally_connector_tables.up.sql`
- Create: `db/migrations/000027_create_tally_connector_tables.down.sql`

**Step 1: Write the UP migration**

Create `db/migrations/000027_create_tally_connector_tables.up.sql`:

```sql
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
```

**Step 2: Write the DOWN migration**

Create `db/migrations/000027_create_tally_connector_tables.down.sql`:

```sql
DROP TABLE IF EXISTS tally_vouchers;
DROP TABLE IF EXISTS sync_events;
DROP TABLE IF EXISTS tally_cost_centres;
DROP TABLE IF EXISTS tally_units;
DROP TABLE IF EXISTS tally_godowns;
DROP TABLE IF EXISTS tally_stock_items;
DROP TABLE IF EXISTS tally_ledgers;
DROP TABLE IF EXISTS connector_agents;
```

**Step 3: Verify migration applies cleanly**

Run: `make migrate-up`
Expected: Migration 000027 applied successfully.

Run: `make migrate-down`
Expected: Migration rolled back, tables dropped.

Run: `make migrate-up` again to leave DB in migrated state.

**Step 4: Commit**

```bash
git add db/migrations/000027_create_tally_connector_tables.up.sql db/migrations/000027_create_tally_connector_tables.down.sql
git commit -m "feat: add migration 000027 for tally connector tables

Creates connector_agents, tally_ledgers, tally_stock_items,
tally_godowns, tally_units, tally_cost_centres, sync_events,
and tally_vouchers tables for the Tally connector sync system."
```

---

### Task 2: Domain Models and Enums

**Files:**
- Modify: `internal/domain/models.go` — add connector domain structs
- Modify: `internal/domain/enums.go` — add sync-related enums
- Modify: `internal/domain/errors.go` — add sync-related sentinel errors

**Step 1: Add domain models**

Add to `internal/domain/models.go` (at the end, before closing):

```go
// ConnectorAgent represents a registered on-premise Tally connector agent.
type ConnectorAgent struct {
	ID               uuid.UUID  `db:"id" json:"id"`
	TenantID         uuid.UUID  `db:"tenant_id" json:"tenant_id"`
	ServiceAccountID uuid.UUID  `db:"service_account_id" json:"service_account_id"`
	AgentVersion     string     `db:"agent_version" json:"agent_version"`
	TallyCompany     string     `db:"tally_company" json:"tally_company"`
	TallyPort        int        `db:"tally_port" json:"tally_port"`
	OSInfo           string     `db:"os_info" json:"os_info"`
	Status           AgentStatus `db:"status" json:"status"`
	LastHeartbeat    *time.Time `db:"last_heartbeat" json:"last_heartbeat"`
	RegisteredAt     time.Time  `db:"registered_at" json:"registered_at"`
}

// TallyLedger represents a ledger master from a customer's Tally instance.
type TallyLedger struct {
	ID          uuid.UUID `db:"id" json:"id"`
	TenantID    uuid.UUID `db:"tenant_id" json:"tenant_id"`
	Name        string    `db:"name" json:"name"`
	ParentGroup string    `db:"parent_group" json:"parent_group"`
	GSTIN       string    `db:"gstin" json:"gstin"`
	State       string    `db:"state" json:"state"`
	TaxType     string    `db:"tax_type" json:"tax_type"`
	TaxRate     float64   `db:"tax_rate" json:"tax_rate"`
	IsRevenue   bool      `db:"is_revenue" json:"is_revenue"`
	SyncedAt    time.Time `db:"synced_at" json:"synced_at"`
}

// TallyStockItem represents a stock item master from a customer's Tally instance.
type TallyStockItem struct {
	ID          uuid.UUID `db:"id" json:"id"`
	TenantID    uuid.UUID `db:"tenant_id" json:"tenant_id"`
	Name        string    `db:"name" json:"name"`
	ParentGroup string    `db:"parent_group" json:"parent_group"`
	HSNCode     string    `db:"hsn_code" json:"hsn_code"`
	DefaultUOM  string    `db:"default_uom" json:"default_uom"`
	SyncedAt    time.Time `db:"synced_at" json:"synced_at"`
}

// TallyGodown represents a godown master from a customer's Tally instance.
type TallyGodown struct {
	ID       uuid.UUID `db:"id" json:"id"`
	TenantID uuid.UUID `db:"tenant_id" json:"tenant_id"`
	Name     string    `db:"name" json:"name"`
	Parent   string    `db:"parent" json:"parent"`
	SyncedAt time.Time `db:"synced_at" json:"synced_at"`
}

// TallyUnit represents a unit of measure from a customer's Tally instance.
type TallyUnit struct {
	ID         uuid.UUID `db:"id" json:"id"`
	TenantID   uuid.UUID `db:"tenant_id" json:"tenant_id"`
	Symbol     string    `db:"symbol" json:"symbol"`
	FormalName string    `db:"formal_name" json:"formal_name"`
	SyncedAt   time.Time `db:"synced_at" json:"synced_at"`
}

// TallyCostCentre represents a cost centre from a customer's Tally instance.
type TallyCostCentre struct {
	ID       uuid.UUID `db:"id" json:"id"`
	TenantID uuid.UUID `db:"tenant_id" json:"tenant_id"`
	Name     string    `db:"name" json:"name"`
	Parent   string    `db:"parent" json:"parent"`
	SyncedAt time.Time `db:"synced_at" json:"synced_at"`
}

// SyncEvent records an individual sync operation.
type SyncEvent struct {
	ID                 uuid.UUID  `db:"id" json:"id"`
	TenantID           uuid.UUID  `db:"tenant_id" json:"tenant_id"`
	AgentID            uuid.UUID  `db:"agent_id" json:"agent_id"`
	DocumentID         *uuid.UUID `db:"document_id" json:"document_id,omitempty"`
	Direction          SyncDirection `db:"direction" json:"direction"`
	Status             SyncStatus    `db:"status" json:"status"`
	TallyVoucherID     string     `db:"tally_voucher_id" json:"tally_voucher_id,omitempty"`
	TallyVoucherNumber string     `db:"tally_voucher_number" json:"tally_voucher_number,omitempty"`
	ErrorMessage       string     `db:"error_message" json:"error_message,omitempty"`
	CreatedAt          time.Time  `db:"created_at" json:"created_at"`
}

// TallyVoucher represents a voucher read from a customer's Tally for reconciliation.
type TallyVoucher struct {
	ID             uuid.UUID       `db:"id" json:"id"`
	TenantID       uuid.UUID       `db:"tenant_id" json:"tenant_id"`
	VoucherType    string          `db:"voucher_type" json:"voucher_type"`
	VoucherNumber  string          `db:"voucher_number" json:"voucher_number"`
	VoucherDate    *time.Time      `db:"voucher_date" json:"voucher_date"`
	PartyName      string          `db:"party_name" json:"party_name"`
	PartyGSTIN     string          `db:"party_gstin" json:"party_gstin"`
	Amount         float64         `db:"amount" json:"amount"`
	Narration      string          `db:"narration" json:"narration"`
	LedgerEntries  json.RawMessage `db:"ledger_entries" json:"ledger_entries"`
	RemoteID       string          `db:"remote_id" json:"remote_id"`
	TallyMasterID  string          `db:"tally_master_id" json:"tally_master_id"`
	SyncedAt       time.Time       `db:"synced_at" json:"synced_at"`
}

// VoucherDef is the smart-matched voucher definition sent to the agent for Tally import.
type VoucherDef struct {
	DocumentID      uuid.UUID              `json:"document_id"`
	VoucherType     string                 `json:"voucher_type"`
	VoucherDate     string                 `json:"voucher_date"`
	PartyLedger     string                 `json:"party_ledger"`
	PurchaseLedger  string                 `json:"purchase_ledger"`
	TaxEntries      []VoucherDefTaxEntry   `json:"tax_entries"`
	InventoryItems  []VoucherDefItem       `json:"inventory_items"`
	TotalAmount     float64                `json:"total_amount"`
	Narration       string                 `json:"narration"`
	RemoteID        string                 `json:"remote_id"`
	MatchConfidence map[string]string      `json:"match_confidence"`
}

// VoucherDefTaxEntry is a tax ledger entry within a VoucherDef.
type VoucherDefTaxEntry struct {
	LedgerName string  `json:"ledger_name"`
	Amount     float64 `json:"amount"`
}

// VoucherDefItem is an inventory item within a VoucherDef.
type VoucherDefItem struct {
	StockItem string  `json:"stock_item"`
	Quantity  float64 `json:"quantity"`
	Rate      float64 `json:"rate"`
	Amount    float64 `json:"amount"`
	UOM       string  `json:"uom"`
	Godown    string  `json:"godown"`
	HSNCode   string  `json:"hsn_code"`
}
```

**Step 2: Add enums**

Add to `internal/domain/enums.go`:

```go
// AgentStatus represents the lifecycle status of a connector agent.
type AgentStatus string

const (
	AgentStatusRegistered   AgentStatus = "registered"
	AgentStatusOnline       AgentStatus = "online"
	AgentStatusOffline      AgentStatus = "offline"
	AgentStatusDisconnected AgentStatus = "disconnected"
)

var ValidAgentStatuses = map[AgentStatus]bool{
	AgentStatusRegistered:   true,
	AgentStatusOnline:       true,
	AgentStatusOffline:      true,
	AgentStatusDisconnected: true,
}

// SyncDirection indicates whether a sync event is agent→cloud or cloud→agent.
type SyncDirection string

const (
	SyncDirectionInbound  SyncDirection = "inbound"
	SyncDirectionOutbound SyncDirection = "outbound"
)

// SyncStatus represents the outcome of a sync event.
type SyncStatus string

const (
	SyncStatusPending   SyncStatus = "pending"
	SyncStatusSuccess   SyncStatus = "success"
	SyncStatusFailed    SyncStatus = "failed"
	SyncStatusSkipped   SyncStatus = "skipped"
)

var ValidSyncStatuses = map[SyncStatus]bool{
	SyncStatusPending: true,
	SyncStatusSuccess: true,
	SyncStatusFailed:  true,
	SyncStatusSkipped: true,
}
```

**Step 3: Add sentinel errors**

Add to `internal/domain/errors.go`:

```go
var (
	ErrAgentNotFound      = errors.New("connector agent not found")
	ErrAgentAlreadyExists = errors.New("connector agent already registered for this service account")
)
```

**Step 4: Run lint**

Run: `make lint`
Expected: PASS, no lint errors.

**Step 5: Commit**

```bash
git add internal/domain/models.go internal/domain/enums.go internal/domain/errors.go
git commit -m "feat: add domain models and enums for tally connector

Adds ConnectorAgent, TallyLedger, TallyStockItem, TallyGodown,
TallyUnit, TallyCostCentre, SyncEvent, TallyVoucher, VoucherDef
domain types plus AgentStatus, SyncDirection, SyncStatus enums."
```

---

### Task 3: Port Interfaces (Repository Contracts)

**Files:**
- Create: `internal/port/tally_master_repository.go`
- Create: `internal/port/sync_repository.go`
- Create: `internal/port/tally_voucher_repository.go`

**Step 1: Create tally master repository port**

Create `internal/port/tally_master_repository.go`:

```go
package port

import (
	"context"

	"github.com/google/uuid"

	"satvos/internal/domain"
)

// TallyMasterRepository handles persistence of Tally master data (ledgers, stock items, etc.).
type TallyMasterRepository interface {
	// Ledgers
	UpsertLedgers(ctx context.Context, tenantID uuid.UUID, ledgers []domain.TallyLedger) error
	ListLedgers(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyLedger, error)
	FindLedgerByGSTIN(ctx context.Context, tenantID uuid.UUID, gstin string) (*domain.TallyLedger, error)
	FindTaxLedger(ctx context.Context, tenantID uuid.UUID, taxType string, taxRate float64) (*domain.TallyLedger, error)
	FindPurchaseLedger(ctx context.Context, tenantID uuid.UUID) (*domain.TallyLedger, error)

	// Stock items
	UpsertStockItems(ctx context.Context, tenantID uuid.UUID, items []domain.TallyStockItem) error
	ListStockItems(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyStockItem, error)
	FindStockItemByHSN(ctx context.Context, tenantID uuid.UUID, hsnCode string) (*domain.TallyStockItem, error)

	// Godowns
	UpsertGodowns(ctx context.Context, tenantID uuid.UUID, godowns []domain.TallyGodown) error
	ListGodowns(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyGodown, error)
	GetDefaultGodown(ctx context.Context, tenantID uuid.UUID) (*domain.TallyGodown, error)

	// Units
	UpsertUnits(ctx context.Context, tenantID uuid.UUID, units []domain.TallyUnit) error
	ListUnits(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyUnit, error)
	FindUnitBySymbol(ctx context.Context, tenantID uuid.UUID, symbol string) (*domain.TallyUnit, error)

	// Cost centres
	UpsertCostCentres(ctx context.Context, tenantID uuid.UUID, centres []domain.TallyCostCentre) error
	ListCostCentres(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyCostCentre, error)
}
```

**Step 2: Create sync repository port**

Create `internal/port/sync_repository.go`:

```go
package port

import (
	"context"

	"github.com/google/uuid"

	"satvos/internal/domain"
)

// SyncRepository manages connector agents and sync events.
type SyncRepository interface {
	// Agent lifecycle
	RegisterAgent(ctx context.Context, agent *domain.ConnectorAgent) error
	GetAgentByServiceAccount(ctx context.Context, tenantID, serviceAccountID uuid.UUID) (*domain.ConnectorAgent, error)
	GetAgentByID(ctx context.Context, tenantID, agentID uuid.UUID) (*domain.ConnectorAgent, error)
	UpdateHeartbeat(ctx context.Context, agentID uuid.UUID, status domain.AgentStatus, tallyCompany string, tallyPort int, version string) error

	// Sync events
	CreateSyncEvent(ctx context.Context, event *domain.SyncEvent) error
	UpdateSyncEventStatus(ctx context.Context, eventID uuid.UUID, status domain.SyncStatus, tallyVoucherID, tallyVoucherNumber, errorMessage string) error
	ListSyncEvents(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.SyncEvent, int, error)

	// Outbound queue — documents approved for export that haven't been synced yet
	ListOutboundDocuments(ctx context.Context, tenantID uuid.UUID, cursor string, limit int) ([]domain.Document, string, error)
}
```

**Step 3: Create tally voucher repository port**

Create `internal/port/tally_voucher_repository.go`:

```go
package port

import (
	"context"

	"github.com/google/uuid"

	"satvos/internal/domain"
)

// TallyVoucherRepository handles persistence of inbound Tally vouchers.
type TallyVoucherRepository interface {
	UpsertVouchers(ctx context.Context, tenantID uuid.UUID, vouchers []domain.TallyVoucher) error
	ListByTenant(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.TallyVoucher, int, error)
	GetByID(ctx context.Context, tenantID, voucherID uuid.UUID) (*domain.TallyVoucher, error)
}
```

**Step 4: Run lint**

Run: `make lint`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/port/tally_master_repository.go internal/port/sync_repository.go internal/port/tally_voucher_repository.go
git commit -m "feat: add port interfaces for tally connector repositories

Defines TallyMasterRepository, SyncRepository, and
TallyVoucherRepository interfaces for the sync system."
```

---

### Task 4: Postgres Repository — Tally Masters

**Files:**
- Create: `internal/repository/postgres/tally_master_repo.go`
- Create: `tests/unit/repository/tally_master_repo_test.go`

**Step 1: Write the tally master repo tests**

Create `tests/unit/repository/tally_master_repo_test.go` — test UpsertLedgers (insert + update), FindLedgerByGSTIN (found + not found), FindTaxLedger, UpsertStockItems, FindStockItemByHSN. Use `sqlmock` or test against real DB if integration tests exist.

Since the codebase uses hand-written mocks for unit tests and real DB for integration, write mock-based tests for the service layer (Task 7) and keep repo tests minimal — verify SQL is valid via `make test` with a test DB.

**Step 2: Implement tally master repo**

Create `internal/repository/postgres/tally_master_repo.go`:

```go
package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"satvos/internal/domain"
)

type tallyMasterRepo struct {
	db *sqlx.DB
}

func NewTallyMasterRepo(db *sqlx.DB) *tallyMasterRepo {
	return &tallyMasterRepo{db: db}
}

func (r *tallyMasterRepo) UpsertLedgers(ctx context.Context, tenantID uuid.UUID, ledgers []domain.TallyLedger) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for i := range ledgers {
		l := &ledgers[i]
		l.TenantID = tenantID
		l.SyncedAt = time.Now().UTC()
		if l.ID == uuid.Nil {
			l.ID = uuid.New()
		}
		_, err := tx.NamedExecContext(ctx, `
			INSERT INTO tally_ledgers (id, tenant_id, name, parent_group, gstin, state, tax_type, tax_rate, is_revenue, synced_at)
			VALUES (:id, :tenant_id, :name, :parent_group, :gstin, :state, :tax_type, :tax_rate, :is_revenue, :synced_at)
			ON CONFLICT (tenant_id, name) DO UPDATE SET
				parent_group = EXCLUDED.parent_group,
				gstin = EXCLUDED.gstin,
				state = EXCLUDED.state,
				tax_type = EXCLUDED.tax_type,
				tax_rate = EXCLUDED.tax_rate,
				is_revenue = EXCLUDED.is_revenue,
				synced_at = EXCLUDED.synced_at`, l)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *tallyMasterRepo) ListLedgers(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyLedger, error) {
	var ledgers []domain.TallyLedger
	err := r.db.SelectContext(ctx, &ledgers,
		`SELECT * FROM tally_ledgers WHERE tenant_id = $1 ORDER BY name`, tenantID)
	return ledgers, err
}

func (r *tallyMasterRepo) FindLedgerByGSTIN(ctx context.Context, tenantID uuid.UUID, gstin string) (*domain.TallyLedger, error) {
	var l domain.TallyLedger
	err := r.db.GetContext(ctx, &l,
		`SELECT * FROM tally_ledgers WHERE tenant_id = $1 AND gstin = $2 LIMIT 1`, tenantID, gstin)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &l, err
}

func (r *tallyMasterRepo) FindTaxLedger(ctx context.Context, tenantID uuid.UUID, taxType string, taxRate float64) (*domain.TallyLedger, error) {
	var l domain.TallyLedger
	err := r.db.GetContext(ctx, &l,
		`SELECT * FROM tally_ledgers WHERE tenant_id = $1 AND tax_type = $2 AND tax_rate = $3 LIMIT 1`,
		tenantID, taxType, taxRate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &l, err
}

func (r *tallyMasterRepo) FindPurchaseLedger(ctx context.Context, tenantID uuid.UUID) (*domain.TallyLedger, error) {
	var l domain.TallyLedger
	err := r.db.GetContext(ctx, &l,
		`SELECT * FROM tally_ledgers WHERE tenant_id = $1 AND parent_group = 'Purchase Accounts' LIMIT 1`,
		tenantID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &l, err
}

func (r *tallyMasterRepo) UpsertStockItems(ctx context.Context, tenantID uuid.UUID, items []domain.TallyStockItem) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for i := range items {
		item := &items[i]
		item.TenantID = tenantID
		item.SyncedAt = time.Now().UTC()
		if item.ID == uuid.Nil {
			item.ID = uuid.New()
		}
		_, err := tx.NamedExecContext(ctx, `
			INSERT INTO tally_stock_items (id, tenant_id, name, parent_group, hsn_code, default_uom, synced_at)
			VALUES (:id, :tenant_id, :name, :parent_group, :hsn_code, :default_uom, :synced_at)
			ON CONFLICT (tenant_id, name) DO UPDATE SET
				parent_group = EXCLUDED.parent_group,
				hsn_code = EXCLUDED.hsn_code,
				default_uom = EXCLUDED.default_uom,
				synced_at = EXCLUDED.synced_at`, item)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *tallyMasterRepo) ListStockItems(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyStockItem, error) {
	var items []domain.TallyStockItem
	err := r.db.SelectContext(ctx, &items,
		`SELECT * FROM tally_stock_items WHERE tenant_id = $1 ORDER BY name`, tenantID)
	return items, err
}

func (r *tallyMasterRepo) FindStockItemByHSN(ctx context.Context, tenantID uuid.UUID, hsnCode string) (*domain.TallyStockItem, error) {
	var item domain.TallyStockItem
	err := r.db.GetContext(ctx, &item,
		`SELECT * FROM tally_stock_items WHERE tenant_id = $1 AND hsn_code = $2 LIMIT 1`, tenantID, hsnCode)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &item, err
}

func (r *tallyMasterRepo) UpsertGodowns(ctx context.Context, tenantID uuid.UUID, godowns []domain.TallyGodown) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for i := range godowns {
		g := &godowns[i]
		g.TenantID = tenantID
		g.SyncedAt = time.Now().UTC()
		if g.ID == uuid.Nil {
			g.ID = uuid.New()
		}
		_, err := tx.NamedExecContext(ctx, `
			INSERT INTO tally_godowns (id, tenant_id, name, parent, synced_at)
			VALUES (:id, :tenant_id, :name, :parent, :synced_at)
			ON CONFLICT (tenant_id, name) DO UPDATE SET
				parent = EXCLUDED.parent,
				synced_at = EXCLUDED.synced_at`, g)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *tallyMasterRepo) ListGodowns(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyGodown, error) {
	var godowns []domain.TallyGodown
	err := r.db.SelectContext(ctx, &godowns,
		`SELECT * FROM tally_godowns WHERE tenant_id = $1 ORDER BY name`, tenantID)
	return godowns, err
}

func (r *tallyMasterRepo) GetDefaultGodown(ctx context.Context, tenantID uuid.UUID) (*domain.TallyGodown, error) {
	var g domain.TallyGodown
	err := r.db.GetContext(ctx, &g,
		`SELECT * FROM tally_godowns WHERE tenant_id = $1 ORDER BY name LIMIT 1`, tenantID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &g, err
}

func (r *tallyMasterRepo) UpsertUnits(ctx context.Context, tenantID uuid.UUID, units []domain.TallyUnit) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for i := range units {
		u := &units[i]
		u.TenantID = tenantID
		u.SyncedAt = time.Now().UTC()
		if u.ID == uuid.Nil {
			u.ID = uuid.New()
		}
		_, err := tx.NamedExecContext(ctx, `
			INSERT INTO tally_units (id, tenant_id, symbol, formal_name, synced_at)
			VALUES (:id, :tenant_id, :symbol, :formal_name, :synced_at)
			ON CONFLICT (tenant_id, symbol) DO UPDATE SET
				formal_name = EXCLUDED.formal_name,
				synced_at = EXCLUDED.synced_at`, u)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *tallyMasterRepo) ListUnits(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyUnit, error) {
	var units []domain.TallyUnit
	err := r.db.SelectContext(ctx, &units,
		`SELECT * FROM tally_units WHERE tenant_id = $1 ORDER BY symbol`, tenantID)
	return units, err
}

func (r *tallyMasterRepo) FindUnitBySymbol(ctx context.Context, tenantID uuid.UUID, symbol string) (*domain.TallyUnit, error) {
	var u domain.TallyUnit
	err := r.db.GetContext(ctx, &u,
		`SELECT * FROM tally_units WHERE tenant_id = $1 AND symbol = $2 LIMIT 1`, tenantID, symbol)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (r *tallyMasterRepo) UpsertCostCentres(ctx context.Context, tenantID uuid.UUID, centres []domain.TallyCostCentre) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for i := range centres {
		cc := &centres[i]
		cc.TenantID = tenantID
		cc.SyncedAt = time.Now().UTC()
		if cc.ID == uuid.Nil {
			cc.ID = uuid.New()
		}
		_, err := tx.NamedExecContext(ctx, `
			INSERT INTO tally_cost_centres (id, tenant_id, name, parent, synced_at)
			VALUES (:id, :tenant_id, :name, :parent, :synced_at)
			ON CONFLICT (tenant_id, name) DO UPDATE SET
				parent = EXCLUDED.parent,
				synced_at = EXCLUDED.synced_at`, cc)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *tallyMasterRepo) ListCostCentres(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyCostCentre, error) {
	var centres []domain.TallyCostCentre
	err := r.db.SelectContext(ctx, &centres,
		`SELECT * FROM tally_cost_centres WHERE tenant_id = $1 ORDER BY name`, tenantID)
	return centres, err
}
```

**Step 3: Run lint and tests**

Run: `make lint`
Expected: PASS.

**Step 4: Commit**

```bash
git add internal/repository/postgres/tally_master_repo.go
git commit -m "feat: implement TallyMasterRepository postgres adapter

UPSERT-based persistence for ledgers, stock items, godowns,
units, and cost centres with tenant isolation."
```

---

### Task 5: Postgres Repository — Sync and Tally Vouchers

**Files:**
- Create: `internal/repository/postgres/sync_repo.go`
- Create: `internal/repository/postgres/tally_voucher_repo.go`

**Step 1: Implement sync repo**

Create `internal/repository/postgres/sync_repo.go`:

```go
package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"satvos/internal/domain"
)

type syncRepo struct {
	db *sqlx.DB
}

func NewSyncRepo(db *sqlx.DB) *syncRepo {
	return &syncRepo{db: db}
}

func (r *syncRepo) RegisterAgent(ctx context.Context, agent *domain.ConnectorAgent) error {
	if agent.ID == uuid.Nil {
		agent.ID = uuid.New()
	}
	agent.RegisteredAt = time.Now().UTC()
	agent.Status = domain.AgentStatusRegistered
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO connector_agents (id, tenant_id, service_account_id, agent_version, tally_company, tally_port, os_info, status, registered_at)
		VALUES (:id, :tenant_id, :service_account_id, :agent_version, :tally_company, :tally_port, :os_info, :status, :registered_at)`, agent)
	return err
}

func (r *syncRepo) GetAgentByServiceAccount(ctx context.Context, tenantID, serviceAccountID uuid.UUID) (*domain.ConnectorAgent, error) {
	var agent domain.ConnectorAgent
	err := r.db.GetContext(ctx, &agent,
		`SELECT * FROM connector_agents WHERE tenant_id = $1 AND service_account_id = $2`, tenantID, serviceAccountID)
	if err == sql.ErrNoRows {
		return nil, domain.ErrAgentNotFound
	}
	return &agent, err
}

func (r *syncRepo) GetAgentByID(ctx context.Context, tenantID, agentID uuid.UUID) (*domain.ConnectorAgent, error) {
	var agent domain.ConnectorAgent
	err := r.db.GetContext(ctx, &agent,
		`SELECT * FROM connector_agents WHERE tenant_id = $1 AND id = $2`, tenantID, agentID)
	if err == sql.ErrNoRows {
		return nil, domain.ErrAgentNotFound
	}
	return &agent, err
}

func (r *syncRepo) UpdateHeartbeat(ctx context.Context, agentID uuid.UUID, status domain.AgentStatus, tallyCompany string, tallyPort int, version string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE connector_agents SET status = $1, last_heartbeat = $2, tally_company = $3, tally_port = $4, agent_version = $5 WHERE id = $6`,
		status, time.Now().UTC(), tallyCompany, tallyPort, version, agentID)
	return err
}

func (r *syncRepo) CreateSyncEvent(ctx context.Context, event *domain.SyncEvent) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	event.CreatedAt = time.Now().UTC()
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO sync_events (id, tenant_id, agent_id, document_id, direction, status, tally_voucher_id, tally_voucher_number, error_message, created_at)
		VALUES (:id, :tenant_id, :agent_id, :document_id, :direction, :status, :tally_voucher_id, :tally_voucher_number, :error_message, :created_at)`, event)
	return err
}

func (r *syncRepo) UpdateSyncEventStatus(ctx context.Context, eventID uuid.UUID, status domain.SyncStatus, tallyVoucherID, tallyVoucherNumber, errorMessage string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE sync_events SET status = $1, tally_voucher_id = $2, tally_voucher_number = $3, error_message = $4 WHERE id = $5`,
		status, tallyVoucherID, tallyVoucherNumber, errorMessage, eventID)
	return err
}

func (r *syncRepo) ListSyncEvents(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.SyncEvent, int, error) {
	var total int
	if err := r.db.GetContext(ctx, &total,
		`SELECT COUNT(*) FROM sync_events WHERE tenant_id = $1`, tenantID); err != nil {
		return nil, 0, err
	}
	var events []domain.SyncEvent
	err := r.db.SelectContext(ctx, &events,
		`SELECT * FROM sync_events WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		tenantID, limit, offset)
	return events, total, err
}

func (r *syncRepo) ListOutboundDocuments(ctx context.Context, tenantID uuid.UUID, cursor string, limit int) ([]domain.Document, string, error) {
	var docs []domain.Document
	var err error
	if cursor == "" {
		err = r.db.SelectContext(ctx, &docs, `
			SELECT d.* FROM documents d
			WHERE d.tenant_id = $1
			  AND d.parsing_status = 'completed'
			  AND d.review_status = 'approved'
			  AND NOT EXISTS (
				SELECT 1 FROM sync_events se
				WHERE se.document_id = d.id AND se.direction = 'outbound' AND se.status = 'success'
			  )
			ORDER BY d.created_at ASC
			LIMIT $2`, tenantID, limit)
	} else {
		cursorID, parseErr := uuid.Parse(cursor)
		if parseErr != nil {
			return nil, "", parseErr
		}
		err = r.db.SelectContext(ctx, &docs, `
			SELECT d.* FROM documents d
			WHERE d.tenant_id = $1
			  AND d.parsing_status = 'completed'
			  AND d.review_status = 'approved'
			  AND d.id > $2
			  AND NOT EXISTS (
				SELECT 1 FROM sync_events se
				WHERE se.document_id = d.id AND se.direction = 'outbound' AND se.status = 'success'
			  )
			ORDER BY d.created_at ASC
			LIMIT $3`, tenantID, cursorID, limit)
	}
	if err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if len(docs) > 0 {
		nextCursor = docs[len(docs)-1].ID.String()
	}
	return docs, nextCursor, nil
}
```

**Step 2: Implement tally voucher repo**

Create `internal/repository/postgres/tally_voucher_repo.go`:

```go
package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"satvos/internal/domain"
)

type tallyVoucherRepo struct {
	db *sqlx.DB
}

func NewTallyVoucherRepo(db *sqlx.DB) *tallyVoucherRepo {
	return &tallyVoucherRepo{db: db}
}

func (r *tallyVoucherRepo) UpsertVouchers(ctx context.Context, tenantID uuid.UUID, vouchers []domain.TallyVoucher) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for i := range vouchers {
		v := &vouchers[i]
		v.TenantID = tenantID
		v.SyncedAt = time.Now().UTC()
		if v.ID == uuid.Nil {
			v.ID = uuid.New()
		}
		_, err := tx.NamedExecContext(ctx, `
			INSERT INTO tally_vouchers (id, tenant_id, voucher_type, voucher_number, voucher_date, party_name, party_gstin, amount, narration, ledger_entries, remote_id, tally_master_id, synced_at)
			VALUES (:id, :tenant_id, :voucher_type, :voucher_number, :voucher_date, :party_name, :party_gstin, :amount, :narration, :ledger_entries, :remote_id, :tally_master_id, :synced_at)
			ON CONFLICT (tenant_id, tally_master_id) DO UPDATE SET
				voucher_type = EXCLUDED.voucher_type,
				voucher_number = EXCLUDED.voucher_number,
				voucher_date = EXCLUDED.voucher_date,
				party_name = EXCLUDED.party_name,
				party_gstin = EXCLUDED.party_gstin,
				amount = EXCLUDED.amount,
				narration = EXCLUDED.narration,
				ledger_entries = EXCLUDED.ledger_entries,
				remote_id = EXCLUDED.remote_id,
				synced_at = EXCLUDED.synced_at`, v)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *tallyVoucherRepo) ListByTenant(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.TallyVoucher, int, error) {
	var total int
	if err := r.db.GetContext(ctx, &total,
		`SELECT COUNT(*) FROM tally_vouchers WHERE tenant_id = $1`, tenantID); err != nil {
		return nil, 0, err
	}
	var vouchers []domain.TallyVoucher
	err := r.db.SelectContext(ctx, &vouchers,
		`SELECT * FROM tally_vouchers WHERE tenant_id = $1 ORDER BY synced_at DESC LIMIT $2 OFFSET $3`,
		tenantID, limit, offset)
	return vouchers, total, err
}

func (r *tallyVoucherRepo) GetByID(ctx context.Context, tenantID, voucherID uuid.UUID) (*domain.TallyVoucher, error) {
	var v domain.TallyVoucher
	err := r.db.GetContext(ctx, &v,
		`SELECT * FROM tally_vouchers WHERE tenant_id = $1 AND id = $2`, tenantID, voucherID)
	if err == sql.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	return &v, err
}
```

**Step 3: Run lint**

Run: `make lint`
Expected: PASS.

**Step 4: Commit**

```bash
git add internal/repository/postgres/sync_repo.go internal/repository/postgres/tally_voucher_repo.go
git commit -m "feat: implement SyncRepository and TallyVoucherRepository postgres adapters

Agent registration, heartbeat tracking, sync event logging,
outbound document queue, and inbound voucher persistence."
```

---

### Task 6: Sync Service

**Files:**
- Create: `internal/service/sync_service.go`
- Create: `mocks/mock_sync_service.go`

**Step 1: Define the sync service interface and implementation**

Create `internal/service/sync_service.go`:

```go
package service

import (
	"context"
	"encoding/json"
	"log"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/port"
)

// SyncService handles connector agent lifecycle and sync operations.
type SyncService interface {
	Register(ctx context.Context, tenantID, serviceAccountID uuid.UUID, version, osInfo string) (*domain.ConnectorAgent, error)
	Heartbeat(ctx context.Context, tenantID, serviceAccountID uuid.UUID, tallyConnected bool, tallyCompany string, tallyPort int, version string, agentErrors []string) error
	SaveMasters(ctx context.Context, tenantID uuid.UUID, masters MasterPayload) error
	ListOutbound(ctx context.Context, tenantID uuid.UUID, cursor string, limit int) ([]OutboundItem, string, error)
	AckOutbound(ctx context.Context, tenantID, serviceAccountID uuid.UUID, results []AckResult) error
	SaveInbound(ctx context.Context, tenantID uuid.UUID, vouchers []domain.TallyVoucher) error
	GetAgentStatus(ctx context.Context, tenantID uuid.UUID) (*domain.ConnectorAgent, error)
}

// MasterPayload contains all Tally master data types in a single upload.
type MasterPayload struct {
	Ledgers     []domain.TallyLedger     `json:"ledgers"`
	StockItems  []domain.TallyStockItem  `json:"stock_items"`
	Godowns     []domain.TallyGodown     `json:"godowns"`
	Units       []domain.TallyUnit       `json:"units"`
	CostCentres []domain.TallyCostCentre `json:"cost_centres"`
}

// OutboundItem pairs a document with its smart-matched voucher definition.
type OutboundItem struct {
	DocumentID     uuid.UUID           `json:"document_id"`
	StructuredData json.RawMessage     `json:"structured_data"`
	VoucherDef     *domain.VoucherDef  `json:"voucher_def"`
	SyncEventID    uuid.UUID           `json:"sync_event_id"`
}

// AckResult reports the outcome of importing one document into Tally.
type AckResult struct {
	SyncEventID        uuid.UUID `json:"sync_event_id"`
	DocumentID         uuid.UUID `json:"document_id"`
	Success            bool      `json:"success"`
	TallyVoucherID     string    `json:"tally_voucher_id,omitempty"`
	TallyVoucherNumber string    `json:"tally_voucher_number,omitempty"`
	ErrorMessage       string    `json:"error_message,omitempty"`
}

type syncService struct {
	syncRepo       port.SyncRepository
	masterRepo     port.TallyMasterRepository
	voucherRepo    port.TallyVoucherRepository
	voucherBuilder VoucherBuilderService
}

func NewSyncService(
	syncRepo port.SyncRepository,
	masterRepo port.TallyMasterRepository,
	voucherRepo port.TallyVoucherRepository,
	voucherBuilder VoucherBuilderService,
) SyncService {
	return &syncService{
		syncRepo:       syncRepo,
		masterRepo:     masterRepo,
		voucherRepo:    voucherRepo,
		voucherBuilder: voucherBuilder,
	}
}

func (s *syncService) Register(ctx context.Context, tenantID, serviceAccountID uuid.UUID, version, osInfo string) (*domain.ConnectorAgent, error) {
	existing, err := s.syncRepo.GetAgentByServiceAccount(ctx, tenantID, serviceAccountID)
	if err == nil && existing != nil {
		return existing, nil
	}

	agent := &domain.ConnectorAgent{
		TenantID:         tenantID,
		ServiceAccountID: serviceAccountID,
		AgentVersion:     version,
		OSInfo:           osInfo,
	}
	if err := s.syncRepo.RegisterAgent(ctx, agent); err != nil {
		return nil, err
	}
	return agent, nil
}

func (s *syncService) Heartbeat(ctx context.Context, tenantID, serviceAccountID uuid.UUID, tallyConnected bool, tallyCompany string, tallyPort int, version string, agentErrors []string) error {
	agent, err := s.syncRepo.GetAgentByServiceAccount(ctx, tenantID, serviceAccountID)
	if err != nil {
		return err
	}

	status := domain.AgentStatusOnline
	if !tallyConnected {
		status = domain.AgentStatusOffline
	}

	if len(agentErrors) > 0 {
		log.Printf("[sync] agent %s reported errors: %v", agent.ID, agentErrors)
	}

	return s.syncRepo.UpdateHeartbeat(ctx, agent.ID, status, tallyCompany, tallyPort, version)
}

func (s *syncService) SaveMasters(ctx context.Context, tenantID uuid.UUID, masters MasterPayload) error {
	if len(masters.Ledgers) > 0 {
		if err := s.masterRepo.UpsertLedgers(ctx, tenantID, masters.Ledgers); err != nil {
			return err
		}
	}
	if len(masters.StockItems) > 0 {
		if err := s.masterRepo.UpsertStockItems(ctx, tenantID, masters.StockItems); err != nil {
			return err
		}
	}
	if len(masters.Godowns) > 0 {
		if err := s.masterRepo.UpsertGodowns(ctx, tenantID, masters.Godowns); err != nil {
			return err
		}
	}
	if len(masters.Units) > 0 {
		if err := s.masterRepo.UpsertUnits(ctx, tenantID, masters.Units); err != nil {
			return err
		}
	}
	if len(masters.CostCentres) > 0 {
		if err := s.masterRepo.UpsertCostCentres(ctx, tenantID, masters.CostCentres); err != nil {
			return err
		}
	}
	return nil
}

func (s *syncService) ListOutbound(ctx context.Context, tenantID uuid.UUID, cursor string, limit int) ([]OutboundItem, string, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}

	docs, nextCursor, err := s.syncRepo.ListOutboundDocuments(ctx, tenantID, cursor, limit)
	if err != nil {
		return nil, "", err
	}

	items := make([]OutboundItem, 0, len(docs))
	for i := range docs {
		doc := &docs[i]

		voucherDef, buildErr := s.voucherBuilder.Build(ctx, tenantID, doc)
		if buildErr != nil {
			log.Printf("[sync] failed to build voucher for doc %s: %v", doc.ID, buildErr)
			continue
		}

		// Create a pending sync event for tracking
		event := &domain.SyncEvent{
			TenantID:  tenantID,
			DocumentID: &doc.ID,
			Direction: domain.SyncDirectionOutbound,
			Status:    domain.SyncStatusPending,
		}
		// AgentID will be set from auth context in the handler
		if createErr := s.syncRepo.CreateSyncEvent(ctx, event); createErr != nil {
			log.Printf("[sync] failed to create sync event for doc %s: %v", doc.ID, createErr)
			continue
		}

		items = append(items, OutboundItem{
			DocumentID:     doc.ID,
			StructuredData: doc.StructuredData,
			VoucherDef:     voucherDef,
			SyncEventID:    event.ID,
		})
	}

	return items, nextCursor, nil
}

func (s *syncService) AckOutbound(ctx context.Context, tenantID, serviceAccountID uuid.UUID, results []AckResult) error {
	for i := range results {
		r := &results[i]
		status := domain.SyncStatusSuccess
		if !r.Success {
			status = domain.SyncStatusFailed
		}
		if err := s.syncRepo.UpdateSyncEventStatus(ctx, r.SyncEventID, status, r.TallyVoucherID, r.TallyVoucherNumber, r.ErrorMessage); err != nil {
			log.Printf("[sync] failed to ack sync event %s: %v", r.SyncEventID, err)
		}
	}
	return nil
}

func (s *syncService) SaveInbound(ctx context.Context, tenantID uuid.UUID, vouchers []domain.TallyVoucher) error {
	return s.voucherRepo.UpsertVouchers(ctx, tenantID, vouchers)
}

func (s *syncService) GetAgentStatus(ctx context.Context, tenantID uuid.UUID) (*domain.ConnectorAgent, error) {
	// For now, return the first agent for the tenant
	// Future: support multiple agents
	return nil, domain.ErrAgentNotFound
}
```

**Step 2: Create mock**

Create `mocks/mock_sync_service.go` — hand-written mock implementing `SyncService` interface with `testify/mock`.

**Step 3: Run lint**

Run: `make lint`
Expected: PASS.

**Step 4: Commit**

```bash
git add internal/service/sync_service.go mocks/mock_sync_service.go
git commit -m "feat: implement SyncService for connector operations

Handles agent registration, heartbeat, master data sync,
outbound document queue with voucher building, ACK processing,
and inbound voucher persistence."
```

---

### Task 7: Voucher Builder Service

This is the core brain — matches parsed invoices against synced Tally masters.

**Files:**
- Create: `internal/service/voucher_builder.go`
- Create: `tests/unit/service/voucher_builder_test.go`

**Step 1: Write the failing tests**

Create `tests/unit/service/voucher_builder_test.go` with test cases:

1. **TestVoucherBuilder_FullMatch** — all entities matched (GSTIN found, HSN found, tax ledger found)
2. **TestVoucherBuilder_NoLedgerMatch** — GSTIN not in masters, falls back to convention-based name
3. **TestVoucherBuilder_NoStockItem** — HSN not found, uses line item description
4. **TestVoucherBuilder_NoTaxLedger** — tax type+rate not found, uses convention name
5. **TestVoucherBuilder_EmptyMasters** — no masters synced at all, all convention-based
6. **TestVoucherBuilder_InterstateTax** — IGST instead of CGST/SGST

Each test should mock `TallyMasterRepository` and verify the returned `VoucherDef` fields and `MatchConfidence`.

```go
package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"satvos/internal/domain"
	"satvos/internal/service"
)

// ... mock TallyMasterRepository ...

func TestVoucherBuilder_FullMatch(t *testing.T) {
	masterRepo := new(MockTallyMasterRepo)
	builder := service.NewVoucherBuilderService(masterRepo)

	tenantID := uuid.New()
	doc := testDocument(tenantID)

	// Party ledger matched by GSTIN
	masterRepo.On("FindLedgerByGSTIN", mock.Anything, tenantID, "27AABCU9603R1ZM").
		Return(&domain.TallyLedger{Name: "Seller Pvt Ltd"}, nil)

	// Purchase ledger
	masterRepo.On("FindPurchaseLedger", mock.Anything, tenantID).
		Return(&domain.TallyLedger{Name: "Purchase Account"}, nil)

	// Tax ledgers
	masterRepo.On("FindTaxLedger", mock.Anything, tenantID, "CGST", 9.0).
		Return(&domain.TallyLedger{Name: "Input CGST @9%"}, nil)
	masterRepo.On("FindTaxLedger", mock.Anything, tenantID, "SGST", 9.0).
		Return(&domain.TallyLedger{Name: "Input SGST @9%"}, nil)

	// Stock item by HSN
	masterRepo.On("FindStockItemByHSN", mock.Anything, tenantID, "8471").
		Return(&domain.TallyStockItem{Name: "Computer", DefaultUOM: "Nos"}, nil)

	// Default godown
	masterRepo.On("GetDefaultGodown", mock.Anything, tenantID).
		Return(&domain.TallyGodown{Name: "Main Godown"}, nil)

	// Unit
	masterRepo.On("FindUnitBySymbol", mock.Anything, tenantID, "NOS").
		Return(&domain.TallyUnit{Symbol: "Nos"}, nil)

	vdef, err := builder.Build(ctx, tenantID, doc)
	require.NoError(t, err)
	assert.Equal(t, "Seller Pvt Ltd", vdef.PartyLedger)
	assert.Equal(t, "exact_gstin", vdef.MatchConfidence["party_ledger"])
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./tests/unit/service/voucher_builder_test.go -v`
Expected: FAIL (service not implemented yet).

**Step 3: Implement the voucher builder**

Create `internal/service/voucher_builder.go`:

```go
package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/normalize"
	"satvos/internal/port"
	"satvos/internal/validator/invoice"
)

// VoucherBuilderService builds smart-matched VoucherDefs from parsed documents.
type VoucherBuilderService interface {
	Build(ctx context.Context, tenantID uuid.UUID, doc *domain.Document) (*domain.VoucherDef, error)
}

type voucherBuilder struct {
	masterRepo port.TallyMasterRepository
}

func NewVoucherBuilderService(masterRepo port.TallyMasterRepository) VoucherBuilderService {
	return &voucherBuilder{masterRepo: masterRepo}
}

func (b *voucherBuilder) Build(ctx context.Context, tenantID uuid.UUID, doc *domain.Document) (*domain.VoucherDef, error) {
	if doc.StructuredData == nil {
		return nil, fmt.Errorf("document %s has no structured data", doc.ID)
	}

	var inv invoice.GSTInvoice
	if err := json.Unmarshal(doc.StructuredData, &inv); err != nil {
		return nil, fmt.Errorf("parsing structured data for doc %s: %w", doc.ID, err)
	}

	confidence := make(map[string]string)

	// 1. Match party ledger
	partyLedger := b.matchPartyLedger(ctx, tenantID, &inv, confidence)

	// 2. Match purchase ledger
	purchaseLedger := b.matchPurchaseLedger(ctx, tenantID, confidence)

	// 3. Match tax ledgers
	taxEntries := b.matchTaxLedgers(ctx, tenantID, &inv, confidence)

	// 4. Match stock items + godown + UOM
	inventoryItems := b.matchInventoryItems(ctx, tenantID, &inv, confidence)

	// 5. Build VoucherDef
	vdef := &domain.VoucherDef{
		DocumentID:      doc.ID,
		VoucherType:     "Purchase",
		VoucherDate:     inv.Invoice.Date,
		PartyLedger:     partyLedger,
		PurchaseLedger:  purchaseLedger,
		TaxEntries:      taxEntries,
		InventoryItems:  inventoryItems,
		TotalAmount:     inv.Totals.Total,
		Narration:       fmt.Sprintf("Purchase from %s, Invoice %s", inv.Seller.Name, inv.Invoice.Number),
		RemoteID:        fmt.Sprintf("%s-%s", tenantID, doc.ID),
		MatchConfidence: confidence,
	}

	return vdef, nil
}

func (b *voucherBuilder) matchPartyLedger(ctx context.Context, tenantID uuid.UUID, inv *invoice.GSTInvoice, confidence map[string]string) string {
	// Priority 1: GSTIN exact match
	if inv.Seller.GSTIN != "" {
		ledger, err := b.masterRepo.FindLedgerByGSTIN(ctx, tenantID, inv.Seller.GSTIN)
		if err == nil && ledger != nil {
			confidence["party_ledger"] = "exact_gstin"
			return ledger.Name
		}
	}

	// Priority 2: Normalized name match — would require listing all ledgers and fuzzy matching.
	// For now, use convention-based name.
	confidence["party_ledger"] = "convention"
	return normalize.CompanyName(inv.Seller.Name)
}

func (b *voucherBuilder) matchPurchaseLedger(ctx context.Context, tenantID uuid.UUID, confidence map[string]string) string {
	ledger, err := b.masterRepo.FindPurchaseLedger(ctx, tenantID)
	if err == nil && ledger != nil {
		confidence["purchase_ledger"] = "exact_group"
		return ledger.Name
	}
	confidence["purchase_ledger"] = "convention"
	return "Purchase Accounts"
}

func (b *voucherBuilder) matchTaxLedgers(ctx context.Context, tenantID uuid.UUID, inv *invoice.GSTInvoice, confidence map[string]string) []domain.VoucherDefTaxEntry {
	var entries []domain.VoucherDefTaxEntry

	type taxInfo struct {
		taxType string
		amount  float64
		rate    float64
	}

	var taxes []taxInfo
	if inv.Totals.CGST > 0 {
		rate := 0.0
		if inv.Totals.TaxableAmount > 0 {
			rate = (inv.Totals.CGST / inv.Totals.TaxableAmount) * 100
		}
		taxes = append(taxes, taxInfo{"CGST", inv.Totals.CGST, rate})
	}
	if inv.Totals.SGST > 0 {
		rate := 0.0
		if inv.Totals.TaxableAmount > 0 {
			rate = (inv.Totals.SGST / inv.Totals.TaxableAmount) * 100
		}
		taxes = append(taxes, taxInfo{"SGST", inv.Totals.SGST, rate})
	}
	if inv.Totals.IGST > 0 {
		rate := 0.0
		if inv.Totals.TaxableAmount > 0 {
			rate = (inv.Totals.IGST / inv.Totals.TaxableAmount) * 100
		}
		taxes = append(taxes, taxInfo{"IGST", inv.Totals.IGST, rate})
	}

	for _, tax := range taxes {
		ledger, err := b.masterRepo.FindTaxLedger(ctx, tenantID, tax.taxType, tax.rate)
		if err == nil && ledger != nil {
			confidence[fmt.Sprintf("tax_%s", tax.taxType)] = "exact_rate"
			entries = append(entries, domain.VoucherDefTaxEntry{
				LedgerName: ledger.Name,
				Amount:     tax.amount,
			})
		} else {
			confidence[fmt.Sprintf("tax_%s", tax.taxType)] = "convention"
			entries = append(entries, domain.VoucherDefTaxEntry{
				LedgerName: fmt.Sprintf("Input %s @%.0f%%", tax.taxType, tax.rate),
				Amount:     tax.amount,
			})
		}
	}

	return entries
}

func (b *voucherBuilder) matchInventoryItems(ctx context.Context, tenantID uuid.UUID, inv *invoice.GSTInvoice, confidence map[string]string) []domain.VoucherDefItem {
	items := make([]domain.VoucherDefItem, 0, len(inv.LineItems))

	defaultGodown := ""
	godown, err := b.masterRepo.GetDefaultGodown(ctx, tenantID)
	if err == nil && godown != nil {
		defaultGodown = godown.Name
	}

	for i := range inv.LineItems {
		li := &inv.LineItems[i]
		item := domain.VoucherDefItem{
			Quantity: li.Quantity,
			Rate:     li.Rate,
			Amount:   li.Amount,
			HSNCode:  li.HSNCode,
			Godown:   defaultGodown,
		}

		// Match stock item by HSN
		if li.HSNCode != "" {
			stockItem, findErr := b.masterRepo.FindStockItemByHSN(ctx, tenantID, li.HSNCode)
			if findErr == nil && stockItem != nil {
				item.StockItem = stockItem.Name
				if stockItem.DefaultUOM != "" {
					item.UOM = stockItem.DefaultUOM
				}
				confidence[fmt.Sprintf("item_%d", i)] = "exact_hsn"
			} else {
				item.StockItem = li.Description
				confidence[fmt.Sprintf("item_%d", i)] = "description_fallback"
			}
		} else {
			item.StockItem = li.Description
			confidence[fmt.Sprintf("item_%d", i)] = "no_hsn"
		}

		// Match UOM if not already set from stock item
		if item.UOM == "" && li.UOM != "" {
			unit, findErr := b.masterRepo.FindUnitBySymbol(ctx, tenantID, li.UOM)
			if findErr == nil && unit != nil {
				item.UOM = unit.Symbol
			} else {
				item.UOM = li.UOM
			}
		}
		if item.UOM == "" {
			item.UOM = "Nos"
		}

		items = append(items, item)
	}

	return items
}
```

**Step 4: Run tests**

Run: `go test ./tests/unit/service/voucher_builder_test.go -v`
Expected: PASS.

**Step 5: Run lint**

Run: `make lint`
Expected: PASS.

**Step 6: Commit**

```bash
git add internal/service/voucher_builder.go tests/unit/service/voucher_builder_test.go
git commit -m "feat: implement VoucherBuilderService for smart Tally matching

Matches parsed invoices against synced Tally masters:
GSTIN for party ledgers, tax_type+rate for tax ledgers,
HSN for stock items, with convention-based fallbacks.
Reports match confidence per entity."
```

---

### Task 8: Sync Handler (API Endpoints)

**Files:**
- Create: `internal/handler/sync_handler.go`
- Create: `tests/unit/handler/sync_handler_test.go`

**Step 1: Write handler tests**

Create `tests/unit/handler/sync_handler_test.go` — tests for:
1. `POST /sync/v1/register` — success, missing fields
2. `POST /sync/v1/heartbeat` — success, agent not found
3. `POST /sync/v1/masters` — success with ledgers + stock items
4. `GET /sync/v1/outbound` — success with cursor pagination
5. `POST /sync/v1/ack` — success, partial failures
6. `POST /sync/v1/inbound` — success with vouchers
7. Auth context missing → 401

**Step 2: Implement sync handler**

Create `internal/handler/sync_handler.go`:

```go
package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"satvos/internal/domain"
	"satvos/internal/service"
)

type SyncHandler struct {
	syncSvc service.SyncService
}

func NewSyncHandler(syncSvc service.SyncService) *SyncHandler {
	return &SyncHandler{syncSvc: syncSvc}
}

// Register handles POST /sync/v1/register
func (h *SyncHandler) Register(c *gin.Context) {
	tenantID, userID, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	var req struct {
		Version string `json:"version"`
		OSInfo  string `json:"os_info"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	agent, err := h.syncSvc.Register(c.Request.Context(), tenantID, userID, req.Version, req.OSInfo)
	if err != nil {
		HandleError(c, err)
		return
	}
	RespondCreated(c, agent)
}

// Heartbeat handles POST /sync/v1/heartbeat
func (h *SyncHandler) Heartbeat(c *gin.Context) {
	tenantID, userID, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	var req struct {
		TallyConnected bool     `json:"tally_connected"`
		TallyCompany   string   `json:"tally_company"`
		TallyPort      int      `json:"tally_port"`
		Version        string   `json:"version"`
		Errors         []string `json:"errors"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	if err := h.syncSvc.Heartbeat(c.Request.Context(), tenantID, userID, req.TallyConnected, req.TallyCompany, req.TallyPort, req.Version, req.Errors); err != nil {
		HandleError(c, err)
		return
	}
	RespondOK(c, gin.H{"status": "ok"})
}

// Masters handles POST /sync/v1/masters
func (h *SyncHandler) Masters(c *gin.Context) {
	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	var payload service.MasterPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	if err := h.syncSvc.SaveMasters(c.Request.Context(), tenantID, payload); err != nil {
		HandleError(c, err)
		return
	}
	RespondOK(c, gin.H{"status": "ok"})
}

// Outbound handles GET /sync/v1/outbound
func (h *SyncHandler) Outbound(c *gin.Context) {
	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	cursor := c.Query("cursor")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 || limit > 50 {
		limit = 50
	}

	items, nextCursor, err := h.syncSvc.ListOutbound(c.Request.Context(), tenantID, cursor, limit)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{
		"items":       items,
		"next_cursor": nextCursor,
	})
}

// Ack handles POST /sync/v1/ack
func (h *SyncHandler) Ack(c *gin.Context) {
	tenantID, userID, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	var req struct {
		Results []service.AckResult `json:"results"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	if err := h.syncSvc.AckOutbound(c.Request.Context(), tenantID, userID, req.Results); err != nil {
		HandleError(c, err)
		return
	}
	RespondOK(c, gin.H{"status": "ok"})
}

// Inbound handles POST /sync/v1/inbound
func (h *SyncHandler) Inbound(c *gin.Context) {
	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	var req struct {
		Vouchers []domain.TallyVoucher `json:"vouchers"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	if err := h.syncSvc.SaveInbound(c.Request.Context(), tenantID, req.Vouchers); err != nil {
		HandleError(c, err)
		return
	}
	RespondOK(c, gin.H{"status": "ok"})
}
```

**Step 3: Run tests**

Run: `go test ./tests/unit/handler/sync_handler_test.go -v`
Expected: PASS.

**Step 4: Run lint**

Run: `make lint`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/handler/sync_handler.go tests/unit/handler/sync_handler_test.go
git commit -m "feat: implement SyncHandler for connector API endpoints

Handles register, heartbeat, masters upload, outbound pull,
ack, and inbound voucher upload. All service-account auth."
```

---

### Task 9: Router Wiring and main.go Integration

**Files:**
- Modify: `internal/router/router.go` — add sync API routes
- Modify: `cmd/server/main.go` — wire repos, services, handler

**Step 1: Add sync routes to router**

Add to `router.go` — a new route group under `protected`:

```go
// Sync API — connector agent endpoints (service account auth only)
syncGroup := protected.Group("/sync/v1")
syncGroup.Use(middleware.RequireRole(domain.RoleService))
{
    syncGroup.POST("/register", syncH.Register)
    syncGroup.POST("/heartbeat", syncH.Heartbeat)
    syncGroup.POST("/masters", syncH.Masters)
    syncGroup.GET("/outbound", syncH.Outbound)
    syncGroup.POST("/ack", syncH.Ack)
    syncGroup.POST("/inbound", syncH.Inbound)
}
```

Update `Setup()` signature to accept `syncH *handler.SyncHandler`.

**Step 2: Wire in main.go**

Add to `cmd/server/main.go`:

```go
// Tally connector repos
tallyMasterRepo := postgres.NewTallyMasterRepo(db)
syncRepo := postgres.NewSyncRepo(db)
tallyVoucherRepo := postgres.NewTallyVoucherRepo(db)

// Voucher builder + sync service
voucherBuilder := service.NewVoucherBuilderService(tallyMasterRepo)
syncSvc := service.NewSyncService(syncRepo, tallyMasterRepo, tallyVoucherRepo, voucherBuilder)

// Sync handler
syncH := handler.NewSyncHandler(syncSvc)
```

Pass `syncH` to `router.Setup(...)`.

**Step 3: Run `make build` to verify compilation**

Run: `make build`
Expected: PASS, binary compiles.

**Step 4: Run all tests**

Run: `make test-unit`
Expected: All existing + new tests pass.

**Step 5: Run lint**

Run: `make lint`
Expected: PASS.

**Step 6: Commit**

```bash
git add internal/router/router.go cmd/server/main.go
git commit -m "feat: wire tally connector sync API into router and main

Adds /sync/v1/ route group (service account auth only) with
register, heartbeat, masters, outbound, ack, inbound endpoints.
Wires TallyMasterRepo, SyncRepo, TallyVoucherRepo, VoucherBuilderService,
SyncService, and SyncHandler in main.go."
```

---

## Part 2 — Connector Agent (New Repo)

All agent code lives in a new repo at `/home/muditsahni/Documents/Projects/satvos-tally-connector`.

---

### Task 10: Initialize Go Module

**Files:**
- Create: `go.mod`
- Create: `go.sum` (auto-generated)
- Create: `cmd/connector/main.go` (skeleton)
- Create: `README.md`
- Create: `Makefile`
- Create: `.gitignore`
- Create: `configs/connector.example.yaml`

**Step 1: Create the repo directory and initialize Go module**

```bash
mkdir -p /home/muditsahni/Documents/Projects/satvos-tally-connector
cd /home/muditsahni/Documents/Projects/satvos-tally-connector
git init
go mod init github.com/mudsahni/satvos-tally-connector
```

**Step 2: Create skeleton main.go**

Create `cmd/connector/main.go`:

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("SATVOS Tally Connector starting...")
	// TODO: config, sync engine, tally client, cloud client
	return nil
}
```

**Step 3: Create Makefile**

```makefile
.PHONY: build run test lint clean

BINARY=satvos-connector

build:
	go build -o bin/$(BINARY) ./cmd/connector

build-windows:
	GOOS=windows GOARCH=amd64 go build -o bin/$(BINARY).exe ./cmd/connector

run:
	go run ./cmd/connector

test:
	go test ./... -v -count=1 -race

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/
```

**Step 4: Create .gitignore**

```
bin/
*.exe
configs/connector.yaml
.env
```

**Step 5: Create example config**

Create `configs/connector.example.yaml`:

```yaml
satvos:
  base_url: "https://api.satvos.com"
  api_key: "sk_your_api_key_here"

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

**Step 6: Commit**

```bash
git add -A
git commit -m "feat: initialize satvos-tally-connector Go module

Skeleton entry point, Makefile with build/test/lint targets,
example config, .gitignore."
```

---

### Task 11: Config Package

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Step 1: Write config tests**

Test cases: defaults loaded, env var override (`CONNECTOR_SATVOS_API_KEY`), config file loading, validation (API key required).

**Step 2: Implement config**

Create `internal/config/config.go`:

```go
package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	SATVOS SATVOSConfig `mapstructure:"satvos"`
	Tally  TallyConfig  `mapstructure:"tally"`
	Sync   SyncConfig   `mapstructure:"sync"`
	UI     UIConfig     `mapstructure:"ui"`
}

type SATVOSConfig struct {
	BaseURL string `mapstructure:"base_url"`
	APIKey  string `mapstructure:"api_key"`
}

type TallyConfig struct {
	Host    string `mapstructure:"host"`
	Port    int    `mapstructure:"port"`
	Company string `mapstructure:"company"`
}

type SyncConfig struct {
	IntervalSeconds int `mapstructure:"interval_seconds"`
	BatchSize       int `mapstructure:"batch_size"`
	RetryAttempts   int `mapstructure:"retry_attempts"`
}

type UIConfig struct {
	Port int `mapstructure:"port"`
}

func Load() (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("satvos.base_url", "https://api.satvos.com")
	v.SetDefault("tally.host", "localhost")
	v.SetDefault("tally.port", 0)
	v.SetDefault("sync.interval_seconds", 30)
	v.SetDefault("sync.batch_size", 50)
	v.SetDefault("sync.retry_attempts", 3)
	v.SetDefault("ui.port", 8321)

	// Config file search paths
	v.SetConfigName("connector")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./configs")
	v.AddConfigPath("$APPDATA/satvos-connector")
	v.AddConfigPath("$HOME/.satvos-connector")

	// Env vars: CONNECTOR_ prefix
	v.SetEnvPrefix("CONNECTOR")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.SATVOS.APIKey == "" {
		return fmt.Errorf("satvos.api_key is required (set CONNECTOR_SATVOS_API_KEY or in config file)")
	}
	if c.Sync.IntervalSeconds < 5 {
		c.Sync.IntervalSeconds = 5
	}
	if c.Sync.BatchSize < 1 || c.Sync.BatchSize > 100 {
		c.Sync.BatchSize = 50
	}
	return nil
}
```

**Step 3: Run tests**

Run: `go test ./internal/config/... -v`
Expected: PASS.

**Step 4: Commit**

```bash
git add internal/config/
git commit -m "feat: add Viper-based config with CONNECTOR_ env prefix

Loads from connector.yaml or env vars. API key required,
sensible defaults for sync interval, batch size, UI port."
```

---

### Task 12: Tally Client (XML over HTTP)

**Files:**
- Create: `internal/tally/client.go`
- Create: `internal/tally/requests.go`
- Create: `internal/tally/responses.go`
- Create: `internal/tally/health.go`
- Create: `internal/tally/client_test.go`

**Step 1: Write tally client tests**

Test with `httptest.NewServer` that returns mock Tally XML responses. Test cases:
1. Health check — valid Tally response
2. Health check — connection refused
3. Get company info
4. Get ledgers — parse XML response
5. Get stock items
6. Import voucher — success response
7. Import voucher — error response (duplicate REMOTEID)

**Step 2: Implement tally client**

Create `internal/tally/client.go`:

```go
package tally

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client communicates with Tally Prime via XML over HTTP.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(host string, port int) *Client {
	return &Client{
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SendRequest sends an XML request to Tally and returns the raw XML response.
func (c *Client) SendRequest(ctx context.Context, xmlBody []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(xmlBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request to tally: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading tally response: %w", err)
	}

	return body, nil
}

// IsAvailable checks if Tally is reachable and responding.
func (c *Client) IsAvailable(ctx context.Context) bool {
	_, err := c.GetCompanyInfo(ctx)
	return err == nil
}
```

**Step 3: Implement request builders**

Create `internal/tally/requests.go` — functions that build Tally XML request envelopes for:
- `BuildLedgerExportRequest()` — export all ledgers
- `BuildStockItemExportRequest()` — export all stock items
- `BuildGodownExportRequest()` — export all godowns
- `BuildUnitExportRequest()` — export all units
- `BuildCostCentreExportRequest()` — export all cost centres
- `BuildCompanyInfoRequest()` — get company info
- `BuildVoucherImportRequest(xml string)` — import a voucher

**Step 4: Implement response parsers**

Create `internal/tally/responses.go` — XML struct types + parse functions for each response type. Uses `encoding/xml` for parsing (Tally response XML is standard enough for struct-based parsing).

**Step 5: Implement health check**

Create `internal/tally/health.go`:

```go
package tally

import "context"

// CompanyInfo holds basic Tally company information.
type CompanyInfo struct {
	Name         string
	FormalName   string
	BooksBegFrom string
	BooksEndAt   string
}

// GetCompanyInfo retrieves the active company from Tally.
func (c *Client) GetCompanyInfo(ctx context.Context) (*CompanyInfo, error) {
	xml := BuildCompanyInfoRequest()
	resp, err := c.SendRequest(ctx, xml)
	if err != nil {
		return nil, err
	}
	return ParseCompanyInfoResponse(resp)
}
```

**Step 6: Run tests**

Run: `go test ./internal/tally/... -v`
Expected: PASS.

**Step 7: Commit**

```bash
git add internal/tally/
git commit -m "feat: implement Tally XML-over-HTTP client

Sends XML requests to Tally Prime on localhost, parses responses
for ledgers, stock items, godowns, units, cost centres.
Includes health check and voucher import."
```

---

### Task 13: Tally Port Discovery

**Files:**
- Create: `internal/tally/discover.go`
- Create: `internal/tally/discover_test.go`

**Step 1: Write discovery tests**

Test with multiple `httptest.NewServer` instances. Test cases:
1. Default port 9000 responds — returns 9000
2. Port 9000 fails, 9001 responds — returns 9001
3. All ports fail — returns error
4. Cached port still works — returns cached without scanning

**Step 2: Implement discovery**

Create `internal/tally/discover.go`:

```go
package tally

import (
	"context"
	"fmt"
	"log"
)

const (
	DefaultPort = 9000
	MaxPort     = 9010
)

// Discover scans localhost ports 9000-9010 for a running Tally instance.
func Discover(ctx context.Context, host string) (int, error) {
	for port := DefaultPort; port <= MaxPort; port++ {
		client := NewClient(host, port)
		info, err := client.GetCompanyInfo(ctx)
		if err == nil && info != nil {
			log.Printf("[tally] discovered Tally on port %d (company: %s)", port, info.Name)
			return port, nil
		}
	}
	return 0, fmt.Errorf("no Tally instance found on %s ports %d-%d", host, DefaultPort, MaxPort)
}
```

**Step 3: Run tests**

Run: `go test ./internal/tally/... -v -run TestDiscover`
Expected: PASS.

**Step 4: Commit**

```bash
git add internal/tally/discover.go internal/tally/discover_test.go
git commit -m "feat: add Tally port auto-discovery

Scans localhost:9000-9010 for a running Tally instance by
sending a lightweight company info XML request to each port."
```

---

### Task 14: Cloud Client (SATVOS Sync API)

**Files:**
- Create: `internal/cloud/client.go`
- Create: `internal/cloud/types.go`
- Create: `internal/cloud/client_test.go`

**Step 1: Write cloud client tests**

Test with `httptest.NewServer`. Test cases:
1. Register — success 201
2. Heartbeat — success 200
3. PushMasters — success 200
4. PullOutbound — success with items + cursor
5. Ack — success 200
6. PushInbound — success 200
7. Auth header sent correctly (`Authorization: Bearer sk_...`)
8. Server error — retries
9. Network timeout — returns error

**Step 2: Implement cloud client types**

Create `internal/cloud/types.go` — DTOs matching the SATVOS sync API request/response shapes.

**Step 3: Implement cloud client**

Create `internal/cloud/client.go`:

```go
package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client communicates with the SATVOS Sync API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
	}

	return nil
}

func (c *Client) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	var resp RegisterResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/sync/v1/register", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Heartbeat(ctx context.Context, req HeartbeatRequest) error {
	return c.do(ctx, http.MethodPost, "/api/v1/sync/v1/heartbeat", req, nil)
}

func (c *Client) PushMasters(ctx context.Context, req MastersRequest) error {
	return c.do(ctx, http.MethodPost, "/api/v1/sync/v1/masters", req, nil)
}

func (c *Client) PullOutbound(ctx context.Context, cursor string, limit int) (*OutboundResponse, error) {
	path := fmt.Sprintf("/api/v1/sync/v1/outbound?cursor=%s&limit=%d", cursor, limit)
	var resp OutboundResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Ack(ctx context.Context, req AckRequest) error {
	return c.do(ctx, http.MethodPost, "/api/v1/sync/v1/ack", req, nil)
}

func (c *Client) PushInbound(ctx context.Context, req InboundRequest) error {
	return c.do(ctx, http.MethodPost, "/api/v1/sync/v1/inbound", req, nil)
}
```

**Step 4: Run tests**

Run: `go test ./internal/cloud/... -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/cloud/
git commit -m "feat: implement SATVOS Sync API client

HTTPS client for register, heartbeat, masters push,
outbound pull, ack, and inbound push. Bearer token auth."
```

---

### Task 15: VoucherDef to Tally XML Converter

**Files:**
- Create: `internal/convert/xml.go`
- Create: `internal/convert/template.go`
- Create: `internal/convert/xml_test.go`

**Step 1: Write conversion tests**

Test cases:
1. Full voucher with inventory + tax entries → valid Tally XML
2. Verify REMOTEID is set for idempotency
3. Verify amount signs (party=positive, tax=negative, inventory=negative)
4. Verify ISDEEMEDPOSITIVE flags
5. Empty inventory items → voucher without ALLINVENTORYENTRIES
6. Multiple tax entries → correct XML nesting

**Step 2: Implement template**

Create `internal/convert/template.go` — Tally XML template (based on existing `tallyexport/template.go` from SATVOS, but consuming `VoucherDef` instead of `GSTInvoice`).

**Step 3: Implement converter**

Create `internal/convert/xml.go`:

```go
package convert

import (
	"bytes"
	"fmt"
)

// VoucherDefToXML converts a VoucherDef JSON to Tally-importable XML.
func VoucherDefToXML(def *VoucherDef) (string, error) {
	var buf bytes.Buffer
	if err := voucherTemplate.Execute(&buf, toTemplateData(def)); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buf.String(), nil
}
```

**Step 4: Run tests**

Run: `go test ./internal/convert/... -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/convert/
git commit -m "feat: implement VoucherDef to Tally XML converter

Template-based XML generation with correct amount signs,
ISDEEMEDPOSITIVE flags, and REMOTEID for idempotent import."
```

---

### Task 16: Sync Engine (Main Loop Orchestrator)

**Files:**
- Create: `internal/sync/engine.go`
- Create: `internal/sync/masters.go`
- Create: `internal/sync/outbound.go`
- Create: `internal/sync/inbound.go`
- Create: `internal/sync/state.go`
- Create: `internal/sync/engine_test.go`

**Step 1: Write engine tests**

Test the orchestrator with mock cloud client and mock tally client. Test cases:
1. Full sync cycle (heartbeat → push masters → pull outbound → push inbound)
2. Tally not available — skip sync, report in heartbeat
3. SATVOS unreachable — exponential backoff
4. Outbound with items — converts and imports each
5. Import error — ACKs as failed, continues

**Step 2: Implement state tracking**

Create `internal/sync/state.go` — persists cursors and state to JSON file at `%APPDATA%/satvos-connector/state.json`.

**Step 3: Implement master sync**

Create `internal/sync/masters.go` — reads all master types from Tally, pushes to SATVOS.

**Step 4: Implement outbound sync**

Create `internal/sync/outbound.go` — pulls outbound items from SATVOS, converts VoucherDef to XML, imports into Tally, ACKs results.

**Step 5: Implement inbound sync**

Create `internal/sync/inbound.go` — reads vouchers from Tally, pushes to SATVOS.

**Step 6: Implement engine**

Create `internal/sync/engine.go`:

```go
package sync

import (
	"context"
	"log"
	"time"

	"github.com/mudsahni/satvos-tally-connector/internal/cloud"
	"github.com/mudsahni/satvos-tally-connector/internal/config"
	"github.com/mudsahni/satvos-tally-connector/internal/tally"
)

// Engine orchestrates the sync cycle between Tally and SATVOS.
type Engine struct {
	cfg         *config.Config
	cloudClient *cloud.Client
	tallyClient *tally.Client
	state       *State
	stopCh      chan struct{}
}

func NewEngine(cfg *config.Config, cloudClient *cloud.Client, tallyClient *tally.Client) *Engine {
	return &Engine{
		cfg:         cfg,
		cloudClient: cloudClient,
		tallyClient: tallyClient,
		state:       NewState(),
		stopCh:      make(chan struct{}),
	}
}

// Start begins the sync loop. Blocks until Stop() is called or context is cancelled.
func (e *Engine) Start(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(e.cfg.Sync.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	// Run immediately on start
	e.runCycle(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-e.stopCh:
			return nil
		case <-ticker.C:
			e.runCycle(ctx)
		}
	}
}

// Stop signals the engine to stop.
func (e *Engine) Stop() {
	close(e.stopCh)
}

func (e *Engine) runCycle(ctx context.Context) {
	log.Println("[sync] starting sync cycle")

	// 1. Heartbeat
	tallyAvailable := e.tallyClient.IsAvailable(ctx)
	if err := e.sendHeartbeat(ctx, tallyAvailable); err != nil {
		log.Printf("[sync] heartbeat failed: %v", err)
	}

	if !tallyAvailable {
		log.Println("[sync] tally not available, skipping sync")
		return
	}

	// 2. Push masters (Tally → SATVOS)
	if err := e.pushMasters(ctx); err != nil {
		log.Printf("[sync] push masters failed: %v", err)
	}

	// 3. Pull outbound (SATVOS → Tally)
	if err := e.processOutbound(ctx); err != nil {
		log.Printf("[sync] outbound failed: %v", err)
	}

	// 4. Push inbound (Tally → SATVOS)
	if err := e.pushInbound(ctx); err != nil {
		log.Printf("[sync] inbound failed: %v", err)
	}

	log.Println("[sync] sync cycle complete")
}
```

**Step 7: Run tests**

Run: `go test ./internal/sync/... -v`
Expected: PASS.

**Step 8: Commit**

```bash
git add internal/sync/
git commit -m "feat: implement sync engine with 4-phase cycle

Orchestrates heartbeat → push masters → pull outbound → push inbound
every 30 seconds. Handles Tally unavailability and SATVOS errors."
```

---

### Task 17: Local Store (JSON State Persistence)

**Files:**
- Create: `internal/store/local.go`
- Create: `internal/store/local_test.go`

**Step 1: Write tests**

Test cases: save/load state, file doesn't exist (returns defaults), concurrent access.

**Step 2: Implement**

JSON file at `%APPDATA%/satvos-connector/state.json` on Windows, `~/.satvos-connector/state.json` on other platforms. Stores sync cursors, discovered Tally port, pending ACK queue.

**Step 3: Commit**

```bash
git add internal/store/
git commit -m "feat: add JSON-based local state persistence

Stores sync cursors, discovered Tally port, and pending
ACK queue at platform-appropriate config directory."
```

---

### Task 18: Local Web UI

**Files:**
- Create: `internal/ui/server.go`
- Create: `internal/ui/handlers.go`
- Create: `internal/ui/static/index.html`
- Create: `internal/ui/static/setup.html`
- Create: `internal/ui/static/style.css`
- Create: `internal/ui/static/app.js`

**Step 1: Implement UI server**

HTTP server on `:8321` using `embed.FS`. Routes:
- `GET /` — Status dashboard
- `GET /setup` — Setup wizard (paste API key, confirm Tally)
- `POST /setup/apikey` — Save API key
- `GET /api/status` — JSON status (for dashboard polling)
- `GET /api/logs` — Recent log entries
- `POST /api/sync` — Trigger immediate sync

**Step 2: Create HTML/CSS/JS**

Simple, clean dashboard. Status page shows: connection status (SATVOS, Tally), sync stats, last sync time, errors. Setup page: API key input, Tally discovery status, "Connect" button.

**Step 3: Commit**

```bash
git add internal/ui/
git commit -m "feat: add local web UI for setup and status monitoring

Embedded HTTP server on :8321 with setup wizard and
status dashboard. Uses embed.FS for single-binary deployment."
```

---

### Task 19: Windows Service Wrapper

**Files:**
- Create: `internal/service/windows.go`

**Step 1: Implement Windows Service**

Uses `golang.org/x/sys/windows/svc` to run as a Windows Service. Falls back to console mode when not running as a service (for development).

```go
//go:build windows

package service

import (
	"golang.org/x/sys/windows/svc"
)

// RunAsService runs the connector as a Windows Service.
func RunAsService(name string, handler svc.Handler) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if isService {
		return svc.Run(name, handler)
	}
	// Console mode — run directly
	return nil
}
```

**Step 2: Commit**

```bash
git add internal/service/
git commit -m "feat: add Windows Service wrapper

Runs as Windows Service when installed, falls back to
console mode for development."
```

---

### Task 20: Wire Everything in main.go

**Files:**
- Modify: `cmd/connector/main.go`

**Step 1: Wire all components**

```go
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Discover Tally
	tallyPort := cfg.Tally.Port
	if tallyPort == 0 {
		tallyPort, err = tally.Discover(context.Background(), cfg.Tally.Host)
		if err != nil {
			log.Printf("WARNING: %v", err)
		}
	}

	// Clients
	cloudClient := cloud.NewClient(cfg.SATVOS.BaseURL, cfg.SATVOS.APIKey)
	tallyClient := tally.NewClient(cfg.Tally.Host, tallyPort)

	// Register with SATVOS
	// ...

	// Start sync engine
	engine := sync.NewEngine(cfg, cloudClient, tallyClient)

	// Start UI server
	uiServer := ui.NewServer(cfg.UI.Port, engine)

	// Graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Run engine and UI in parallel
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return engine.Start(gctx) })
	g.Go(func() error { return uiServer.Start(gctx) })

	return g.Wait()
}
```

**Step 2: Run build**

Run: `make build`
Expected: Compiles successfully.

**Step 3: Run all tests**

Run: `make test`
Expected: All tests pass.

**Step 4: Commit**

```bash
git add cmd/connector/main.go
git commit -m "feat: wire connector main.go with all components

Config → Tally discovery → cloud client → sync engine → UI server
with graceful shutdown via errgroup."
```

---

### Task 21: Install Script

**Files:**
- Create: `scripts/install.ps1`

**Step 1: Create PowerShell installer**

```powershell
# Install SATVOS Tally Connector as a Windows Service
$ServiceName = "SATVOSTallyConnector"
$BinaryPath = "$PSScriptRoot\..\bin\satvos-connector.exe"
$ConfigDir = "$env:APPDATA\satvos-connector"

# Create config directory
New-Item -ItemType Directory -Force -Path $ConfigDir | Out-Null

# Copy binary
Copy-Item $BinaryPath "$ConfigDir\satvos-connector.exe" -Force

# Install service
New-Service -Name $ServiceName -BinaryPathName "$ConfigDir\satvos-connector.exe" `
    -DisplayName "SATVOS Tally Connector" `
    -Description "Syncs SATVOS Cloud with Tally Prime" `
    -StartupType Automatic

# Start service
Start-Service -Name $ServiceName

# Open setup page
Start-Process "http://localhost:8321/setup"
```

**Step 2: Commit**

```bash
git add scripts/
git commit -m "feat: add PowerShell install script for Windows Service

Copies binary to %APPDATA%, registers as Windows Service,
starts service, opens setup page in browser."
```

---

### Task 22: Final Integration Test + README

**Files:**
- Create: `tests/integration/sync_test.go` (in connector repo)
- Modify: `README.md`

**Step 1: Write integration test**

Uses `httptest.NewServer` for both mock SATVOS API and mock Tally server. Runs a full sync cycle and verifies:
1. Agent registers successfully
2. Heartbeat reports Tally connected
3. Masters pushed to SATVOS
4. Outbound items pulled and imported into Tally
5. ACK sent back to SATVOS

**Step 2: Update README**

Comprehensive README with: overview, architecture diagram (text), quick start, configuration reference, development guide, building from source.

**Step 3: Run full test suite**

Run: `make test`
Expected: All tests pass.

**Step 4: Run lint**

Run: `make lint`
Expected: PASS.

**Step 5: Commit**

```bash
git add tests/ README.md
git commit -m "feat: add integration test and README

Full sync cycle integration test with mock servers.
README with architecture overview and configuration reference."
```

---

## Files Summary

### SATVOS Server (existing repo)

| File | Change | Task |
|------|--------|------|
| `db/migrations/000027_create_tally_connector_tables.{up,down}.sql` | New | 1 |
| `internal/domain/models.go` | Modify | 2 |
| `internal/domain/enums.go` | Modify | 2 |
| `internal/domain/errors.go` | Modify | 2 |
| `internal/port/tally_master_repository.go` | New | 3 |
| `internal/port/sync_repository.go` | New | 3 |
| `internal/port/tally_voucher_repository.go` | New | 3 |
| `internal/repository/postgres/tally_master_repo.go` | New | 4 |
| `internal/repository/postgres/sync_repo.go` | New | 5 |
| `internal/repository/postgres/tally_voucher_repo.go` | New | 5 |
| `internal/service/sync_service.go` | New | 6 |
| `mocks/mock_sync_service.go` | New | 6 |
| `internal/service/voucher_builder.go` | New | 7 |
| `tests/unit/service/voucher_builder_test.go` | New | 7 |
| `internal/handler/sync_handler.go` | New | 8 |
| `tests/unit/handler/sync_handler_test.go` | New | 8 |
| `internal/router/router.go` | Modify | 9 |
| `cmd/server/main.go` | Modify | 9 |

### Connector Agent (new repo: `satvos-tally-connector`)

| File | Change | Task |
|------|--------|------|
| `go.mod`, `Makefile`, `.gitignore`, `configs/connector.example.yaml` | New | 10 |
| `cmd/connector/main.go` | New | 10, 20 |
| `internal/config/config.go`, `config_test.go` | New | 11 |
| `internal/tally/client.go`, `requests.go`, `responses.go`, `health.go` | New | 12 |
| `internal/tally/client_test.go` | New | 12 |
| `internal/tally/discover.go`, `discover_test.go` | New | 13 |
| `internal/cloud/client.go`, `types.go`, `client_test.go` | New | 14 |
| `internal/convert/xml.go`, `template.go`, `xml_test.go` | New | 15 |
| `internal/sync/engine.go`, `masters.go`, `outbound.go`, `inbound.go`, `state.go` | New | 16 |
| `internal/sync/engine_test.go` | New | 16 |
| `internal/store/local.go`, `local_test.go` | New | 17 |
| `internal/ui/server.go`, `handlers.go`, `static/*` | New | 18 |
| `internal/service/windows.go` | New | 19 |
| `scripts/install.ps1` | New | 21 |
| `tests/integration/sync_test.go` | New | 22 |
| `README.md` | New | 22 |

---

## Implementation Order

**Phase 1 — Server-side** (Tasks 1-9): Can be built and tested independently against the existing SATVOS server.

**Phase 2 — Agent** (Tasks 10-22): Built in the new repo. Tasks 10-15 are independent packages that can be built in parallel. Tasks 16-20 integrate them.

**Dependencies:**
- Tasks 1-2 must complete before Tasks 3-5
- Tasks 3-5 must complete before Tasks 6-7
- Tasks 6-7 must complete before Task 8
- Task 8 must complete before Task 9
- Task 10 must complete before Tasks 11-15 (Go module needed)
- Tasks 11-15 are independent of each other
- Tasks 11-15 must complete before Task 16
- Task 16 must complete before Task 20
- Tasks 17-19 are independent of each other

---

## Verification Checklist

### Server-side
1. `make migrate-up` — migration applies cleanly
2. `make lint` — no lint errors
3. `make test-unit` — all tests pass
4. `make build` — compiles
5. Manual: Service account can call all 6 sync endpoints

### Agent
1. `make build` — compiles for current platform
2. `make build-windows` — cross-compiles for Windows
3. `make test` — all tests pass
4. `make lint` — no lint errors
5. Integration test with mock servers passes
6. Manual: Agent discovers Tally, syncs masters, imports voucher
