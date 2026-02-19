# CLAUDE.md — Project Context for Claude Code

## Project Overview

SATVOS is a multi-tenant GST document processing service in Go (hexagonal architecture). JWT auth, 6-tier RBAC (admin/manager/member/viewer/free/service), AWS S3 storage, LLM-powered invoice parsing (multi-provider with dual-parse merge), 59-rule validation engine with reconciliation tiering, document tagging, document audit trail, financial reports (7 endpoints over materialized summaries), free-tier self-registration with quotas and email verification, password reset, Google social login, and API key-based service accounts for programmatic access.

## Key Commands

```bash
make run              # Start the server (go run ./cmd/server)
make build            # Compile to bin/server
make test             # Run all tests (go test ./... -v -count=1)
make test-unit        # Run unit tests only (tests/unit/)
make lint             # Run golangci-lint
make migrate-up       # Apply database migrations
make migrate-down     # Rollback one migration
make docker-up        # Start full stack via Docker Compose
make docker-down      # Stop Docker Compose stack
make seed-hsn         # Load HSN seed data into database
make backfill-summaries # Backfill document_summaries for existing docs
```

## Architecture & Code Layout

```
cmd/server/main.go           Entry point — wires config, DB, storage, services, validator engine, router
cmd/migrate/main.go          Migration CLI (up/down/steps/version)
cmd/seedhsn/main.go          One-time Excel→SQL conversion for HSN codes
cmd/backfill/main.go         Backfill document_summaries for existing parsed documents

internal/
  config/config.go           Loads env vars (SATVOS_ prefix) via viper
  domain/
    models.go                Tenant, User, FileMeta, Collection, Document, DocumentTag, DocumentValidationRule, DocumentAuditEntry, DocumentSummary, report row types
    enums.go                 All enums: FileType, UserRole, FileStatus, CollectionPermission, ParsingStatus,
                             ReviewStatus, ValidationStatus, ReconciliationStatus, ParseMode, AuthProvider, AuditAction, etc.
    errors.go                Sentinel errors (ErrNotFound, ErrForbidden, ErrQuotaExceeded, etc.)
  handler/
    auth_handler.go          login, refresh, register, verify-email, resend-verification, forgot/reset-password, social-login
    service_account_handler.go  CRUD /service-accounts, rotate-key, revoke, permissions (admin only)
    file_handler.go          upload, list, get, delete (free/service: own files only)
    collection_handler.go    CRUD, batch upload, permissions, CSV export, Tally XML export
    document_handler.go      CRUD, retry, review, assignment, review-queue, validation, tags, search, structured-data edit, audit trail
    user_handler.go          CRUD /users
    tenant_handler.go        CRUD /admin/tenants
    report_handler.go        7 report endpoints under /reports (sellers, buyers, party-ledger, financial/tax/hsn-summary, collections-overview)
    stats_handler.go         GET /stats (tenant-scoped, role-filtered)
    health_handler.go        GET /healthz, GET /readyz
    response.go              Standard envelope (success/data/error/meta) + error mapping
  middleware/
    auth.go                  JWT validation + API key auth + tenant/user/role injection, RequireEmailVerified
    cors.go                  CORS (SATVOS_CORS_ALLOWED_ORIGINS)
    tenant.go                Tenant context guard
    logger.go                Request ID, logging, panic recovery
  service/
    auth_service.go          Login (bcrypt), JWT generation/refresh, GenerateTokenPairForUser
    social_auth_service.go   Google social login (verify token, auto-link, auto-register)
    registration_service.go  Free-tier registration, email verification (VerifyEmail, ResendVerification)
    password_reset_service.go ForgotPassword, ResetPassword (JWT "password-reset" audience, 1h, single-use jti)
    file_service.go          Upload (validate + S3 + DB), download URL, delete, ListByUploader
    collection_service.go    CRUD, batch upload, permission checking, EffectivePermission(s)
    document_service.go      CRUD, background LLM parsing, retry, review, assignment, review-queue, validation, tags, quota enforcement, audit trail, summary upsert
    parse_queue_worker.go    Polls queued docs, bounded concurrency, graceful shutdown
    report_service.go        Thin pass-through to ReportRepository for 7 report queries
    stats_service.go         Aggregate stats (role-branching)
    user_service.go          User CRUD (tenant-scoped)
    tenant_service.go        Tenant CRUD
  port/
    repository.go            TenantRepo, UserRepo (CheckAndIncrementQuota, GetByProviderID, LinkProvider), FileMetaRepo interfaces
    social_auth.go           SocialTokenVerifier interface, SocialAuthClaims DTO
    collection_repository.go CollectionRepo, CollectionPermissionRepo, CollectionFileRepo interfaces
    document_repository.go   DocumentRepo (UpdateValidationResults, UpdateAssignment, ClaimQueued, ListReviewQueue), DocTagRepo, DocValidationRuleRepo
    document_audit_repository.go DocumentAuditRepository interface (Create, ListByDocument)
    document_summary_repository.go DocumentSummaryRepository interface (Upsert, UpdateStatuses)
    report_repository.go     ReportRepository interface (7 aggregation queries)
    stats_repository.go      StatsRepository interface
    email.go                 EmailSender interface (SendVerificationEmail, SendPasswordResetEmail)
    document_parser.go       DocumentParser interface (Parse) with ParseInput/ParseOutput DTOs
    hsn_repository.go        HSNRepository interface (LoadAll for in-memory cache)
    duplicate_finder.go      DuplicateInvoiceFinder interface
    storage.go               ObjectStorage interface (Upload, Download, Delete, GetPresignedURL)
  repository/postgres/       SQL implementations for all port interfaces
  email/
    ses/ses_sender.go        AWS SES v2 EmailSender implementation
    noop/noop_sender.go      No-op EmailSender (logs URL to stdout)
  csvexport/writer.go        CSV export (33 columns, UTF-8 BOM, batched)
  tallyexport/               Tally Prime XML export (purchase vouchers)
    voucher.go               Data model structs (Voucher, LedgerEntry, InventoryEntry, TallyExportOptions)
    template.go              text/template XML template + xmlEscape, parsed at init()
    convert.go               GSTInvoice → Voucher conversion, tax ledger grouping by rate
    writer.go                Writer (accumulate-then-flush), BuildFilename
  storage/s3/s3_client.go    S3 implementation (supports LocalStack)
  auth/google/verifier.go    Google ID token verification via tokeninfo endpoint
  parser/
    factory.go               Provider registry (RegisterProvider, NewParser)
    prompt.go                Shared GST invoice extraction prompt
    merge.go                 MergeParser — dual-parse, parallel, field-by-field merge
    fallback.go              FallbackParser — ordered failover with per-parser circuit breaker
    errors.go                RateLimitError type + ParseRetryAfterHeader
    claude/                  Anthropic Messages API parser
    gemini/                  Google Gemini REST API parser
    openai/                  OpenAI Chat Completions API parser
  validator/
    engine.go                Orchestrator: load rules, run validators, compute statuses, auto-seed builtins
    validator.go             Validator interface
    registry.go              Map-based validator registry
    field_status.go          Per-field status from rule results + confidence scores
    invoice/                 59 GST validators: required(12), format(13), math(11), crossfield(7),
                             logical(7), IRN(5), HSN(2), duplicate(1)
      types.go               GSTInvoice, Party, LineItem, Totals, Payment, ConfidenceScores
      builtin_rules.go       AllBuiltinValidators() collects all into BuiltinValidator wrappers
      context.go             WithValidationContext (injects tenantID, docID for data-dependent validators)
  router/router.go           Route definitions, middleware wiring
  mocks/                     Hand-written mocks for testing

tests/unit/                  Unit tests for all packages
db/migrations/               20 SQL migrations (tenants → users → files → collections → documents
                             → validation → reconciliation → multi-parser → tags → queue → hsn
                             → free-tier → email-verification → password-reset → social-auth → audit-log
                             → review-assignment → document-summaries)
```

