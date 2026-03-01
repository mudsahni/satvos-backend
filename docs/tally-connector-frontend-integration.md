# Tally Connector — Frontend Integration Guide

## What This Feature Does

The Tally Connector enables bidirectional sync between SATVOS and the customer's on-premise Tally Prime installation. It consists of two parts:

1. **Server-side Sync API** — new endpoints on the SATVOS server (behind service account auth) that the on-premise agent communicates with. These are NOT called by the frontend directly.
2. **On-premise Connector Agent** — a Go binary that runs on the customer's Windows machine. It has its own local web UI at `http://localhost:8321`. The customer interacts with this directly — not through the SATVOS website.

**The frontend's role** is to let admins set up and monitor the connector, view synced Tally master data, and see sync status on documents. The frontend does NOT interact with the sync protocol itself.

---

## Feature 1: Connector Agent Setup & Monitoring

### What the Admin Needs to Do

To connect Tally, the admin needs to:
1. Create a **service account** (already supported: `POST /api/v1/service-accounts`)
2. Grant it **permissions** on the collections they want to sync
3. Download and install the connector agent on the Windows machine where Tally runs
4. Enter the service account's API key into the agent's config

### Data Available for the Dashboard

**Connector Agent status** is available via `GET /api/v1/admin/connectors`. The data model:

```json
{
  "id": "uuid",
  "tenant_id": "uuid",
  "service_account_id": "uuid",
  "agent_version": "0.1.0",
  "tally_company": "My Company Pvt Ltd",
  "tally_port": 9000,
  "os_info": "windows/amd64",
  "status": "online",           // "registered" | "online" | "offline" | "disconnected"
  "last_heartbeat": "2026-02-28T10:30:00Z",
  "registered_at": "2026-02-28T09:00:00Z"
}
```

**Status meanings:**
- `registered` — Agent registered but hasn't sent a heartbeat yet (just installed)
- `online` — Agent is running AND Tally is reachable on the customer's machine
- `offline` — Agent is running but Tally is NOT reachable (Tally might be closed)
- `disconnected` — No heartbeat received recently (agent may be stopped)

**Endpoint:** `GET /api/v1/admin/connectors` (admin-only, JWT auth)

Returns an array of connector agents for the tenant. Unpaginated (few agents per tenant).

```bash
curl -H "Authorization: Bearer <admin-jwt>" \
  https://your-server/api/v1/admin/connectors
```

Response:
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "tenant_id": "uuid",
      "service_account_id": "uuid",
      "agent_version": "0.1.0",
      "tally_company": "My Company Pvt Ltd",
      "tally_port": 9000,
      "os_info": "windows/amd64",
      "status": "online",
      "last_heartbeat": "2026-02-28T10:30:00Z",
      "registered_at": "2026-02-28T09:00:00Z"
    }
  ]
}
```

**Heartbeat interval:** The agent sends heartbeats every 30 seconds (configurable). If `last_heartbeat` is older than ~2 minutes, the agent is likely down.

---

## Feature 2: Synced Tally Masters (Read-Only Reference Data)

The connector agent pushes the customer's Tally master data to SATVOS every sync cycle. This data is stored per-tenant and is used internally for smart voucher matching, but it's also useful for the frontend as read-only reference data.

### Five Master Types

#### 1. Ledgers (`tally_ledgers`)
```json
{
  "id": "uuid",
  "name": "HDFC Bank",
  "parent_group": "Bank Accounts",
  "gstin": "27AABCH1234A1Z5",
  "state": "Maharashtra",
  "tax_type": "CGST",         // "CGST" | "SGST" | "IGST" | "" (non-tax ledger)
  "tax_rate": 9.0,
  "is_revenue": false,
  "synced_at": "2026-02-28T10:30:00Z"
}
```
Ledgers are the most important master type. They include party accounts (suppliers/customers with GSTIN), tax accounts (CGST/SGST/IGST at specific rates), bank accounts, purchase accounts, and expense accounts.

#### 2. Stock Items (`tally_stock_items`)
```json
{
  "id": "uuid",
  "name": "Steel Rod 12mm TMT",
  "parent_group": "Raw Materials",
  "hsn_code": "72142000",
  "default_uom": "Kgs",
  "synced_at": "2026-02-28T10:30:00Z"
}
```

#### 3. Godowns / Locations (`tally_godowns`)
```json
{
  "id": "uuid",
  "name": "Main Warehouse",
  "parent": "Main Location",
  "synced_at": "2026-02-28T10:30:00Z"
}
```

#### 4. Units of Measure (`tally_units`)
```json
{
  "id": "uuid",
  "symbol": "Kgs",
  "formal_name": "Kilograms",
  "synced_at": "2026-02-28T10:30:00Z"
}
```

#### 5. Cost Centres (`tally_cost_centres`)
```json
{
  "id": "uuid",
  "name": "Head Office",
  "parent": "",
  "synced_at": "2026-02-28T10:30:00Z"
}
```

### Endpoints (all admin-only, JWT auth)

#### Ledgers — `GET /api/v1/admin/tally-masters/ledgers`

Paginated. Supports filters:

| Query Param | Type | Description |
|---|---|---|
| `parent_group` | string | Filter by parent group (e.g., "Duties & Taxes", "Sundry Creditors") |
| `tax_type` | string | Filter by tax type ("CGST", "SGST", "IGST") |
| `search` | string | Search by name or GSTIN (case-insensitive) |
| `offset` | int | Pagination offset (default 0) |
| `limit` | int | Page size, max 100 (default 20) |

```bash
curl -H "Authorization: Bearer <admin-jwt>" \
  "https://your-server/api/v1/admin/tally-masters/ledgers?search=CGST&limit=5"