## Data Flow

```
Request → Gin Router → Middleware (Logger → Auth → TenantGuard) → Handler → Service → Repository/Storage → PostgreSQL/S3
```

## Document Lifecycle

1. **Upload**: `POST /files/upload` → S3 + DB (optional `collection_id`)
2. **Create & Parse**: `POST /documents` → creates doc (pending) → background goroutine downloads from S3, sends to LLM, saves structured_data + confidence_scores + field_provenance, extracts auto-tags, upserts `document_summaries` row → completed/failed/queued
3. **Rate-limit retry**: If all parsers return 429, doc is queued with `retry_after`. `ParseQueueWorker` polls every 10s, re-dispatches with bounded concurrency (max 5 attempts)
4. **Validate**: Auto-triggered after parse. Engine auto-seeds builtin rules, runs 59 validators, computes `validation_status` and `reconciliation_status` independently, saves JSONB results
5. **Assign**: `PUT /documents/:id/assign` → soft-assign to a user for review (or unassign). Clears on retry. `GET /documents/review-queue` lists docs assigned to the caller that are parsed and pending review
6. **Review**: `PUT /documents/:id/review` → approve/reject with notes
7. **Manual edit**: `PUT /documents/:id` → validates JSON, sets confidence→1.0, resets review, re-extracts auto-tags, re-runs validation, re-upserts summary
8. **Audit trail**: Every mutation (create, parse, retry, review, edit, validate, assign, tags, delete) writes an append-only `document_audit_log` entry. `GET /documents/:id/audit` returns paginated history. Audit failures never block business logic