```

Response includes `meta` with `total`, `offset`, `limit`.

#### Stock Items — `GET /api/v1/admin/tally-masters/stock-items`

Paginated. Supports filters:

| Query Param | Type | Description |
|---|---|---|
| `parent_group` | string | Filter by parent group |
| `hsn_code` | string | Filter by exact HSN code |
| `search` | string | Search by name (case-insensitive) |
| `offset` | int | Pagination offset (default 0) |
| `limit` | int | Page size, max 100 (default 20) |

#### Godowns — `GET /api/v1/admin/tally-masters/godowns`

Unpaginated. Returns full list (typically < 50 entries).

#### Units — `GET /api/v1/admin/tally-masters/units`

Unpaginated. Returns full list.

#### Cost Centres — `GET /api/v1/admin/tally-masters/cost-centres`

Unpaginated. Returns full list.

**Note:** Ledgers typically have hundreds of entries for a real company. Stock items can have thousands. These two are paginated. Godowns, units, and cost centres are usually < 50 entries each.

---

## Feature 3: Sync Events (Audit Trail for Sync)

Every document that gets pushed to Tally (or pulled from Tally) creates a sync event. This provides a per-document sync history.

```json
{
  "id": "uuid",
  "agent_id": "uuid",
  "document_id": "uuid",
  "direction": "outbound",        // "outbound" = SATVOS → Tally, "inbound" = Tally → SATVOS
  "status": "success",            // "pending" | "success" | "failed" | "skipped"
  "tally_voucher_id": "abc123",   // Tally's internal master ID (populated on success)
  "tally_voucher_number": "PUR/2026/001",
  "error_message": "",
  "created_at": "2026-02-28T10:31:00Z"
}
```

**Status meanings:**
- `pending` — Document was dispatched to the agent but no ACK received yet
- `success` — Agent confirmed it imported the voucher into Tally successfully
- `failed` — Agent tried to import but Tally returned an error
- `skipped` — Not used currently; reserved for future selective sync

**Per-document sync status** can be inferred by querying sync events for a document. A document with a `success` outbound event has been pushed to Tally. A document with only `failed` events needs attention.

### Endpoint: `GET /api/v1/documents/:id/sync-events`

Paginated. Accessible to any authenticated user (same auth as document audit trail). Returns sync events for a specific document, ordered by `created_at DESC`.

| Query Param | Type | Description |
|---|---|---|
| `offset` | int | Pagination offset (default 0) |
| `limit` | int | Page size, max 100 (default 20) |

```bash
curl -H "Authorization: Bearer <jwt>" \
  "https://your-server/api/v1/documents/<doc-id>/sync-events"
```

Response:
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "agent_id": "uuid",
      "document_id": "uuid",
      "direction": "outbound",
      "status": "success",
      "tally_voucher_id": "abc123",
      "tally_voucher_number": "PUR/2026/001",
      "error_message": "",
      "created_at": "2026-02-28T10:31:00Z"
    }
  ],
  "meta": { "total": 1, "offset": 0, "limit": 20 }
}
```

Returns 404 if the connector feature is not available (nil sync repo).

**Recommendation:** Show sync event history on the document detail page (similar to the existing audit trail tab). Add a "Tally Sync" column or badge on the document list: "Synced" (green), "Pending" (yellow), "Failed" (red), "Not synced" (gray).

---

## Feature 4: Voucher Matching Preview (VoucherDef)

When a document is queued for outbound sync, the server builds a **VoucherDef** — a smart-matched mapping from the parsed invoice to Tally ledger/stock item names. This is returned as part of the outbound payload.

The VoucherDef includes match confidence metadata:

```json
{
  "document_id": "uuid",
  "voucher_type": "Purchase",
  "voucher_date": "2026-01-15",
  "party_ledger": "HDFC Bank",
  "purchase_ledger": "Purchase Accounts",
  "tax_entries": [
    { "ledger_name": "Input CGST @9%", "amount": 900.0 },
    { "ledger_name": "Input SGST @9%", "amount": 900.0 }
  ],
  "inventory_items": [
    {
      "stock_item": "Steel Rod 12mm TMT",
      "quantity": 100,
      "rate": 50.0,
      "amount": 5000.0,
      "uom": "Kgs",
      "godown": "Main Warehouse",
      "hsn_code": "72142000"
    }
  ],
  "total_amount": 6800.0,
  "narration": "Supplier Name - INV-2026-001",
  "remote_id": "tenant-uuid-doc-uuid",
  "match_confidence": {
    "party_ledger": "exact_gstin",      // best — matched via GSTIN
    "purchase_ledger": "convention",     // used default "Purchase Accounts"
    "tax_cgst": "exact_rate",           // found exact ledger in Tally masters
    "tax_sgst": "convention",           // used convention-based name "Input SGST @9%"
    "item_0": "exact_hsn",             // matched stock item via HSN code
    "item_1": "description_fallback"   // no HSN match, used line item description
  }
}
```

**Match confidence levels (ordered best to worst):**

| Confidence | Meaning |
|---|---|
| `exact_gstin` | Party ledger matched by GSTIN — guaranteed correct |
| `exact_group` | Purchase ledger found by account group in Tally |
| `exact_rate` | Tax ledger found by type + rate in Tally |
| `exact_hsn` | Stock item found by HSN code in Tally |
| `description_fallback` | HSN not found, used invoice line item description as stock name |
| `no_hsn` | No HSN code on line item at all, used description or "Unknown Item" |
| `convention` | Used a convention-based name (e.g., "Input CGST @9%", "Purchase Accounts") — will auto-create in Tally if it doesn't exist |

**Recommendation:** Show this as a "Tally Sync Preview" on the document detail page. Color-code the confidence levels: green for `exact_*`, yellow for `description_fallback`/`convention`, red for `no_hsn`. This helps accountants catch mapping issues before the voucher gets imported into Tally.

### Endpoint: `GET /api/v1/documents/:id/voucher-preview`

Accessible to any authenticated user with collection access (enforced via `GetByID` permission check). Builds a VoucherDef on the fly by matching the parsed document against the tenant's Tally masters.

```bash
curl -H "Authorization: Bearer <jwt>" \
  "https://your-server/api/v1/documents/<doc-id>/voucher-preview"
```

**Requirements:**
- Document must have `parsing_status = completed`. Returns 400 otherwise.
- Returns 404 if the voucher builder is not available (no Tally connector configured).
- Returns 403 if the user lacks collection permission on the document.