## Reports

- **Materialized summaries**: `document_summaries` table denormalizes parsed invoice data (seller/buyer, amounts, dates, statuses) for fast aggregation. Populated via non-blocking upsert hooks in document service (parse, edit, review). `ON DELETE CASCADE` from `documents`
- **7 endpoints** under `GET /api/v1/reports/`: `sellers`, `buyers`, `party-ledger`, `financial-summary`, `tax-summary`, `hsn-summary`, `collections-overview`
- **Viewer/free role scoping**: Queries filter by accessible collections via `collection_permissions` subquery
- **HSN report uses JSONB**: Queries `documents.structured_data` with `jsonb_array_elements` rather than the summary table (line items aren't denormalized)
- **Time-series granularity**: `daily`, `weekly`, `monthly` (default), `quarterly`, `yearly` — via PostgreSQL `date_trunc`
- **Backfill CLI**: `make backfill-summaries` (runs `cmd/backfill/main.go`) — populates summaries for existing parsed documents
- **Summary upsert is non-blocking**: Same pattern as audit trail — errors logged, never propagated. nil summaryRepo is safe (skipped)

## Validation Engine

- **59 rules**: 56 built-in (`AllBuiltinValidators()`) + 2 HSN (closure-captured in-memory lookup) + 1 duplicate (closure-captured `DuplicateInvoiceFinder` with JSONB `@>` query)
- **Auto-seeding**: `EnsureBuiltinRules()` creates missing rules per tenant, unique index prevents duplicates
- **Status logic**: Any error failure → invalid; only warnings → warning; all pass → valid
- **Reconciliation**: 22 rules marked `reconciliation_critical` for GSTR-2A/2B matching. Computed independently — non-critical failures don't affect `reconciliation_status`
- **Field status**: error failure → `invalid`; warning failure → `unsure`; confidence ≤ 0.5 → `unsure`; else → `valid`
- **Storage**: JSONB on `documents.validation_results` (not a separate table)
- **Context injection**: Engine calls `WithValidationContext(ctx, tenantID, docID)` so data-dependent validators can access them

### Validator Categories

| Category | Count | Key Prefix | File |
|----------|-------|------------|------|
| Required Fields | 12 | `req.*` | `invoice/required.go` |
| Format | 13 | `fmt.*` | `invoice/format.go` |
| Mathematical | 11 | `math.*` | `invoice/math.go` |
| Cross-field | 7 | `xf.*` | `invoice/crossfield.go` |
| Logical | 7 | `logic.*` | `invoice/logical.go` |
| IRN | 5 | `fmt.invoice.*`, `xf.invoice.*`, `logic.invoice.*` | `invoice/irn.go` |
| HSN | 2 | `logic.line_item.hsn_exists`, `xf.line_item.hsn_rate` | `invoice/hsn.go` |
| Duplicate | 1 | `logic.invoice.duplicate` | `invoice/duplicate.go` |

### Duplicate Detection Tiers

The `logic.invoice.duplicate` validator uses three match tiers:

| Tier | Match Criteria | Confidence | Effective Severity |
|------|---------------|------------|-------------------|
| `exact_irn` | Same IRN (64-char hex) | Guaranteed | Error |
| `strong` | Seller GSTIN + Invoice Number + same Financial Year | Very high | Error |
| `weak` | Seller GSTIN + Invoice Number (cross-FY) | Possible | Warning |

`FindDuplicates` accepts `sellerGSTIN`, `invoiceNumber`, `financialYear` (derived by validator via `DeriveFinancialYear()`), and `irn` (lowercased). Tier 2 JOINs `document_summaries.invoice_date` for FY matching. Results include `MatchType` and `DocumentID` for frontend linking.

## Multi-Parser Architecture

- **Providers**: Claude, Gemini, OpenAI — registered via `parser.RegisterProvider()` in `main.go`
- **FallbackParser**: Tries parsers in order; on 429, opens per-parser circuit breaker (skipped until `resetAt`). If all rate-limited, returns `RateLimitError` with earliest retry. Thread-safe via `sync.RWMutex`
- **MergeParser**: Wraps two FallbackParsers, runs in parallel, merges field-by-field. Agreement → boosted confidence; one empty → use non-empty; disagreement → prefer format-matching value. Line items: pick longer array
- **Parse modes**: `single` (FallbackParser [primary, secondary, tertiary]) or `dual` (MergeParser of two FallbackParsers)
- **Field provenance**: JSONB recording `"agree"`, `"primary"`, `"secondary"`, `"primary_format"`, `"secondary_format"`, `"disagreement"`, or `"manual_edit"`
- **Config**: `SATVOS_PARSER_{PRIMARY,SECONDARY,TERTIARY}_{PROVIDER,API_KEY,MODEL}`. Legacy flat fields still work

## Service Accounts

- **Purpose**: Non-human API identities for programmatic access (ERP integrations, batch processing, automation)
- **Auth**: API key-based (`sk_<64-hex-chars>`). Keys are SHA-256 hashed, prefix-indexed for lookup. `Authorization: Bearer sk_...`
- **Role**: `RoleService` (level 0, no implicit collection access). All access is via explicit `service_account_permissions` grants
- **Management**: Admin-only CRUD under `POST/GET/DELETE /api/v1/service-accounts`, plus `POST /:id/rotate-key`, `POST /:id/revoke`
- **Permissions**: `POST/GET/DELETE /api/v1/service-accounts/:id/permissions` — grants editor/viewer/owner on specific collections
- **Restrictions**: Cannot review documents (`PUT /documents/:id/review`), cannot be assigned documents (`PUT /documents/:id/assign`), cannot manage collection permissions (`SetPermission`/`ListPermissions`/`RemovePermission`), cannot manage users/tenants
- **Allowed**: Create collections (auto-granted owner via `service_account_permissions`), upload files, create/list/get documents, validate, manage tags, read reports/stats, export CSV/Tally (all scoped to permitted collections)
- **Auto-creation**: `EnsureInboundEmailServiceAccount` idempotently creates an `inbound_email` SA per tenant (called on tenant creation and free-tier startup). `createdBy=uuid.Nil` for system-created SAs
- **File isolation**: Like free tier — only sees files it uploaded
- **Key rotation**: `POST /service-accounts/:id/rotate-key` generates new key, invalidates old one
- **Expiry**: Optional `expires_at` on creation. Expired keys return `ErrAPIKeyRevoked`
- **Last used tracking**: `last_used_at` updated non-blocking on each API key authentication
- **Audit trail**: Service account ID recorded as `user_id` in audit entries — identifiable by `RoleService` role
- **DB tables**: `service_accounts` (keys, metadata), `service_account_permissions` (collection grants)
- **Migration**: `000025_create_service_accounts.up.sql`

## Key Conventions

- **Env config**: All `SATVOS_` prefixed. See `internal/config/config.go` for all vars and defaults
- **Response envelope**: `{"success": bool, "data": ..., "error": ..., "meta": ...}` from `handler/response.go`
- **Tenant isolation**: Every DB query includes `tenant_id` from JWT claims
- **Error mapping**: Domain errors → HTTP codes in `handler/response.go`
- **Tenant roles**: admin (level 4, implicit owner) > manager (3, editor) > member (2, viewer) > viewer (1, no implicit) > free (0, no implicit) > service (0, no implicit, separate permission table). Effective perm = `max(implicit, explicit)`. No cap — explicit owner grant on a viewer is respected. Service accounts use `service_account_permissions` table instead of `collection_permissions`. Helpers: `RoleLevel()`, `ImplicitCollectionPerm()`, `ServiceAccountCollectionPerm()`
- **Free tier**: Self-registration → shared "satvos" tenant, `free` role, personal collection (owner), per-user monthly quota (default 5). File listing filtered by uploader. Quota: `CheckAndIncrementQuota()` atomic SQL, 30-day period, `limit=0` → unlimited
- **Registration**: `POST /auth/register` → `RegistrationService` creates user + collection + tokens + sends verification email. Email failure doesn't fail registration. Disable by passing nil `RegistrationService` to `NewAuthHandler`
- **Email verification**: JWT `"email-verification"` audience, 24h expiry. `RequireEmailVerified` middleware checks DB for `free` role only. Gates: `POST /files/upload`, `POST /documents`. Config: `SATVOS_EMAIL_PROVIDER` ("ses"/"noop"), `SATVOS_EMAIL_FROM_ADDRESS`, `SATVOS_EMAIL_FRONTEND_URL`
- **Password reset**: `POST /auth/forgot-password` (always 200, no enumeration) → `POST /auth/reset-password` (single-use via `password_reset_token_id` jti). Does NOT invalidate existing tokens
- **Social login**: `POST /auth/social-login` (Google only). Frontend sends ID token → backend validates via `SocialTokenVerifier`. Auto-links if email matches existing user. Auto-verifies email. New users get personal collection. Config: `SATVOS_GOOGLE_AUTH_CLIENT_ID` (empty = disabled). Only for free-tier tenant. OAuth-only users (`password_hash=""`) blocked from password login (`ErrPasswordLoginNotAllowed`)
- **Collections**: Permission-based (owner/editor/viewer) + role hierarchy. `document_count` computed via SQL subquery. `EffectivePermissions` batch-optimized. `SetPermission` validates target user belongs to same tenant. `Create` accepts optional `owner_email` — if set, looks up user by email in the same tenant and grants them owner permission (non-blocking: user not found or upsert failure is logged and skipped, never fails creation)
- **CSV export**: `GET /collections/:id/export/csv` — 33 columns, reconciliation fields first, UTF-8 BOM, batched 200 docs
- **Tally XML export**: `GET /collections/:id/export/tally?company_name=<optional>` — Tally Prime purchase voucher XML. Converts parsed invoices to vouchers with inventory entries, tax ledger grouping by rate (CGST/SGST/IGST), convention-based ledger naming (`Purchase@{rate}%Gst`, `Input Cgst @{rate}%`). Uses `text/template` (not `encoding/xml`) due to Tally's non-standard element names. Batched 200 docs, accumulate-then-flush. `company_name` defaults to collection name. `REMOTEID` = `tenantID-docID` for idempotent reimport
- **Document parsing**: Background goroutine. `CreateAndParse`/`RetryParse` return **copies** to prevent data races. Status: pending → processing → completed/failed/queued
- **Document tags**: Key-value pairs with `source` (user/auto). Auto-tags extracted from parsed data, regenerated on retry/edit
- **Manual edit**: Validates JSON, sets confidence→1.0, resets review, re-extracts auto-tags, re-runs validation, sets provenance to `manual_edit`
- **Passwords**: bcrypt cost 12, min 8 chars. **JWT**: HS256, access 15m, refresh 7d
- **Review assignment**: `PUT /documents/:id/assign` soft-assigns a document to a user (`assigned_to`, `assigned_at`, `assigned_by`). Not a lock — anyone with access can still review. Assignee cannot approve/reject their own assignment (`ErrAssigneeCannotReview`). Retry clears assignment. `GET /documents/review-queue` returns docs assigned to the caller that are `parsing_status=completed` and `review_status=pending`. Document list endpoints accept `?assigned_to=<uuid>` filter
- **Audit trail**: Append-only `document_audit_log` table (no FK constraints — survives entity deletion). 13 actions covering every document mutation. `audit()` helper on service is nil-safe and non-blocking (errors logged, never returned). Handler reads audit repo directly (bypasses service) for deleted-document support. JSONB `changes` column stores action-specific metadata summaries (no full structured_data diffs). `document.validation_completed` emitted after every successful validation with `{validation_status, reconciliation_status, trigger}` where trigger is `"parse"`, `"edit"`, or `"manual"`. `document.assigned` emitted on assign/unassign with `{assigned_to, assigned_by}`
- **Reports**: 7 read-only aggregation endpoints under `/reports/`. `document_summaries` table is materialized (not computed on the fly). Summary upsert hooks run after parse/edit/review (non-blocking). Backfill CLI for existing data: `make backfill-summaries`
- **Testing**: testify + hand-written mocks in `/mocks/`. CI runs with `-race` flag

## Tech Stack

Go 1.24, Gin, PostgreSQL 16 (sqlx + pgx/v5), AWS S3 (aws-sdk-go-v2), AWS SES v2, JWT (golang-jwt/v5), bcrypt, Viper, golang-migrate, Docker/Compose, LocalStack, Claude/Gemini/OpenAI APIs

## Important Files for Common Tasks

- **Adding an endpoint**: `router/router.go` → handler in `handler/` → service in `service/`. Add `domain.RoleFree` to `RequireRole` if free role needs access
- **Adding a domain model**: `domain/models.go` → port in `port/` → repo in `repository/postgres/`
- **Adding a migration**: `db/migrations/` (sequential numbered SQL, up + down)
- **Modifying config**: `config/config.go` (struct + viper binding)
- **Adding a parser provider**: Implement `port.DocumentParser` in `parser/<provider>/`, register via `parser.RegisterProvider()` in `main.go`, use `parser.BuildGSTInvoicePrompt()`
- **Adding a validation rule**: Create in `validator/invoice/`, add to `*Validators()` function. Data-dependent validators use closure-capture pattern (see HSN/duplicate). Context available via `invoice.TenantIDFromContext(ctx)` / `DocumentIDFromContext(ctx)`
- **Modifying CSV columns**: `csvexport/writer.go` — `columns` slice + `documentToRow`
- **Modifying Tally export**: Template in `tallyexport/template.go`. Conversion logic in `tallyexport/convert.go`. Data model in `tallyexport/voucher.go`. Writer in `tallyexport/writer.go`. Handler in `handler/collection_handler.go` (`ExportTally`). Route in `router/router.go`
- **Modifying free tier**: Quota in `SATVOS_FREE_TIER_MONTHLY_LIMIT`. Registration in `service/registration_service.go`. Quota SQL in `repository/postgres/user_repo.go`. File isolation in `handler/file_handler.go`
- **Modifying email verification**: Service in `registration_service.go`. Middleware in `middleware/auth.go`. Sender in `port/email.go` → `email/ses/` or `email/noop/`
- **Modifying password reset**: Service in `service/password_reset_service.go`. Repo in `repository/postgres/user_repo.go`. Handler in `handler/auth_handler.go`
- **Adding a social login provider**: Implement `port.SocialTokenVerifier` in `auth/<provider>/`, register in `main.go` verifiers map, add `AuthProvider` const in `domain/enums.go`
- **Modifying service accounts**: Model in `domain/models.go` (`ServiceAccount`). Port in `port/service_account_repository.go`. Repo in `repository/postgres/service_account_repo.go`. Service in `service/service_account_service.go`. Handler in `handler/service_account_handler.go`. Routes in `router/router.go` (`service-accounts` group). Auth middleware in `middleware/auth.go` (API key path). Permission helper in `service/permission_helper.go` (`ServiceAccountCollectionPerm`)
- **Modifying audit trail**: Domain in `domain/enums.go` (`AuditAction` consts). Port in `port/document_audit_repository.go`. Repo in `repository/postgres/document_audit_repo.go`. Service helper in `document_service.go` (`audit()` method). Handler in `document_handler.go` (`ListAudit`). Add new actions: add const to `domain/enums.go`, add `s.audit(...)` call in service method
- **Modifying reports**: Domain row types in `domain/models.go`. Port in `port/report_repository.go`. Repo queries in `repository/postgres/report_repo.go`. Service in `service/report_service.go`. Handler in `handler/report_handler.go`. Routes in `router/router.go` (`reports` group). Summary table in `repository/postgres/document_summary_repo.go`. Backfill CLI in `cmd/backfill/main.go`

## Gotchas

- **Data races**: `CreateAndParse`/`RetryParse` copy the document struct before `go` — mock repos return same pointer, so both caller and goroutine would share memory without the copy
- **Range value copies**: LineItem (136B), DocumentValidationRule (208B) — use `&slice[i]`, enforced by `gocritic.rangeValCopy`
- **Import shadow**: In `document_service.go`, parser params must be `docParser`/`mergeDocParser` (not `parser`). Test files use `p` for mock parsers
- **Builtin shadowing**: Don't name params `max`, `min`, `len`, `cap` — caught by `gocritic.builtinShadow`
- **CI race detector**: Tests must be race-safe. `-race` flag in CI
- **Validation results**: JSONB on `documents` table, NOT a separate table (migrated in 007)
- **HSN loaded at startup**: In-memory map from `hsn_codes` table. Empty table = validators skip gracefully. Restart to reload
- **Free tier isolation**: Shared "satvos" tenant. Isolation via: (1) no implicit collection access, (2) file listing filtered by uploader, (3) explicit grants only
- **Quota period is 30 days**, not calendar month — reset date floats
- **Free role in RequireRole**: Must be explicitly added. Currently allowed on: `POST /files/upload`, `POST /documents`
- **Email verification does DB lookup per request** for free users (acceptable for free-tier volume)
- **Password reset doesn't invalidate sessions** — tokens expire naturally
- **`NewAuthHandler` takes 4 params**: `(authService, registrationService, passwordResetService, socialAuthService)` — any can be nil
- **Audit trail non-blocking**: `audit()` helper logs errors but never fails the parent operation. Audit repo is nil-safe (skipped when nil). No FK constraints on audit table — entries survive document/user/tenant deletion
- **`NewCollectionService` takes 6 params**: `(collectionRepo, collectionPermissionRepo, collectionFileRepo, fileService, userRepo, saPermRepo)` — saPermRepo is optional (nil-safe) for SA collection access
- **`NewTenantService` takes 2 params**: `(tenantRepo, serviceAccountService)` — saSvc is optional (nil-safe), triggers auto-creation of `inbound_email` SA on tenant creation
- **`NewDocumentHandler` takes 2 params**: `(documentService, auditRepo)` — auditRepo used for direct read in `ListAudit`
- **`NewDocumentService` takes 10 params**: `(docRepo, fileRepo, userRepo, permRepo, tagRepo, docParser, storage, validationEngine, auditRepo, summaryRepo)` — summaryRepo can be nil
- **`router.Setup` takes 14 params**: `(authSvc, authH, fileH, tenantH, userH, healthH, collectionH, documentH, statsH, reportH, serviceAccountH, corsOrigins, userRepo, saSvc)`
- **Summary upsert non-blocking**: Same pattern as audit — `upsertSummary`/`updateSummaryStatuses` log errors but never fail the parent operation. Nil summaryRepo is safe
- **HSN report queries JSONB directly**: The `hsn-summary` report uses `jsonb_array_elements` on `documents.structured_data` rather than the summary table, since line items aren't denormalized
- **ReportFilters is a pointer**: `*domain.ReportFilters` (120 bytes) — gocritic hugeParam lint requires pointer passing
- **Tally XML uses text/template**: Not `encoding/xml` — Tally element names like `ALLINVENTORYENTRIES.LIST` don't work with Go's xml struct tags. Template parsed at `init()`. Custom `xmlEscape` func handles `&<>"'` in dynamic values
- **Tally amount signs**: Party ledger = positive (credit), tax ledgers = negative (debit), inventory = negative (debit). `ISDEEMEDPOSITIVE` = `"Yes"` for debit entries
- **Duplicate detection uses document_summaries for FY matching**: Tier-2 (strong) matching JOINs `document_summaries` for the parsed `invoice_date` timestamp, avoiding JSONB date parsing in SQL. Summary must exist for tier-2 to match (non-blocking upsert means brief window where strong match may not fire for just-parsed docs)
- **Stale processing docs**: Server crash mid-parse → doc stuck in `processing` (no staleness detector yet)