Response:
```json
{
  "success": true,
  "data": {
    "document_id": "uuid",
    "voucher_type": "Purchase",
    "voucher_date": "2026-01-15",
    "party_ledger": "HDFC Bank",
    "purchase_ledger": "Purchase Accounts",
    "tax_entries": [...],
    "inventory_items": [...],
    "total_amount": 6800.0,
    "narration": "Supplier Name - INV-2026-001",
    "remote_id": "tenant-uuid-doc-uuid",
    "match_confidence": {
      "party_ledger": "exact_gstin",
      "purchase_ledger": "convention",
      ...
    }
  }
}
```

---

## Feature 5: Inbound Tally Vouchers (Reconciliation Data)

The connector can also push vouchers FROM Tally TO SATVOS (inbound direction). These are stored in `tally_vouchers` and can be used for reconciliation.

```json
{
  "id": "uuid",
  "voucher_type": "Purchase",
  "voucher_number": "PUR/2026/001",
  "voucher_date": "2026-01-15",
  "party_name": "Supplier Pvt Ltd",
  "party_gstin": "27AABCS1234A1Z5",
  "amount": 6800.0,
  "narration": "Purchase from Supplier",
  "ledger_entries": [{"name": "Purchase", "amount": 5000}, ...],
  "remote_id": "tenant-uuid-doc-uuid",
  "tally_master_id": "unique-tally-id",
  "synced_at": "2026-02-28T10:32:00Z"
}
```

This is a future building block for GSTR-2A/2B reconciliation — matching inbound Tally purchase vouchers against SATVOS-parsed invoices.

### Endpoints (admin-only, JWT auth)

#### List Vouchers — `GET /api/v1/admin/tally-vouchers`

Paginated. Supports filters:

| Query Param | Type | Description |
|---|---|---|
| `voucher_type` | string | Filter by type (e.g., "Purchase", "Sales") |
| `party_gstin` | string | Filter by party GSTIN |
| `from` | string | Start date filter, `YYYY-MM-DD` format |
| `to` | string | End date filter, `YYYY-MM-DD` format |
| `offset` | int | Pagination offset (default 0) |
| `limit` | int | Page size, max 100 (default 20) |

```bash
curl -H "Authorization: Bearer <admin-jwt>" \
  "https://your-server/api/v1/admin/tally-vouchers?voucher_type=Purchase&from=2025-01-01&to=2025-12-31"
```

Returns 400 if `from` or `to` is not a valid `YYYY-MM-DD` date.

#### Get Voucher — `GET /api/v1/admin/tally-vouchers/:id`

Returns a single voucher by ID. Returns 404 if not found, 400 if ID is not a valid UUID.

---

## Endpoint Reference (All Implemented)

None of the sync protocol endpoints (`/sync/v1/*`) are for the frontend. Those are for the on-premise agent only (service account auth, `RoleService` role).

The following read-only endpoints are available for the frontend:

| # | Endpoint | Auth | Paginated | Purpose |
|---|---|---|---|---|
| 1 | `GET /api/v1/admin/connectors` | Admin | No | List connector agents with status |
| 2 | `GET /api/v1/admin/tally-masters/ledgers` | Admin | Yes | Browse synced Tally ledgers (filterable) |
| 3 | `GET /api/v1/admin/tally-masters/stock-items` | Admin | Yes | Browse synced stock items (filterable) |
| 4 | `GET /api/v1/admin/tally-masters/godowns` | Admin | No | List godowns |
| 5 | `GET /api/v1/admin/tally-masters/units` | Admin | No | List units of measure |
| 6 | `GET /api/v1/admin/tally-masters/cost-centres` | Admin | No | List cost centres |
| 7 | `GET /api/v1/admin/tally-vouchers` | Admin | Yes | Browse inbound Tally vouchers (filterable) |
| 8 | `GET /api/v1/admin/tally-vouchers/:id` | Admin | No | Get single Tally voucher |
| 9 | `GET /api/v1/documents/:id/sync-events` | Any authed | Yes | Sync history for a document |
| 10 | `GET /api/v1/documents/:id/voucher-preview` | Any authed | No | VoucherDef preview with match confidence |

**Common patterns:**
- Admin endpoints (#1-8) are under `/admin/` and require `RoleAdmin`. They follow the same nil-safe pattern as email config — return 404 `"NOT_AVAILABLE"` if the connector repos aren't initialized.
- Document endpoints (#9-10) are under `/documents/:id/` and are accessible to any authenticated user. #9 follows the audit trail pattern (tenant-scoped). #10 enforces collection permissions via `GetByID`.
- Paginated endpoints accept `offset` and `limit` query params (default 0/20, max 100) and return a `meta` object with `total`, `offset`, `limit`.
- All responses use the standard envelope: `{"success": bool, "data": ..., "error": ..., "meta": ...}`

---

## Suggested Frontend Features (In Priority Order)

### P0 — Connector Setup Flow

**Goal:** Guide an admin through connecting their Tally instance.

**Steps the admin needs to take:**
1. Create a service account (existing UI — `POST /api/v1/service-accounts`)
2. Grant the service account editor or owner permission on the collections to sync
3. Copy the API key
4. Download the connector installer (a `.exe` or `.zip` — we can host this or provide a download link)
5. Run the installer on the Windows machine where Tally is installed
6. Enter the API key and SATVOS server URL in the connector's setup page (`http://localhost:8321/setup.html`)
7. Verify connection — connector registers, heartbeat goes green

**What the frontend shows:** A setup wizard/checklist that walks through steps 1-3 and provides download/instructions for steps 4-7. After setup, the connector status should appear on the admin dashboard.

### P1 — Connector Status Dashboard

**Goal:** Show admin whether the connector is healthy.

**Endpoint:** `GET /api/v1/admin/connectors`

**Key data points:**
- Agent status (online/offline/disconnected)
- Tally company name (reported by agent)
- Agent version
- Last heartbeat timestamp (show as "X minutes ago")
- OS info
- Number of synced masters (call the tally-masters endpoints and count)
- Sync activity summary (recent sync events: success/fail counts)

### P2 — Document Sync Status

**Goal:** Show whether a document has been synced to Tally on the document list and detail pages.

**The `sync_status` field** is included directly on the Document object — no extra API calls needed. It's returned by all document list/get endpoints (`GET /documents`, `GET /documents/:id`, etc.).

```json
{
  "id": "uuid",
  "sync_status": "synced",
  ...
}
```

**Values:**
| Value | Badge | Meaning |
|---|---|---|
| `not_synced` | Gray | No sync attempted — connector may not be set up, or document not yet picked up |
| `pending` | Yellow | Sync event created, waiting for agent ACK |
| `synced` | Green | Successfully imported into Tally |
| `failed` | Red | Agent reported an error importing into Tally |

The field is maintained automatically by the backend when sync events are created/acknowledged. No frontend writes needed.

**Additional endpoints for the document detail page:**
- `GET /api/v1/documents/:id/sync-events` — paginated sync event history (same UX as the audit trail tab)
- `GET /api/v1/documents/:id/voucher-preview` — VoucherDef with match confidence (color-code confidence levels)

**On the document list page:**
- Use `sync_status` directly from the document object for the badge — no N+1 calls needed

**On the document detail page:**
- Show sync event history tab (call `sync-events` endpoint)
- Show voucher preview tab (call `voucher-preview` endpoint)
- Show the Tally voucher number if successfully synced (from the `success` sync event in the history)

### P3 — Tally Masters Browser

**Goal:** Let admins browse what Tally data has been synced.

**Endpoints:**
- `GET /api/v1/admin/tally-masters/ledgers?parent_group=...&tax_type=...&search=...`
- `GET /api/v1/admin/tally-masters/stock-items?parent_group=...&hsn_code=...&search=...`
- `GET /api/v1/admin/tally-masters/godowns`
- `GET /api/v1/admin/tally-masters/units`
- `GET /api/v1/admin/tally-masters/cost-centres`

**Key views:**
- Ledger list with filters by parent group, tax type, search by name/GSTIN
- Stock item list with filters by parent group, search by name/HSN
- Summary counts (use the `total` from paginated responses for ledgers/stock items, array length for unpaginated)
- Last synced timestamp (each record has a `synced_at` field)

### P4 — Tally Voucher Browser & Reconciliation

**Goal:** Future — browse inbound Tally vouchers and reconcile against SATVOS documents.

**Endpoints:**
- `GET /api/v1/admin/tally-vouchers?voucher_type=...&party_gstin=...&from=...&to=...`
- `GET /api/v1/admin/tally-vouchers/:id`

---

## Manual Steps You (Backend/Infra) Need to Take

### 1. Apply the Database Migration

```bash
cd /path/to/satvos
make migrate-up
```

This runs migration 000027 which creates the 8 new tables: `connector_agents`, `tally_ledgers`, `tally_stock_items`, `tally_godowns`, `tally_units`, `tally_cost_centres`, `sync_events`, `tally_vouchers`.

### 2. Create a Service Account for Testing

Using the admin UI or API:

```bash
# Create a service account
curl -X POST https://your-satvos-server/api/v1/service-accounts \
  -H "Authorization: Bearer <admin-jwt>" \
  -H "Content-Type: application/json" \
  -d '{"name": "tally-connector-test"}'
# → Save the API key from the response (sk_...)

# Grant it permission on a test collection
curl -X POST https://your-satvos-server/api/v1/service-accounts/<sa-id>/permissions \
  -H "Authorization: Bearer <admin-jwt>" \
  -H "Content-Type: application/json" \
  -d '{"collection_id": "<collection-uuid>", "permission": "editor"}'
```

### 3. Build the Connector Agent

```bash
cd /path/to/satvos-tally-connector

# For Windows (the target platform):
GOOS=windows GOARCH=amd64 go build -o bin/satvos-connector.exe ./cmd/connector

# For local testing on Linux:
go build -o bin/satvos-connector ./cmd/connector
```

### 4. Configure the Connector Agent

Create a config file at `./configs/connector.yaml` (or `~/.satvos-connector/connector.yaml`):

```yaml
satvos:
  base_url: "https://your-satvos-server.com"   # or http://localhost:8080 for local
  api_key: "sk_your_service_account_key_here"

tally:
  host: "localhost"
  port: 0          # 0 = auto-discover (scans 9000-9010)
  company: ""      # empty = auto-detect from Tally

sync:
  interval_seconds: 30
  batch_size: 50
  retry_attempts: 3

ui:
  port: 8321
```

Or use environment variables:
```bash
export CONNECTOR_SATVOS_API_KEY="sk_..."
export CONNECTOR_SATVOS_BASE_URL="http://localhost:8080"
```

### 5. Start Tally Prime

Make sure Tally Prime is running on the same machine with its XMLAPI enabled (Tally default: port 9000). To enable the XML API in Tally:
- Open Tally Prime
- Go to: F12 (Configure) → Advanced Configuration
- Set "Enable ODBC Server" to **Yes**
- The port is typically 9000

### 6. Run the Connector Agent

```bash
./bin/satvos-connector
```

You should see:
```
SATVOS Tally Connector v0.1.0 starting...
Using cached Tally port: 9000     (or "Discovered Tally on port 9000")
Registered as agent <uuid>
Sync engine running (interval: 30s). UI at http://localhost:8321
```

### 7. Verify the Sync

1. Open `http://localhost:8321` — you should see the connector dashboard showing online status
2. Verify via the admin API:
   ```bash
   # Check connector agent status
   curl -H "Authorization: Bearer <admin-jwt>" \
     http://localhost:8080/api/v1/admin/connectors
   # → Should show status = 'online', tally_company populated

   # Check synced ledgers
   curl -H "Authorization: Bearer <admin-jwt>" \
     "http://localhost:8080/api/v1/admin/tally-masters/ledgers?limit=5"
   # → Should show synced ledgers with total count in meta

   # Check synced stock items
   curl -H "Authorization: Bearer <admin-jwt>" \
     "http://localhost:8080/api/v1/admin/tally-masters/stock-items?limit=5"
   ```
3. Upload and parse a document in SATVOS (make sure it's in a collection the service account has access to)
4. Preview the voucher mapping before sync:
   ```bash
   curl -H "Authorization: Bearer <admin-jwt>" \
     http://localhost:8080/api/v1/documents/<doc-id>/voucher-preview
   # → Should show VoucherDef with match_confidence
   ```
5. Wait for the next sync cycle (30 seconds) — the connector should pick up the document and import it as a voucher into Tally
6. Check Tally for the new purchase voucher
7. Verify sync events:
   ```bash
   curl -H "Authorization: Bearer <admin-jwt>" \
     http://localhost:8080/api/v1/documents/<doc-id>/sync-events
   # → Should show outbound events with status = 'success'
   ```

### 8. Test Without Tally (Dry Run)

If you don't have Tally installed, you can still test the server-side sync API and master storage:

```bash
# Register agent
curl -X POST http://localhost:8080/api/v1/sync/v1/register \
  -H "Authorization: Bearer sk_your_key" \
  -H "Content-Type: application/json" \
  -d '{"version": "0.1.0", "os_info": "test"}'

# Send heartbeat
curl -X POST http://localhost:8080/api/v1/sync/v1/heartbeat \
  -H "Authorization: Bearer sk_your_key" \
  -H "Content-Type: application/json" \
  -d '{"tally_connected": true, "tally_company": "Test Co", "tally_port": 9000, "version": "0.1.0"}'

# Push some test master data
curl -X POST http://localhost:8080/api/v1/sync/v1/masters \
  -H "Authorization: Bearer sk_your_key" \
  -H "Content-Type: application/json" \
  -d '{
    "ledgers": [
      {"name": "Supplier Pvt Ltd", "parent_group": "Sundry Creditors", "gstin": "27AABCS1234A1Z5"},
      {"name": "Input CGST @9%", "parent_group": "Duties & Taxes", "tax_type": "CGST", "tax_rate": 9.0},
      {"name": "Purchase Accounts", "parent_group": "Purchase Accounts"}
    ],
    "stock_items": [
      {"name": "Widget A", "hsn_code": "84719000", "default_uom": "Nos"}
    ],
    "units": [
      {"symbol": "Nos", "formal_name": "Numbers"}
    ]
  }'

# Pull outbound documents (will be empty if no parsed docs exist)
curl -X POST http://localhost:8080/api/v1/sync/v1/outbound \
  -H "Authorization: Bearer sk_your_key" \
  -H "Content-Type: application/json"
```

Then verify via the frontend-facing admin endpoints:

```bash
# Check connector status (as admin)
curl -H "Authorization: Bearer <admin-jwt>" \
  http://localhost:8080/api/v1/admin/connectors

# Browse synced masters (as admin)
curl -H "Authorization: Bearer <admin-jwt>" \
  "http://localhost:8080/api/v1/admin/tally-masters/ledgers?search=CGST"

curl -H "Authorization: Bearer <admin-jwt>" \
  http://localhost:8080/api/v1/admin/tally-masters/units
```

### 9. Windows Service Installation (Production)

For production deployment on a customer's Windows machine:

```powershell
# Run PowerShell as Administrator
.\scripts\install.ps1
```

This registers `satvos-connector.exe` as a Windows Service that starts automatically on boot.

---

## Important Notes

- **The Sync API endpoints (`/sync/v1/*`) are for the on-premise agent only.** They require `RoleService` (service account auth). The frontend should never call these directly.
- **The connector agent's local UI (`http://localhost:8321`) is only accessible on the customer's machine** (bound to `127.0.0.1`). The frontend cannot embed or proxy it.
- **Master data is per-tenant.** Each tenant's Tally masters are completely isolated.
- **Smart matching is automatic.** The VoucherBuilder matches invoices to Tally masters using GSTIN, HSN codes, tax rates, and account groups. No manual mapping is needed for most cases.
- **Convention-based fallbacks auto-create in Tally.** If a ledger like "Input CGST @9%" doesn't exist in Tally, Tally Prime will auto-create it when the voucher is imported. This is by design.
- **One agent per tenant** is supported currently. Multi-agent support (multiple Tally companies) is a future extension.
- **Documents must be parsed and in an accessible collection** to appear in the outbound queue. The outbound query uses `NOT EXISTS` on sync_events to avoid re-syncing already-synced documents.
