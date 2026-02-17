# Enhanced Duplicate Invoice Detection — Design

**Date**: 2026-02-17
**Status**: Approved

## Problem

The current `logic.invoice.duplicate` validator matches on seller GSTIN + invoice number only. This has two gaps:

1. **Cross-financial-year false positives**: Invoice numbers reset per FY. Two invoices with the same number from different FYs are not duplicates, but the current check flags them.
2. **Misses IRN-based guaranteed duplicates**: If two documents share the same IRN (a government-assigned SHA-256 hash), they are guaranteed duplicates regardless of other fields. The current check doesn't use IRN at all.

## Design: Tiered Duplicate Matching

Enhance the single `logic.invoice.duplicate` validator to use three match tiers in one query.

### Match Tiers

| Tier | Match Criteria | Confidence | Result Severity |
|------|---------------|------------|-----------------|
| `exact_irn` | Same IRN (64-char hex SHA-256) | Guaranteed | Error |
| `strong` | Seller GSTIN + Invoice Number + same Financial Year | Very high | Error |
| `weak` | Seller GSTIN + Invoice Number (cross-FY) | Possible | Warning |

If any `exact_irn` or `strong` match exists, the validator returns an error-severity result. If only `weak` matches exist, it returns a warning.

### Changes

**1. `port/duplicate_finder.go` — DTO and interface**

`DuplicateMatch` gains:
- `MatchType string` — `"exact_irn"`, `"strong"`, or `"weak"`
- `DocumentID uuid.UUID` — for frontend linking

`DuplicateInvoiceFinder.FindDuplicates` signature becomes:
```go
FindDuplicates(ctx, tenantID, excludeDocID, sellerGSTIN, invoiceNumber, invoiceDate, irn string) ([]DuplicateMatch, error)
```

Two new parameters: `invoiceDate` (for FY derivation) and `irn` (for exact match).

**2. `repository/postgres/duplicate_finder_repo.go` — SQL query**

Single query with three match tiers using `CASE WHEN` or `UNION ALL`:
- Tier 1: `structured_data->'invoice'->>'irn' = $irn` (only when IRN is non-empty)
- Tier 2: seller GSTIN + invoice number + invoice date within same FY
- Tier 3: seller GSTIN + invoice number only, excluding docs already matched in tiers 1-2

Financial year logic in SQL: if invoice month >= 4 (April), FY = year; else FY = year - 1. Compare against the current document's FY.

Order by match tier (strongest first), then `created_at DESC`. Limit 10.

**3. `validator/invoice/duplicate.go` — validator logic**

- Extract `inv.Invoice.IRN` and `inv.Invoice.InvoiceDate` in addition to existing fields
- Pass all fields to `FindDuplicates`
- Group results by match type
- If any `exact_irn` or `strong` match: produce error-severity result
- If only `weak` matches: produce warning-severity result
- Message includes match type context for each duplicate

The validator itself keeps `domain.ValidationSeverityWarning` as its base severity (for rule seeding), but the result message and `ActualValue` encode the effective severity so the engine and frontend can distinguish.

**4. No migration needed**

All data lives in JSONB `structured_data`. The query reads `structured_data->'invoice'->>'irn'` and `structured_data->'invoice'->>'invoice_date'` which are already populated by parsing.

**5. Financial year derivation**

Reuse the existing `DeriveFinancialYear()` helper from `irn.go` in the validator. For the SQL query, derive FY inline using PostgreSQL date functions on `structured_data->'invoice'->>'invoice_date'`.

### Test Plan

- All 3 tiers: exact IRN match, strong match (same FY), weak match (cross-FY)
- Mixed tiers in one result set
- Empty IRN (skip tier 1)
- Empty invoice date (skip tier 2, fall back to weak)
- Empty seller GSTIN or invoice number (skip entirely, as before)
- Missing validation context (skip, as before)
- Finder error (graceful degradation, as before)
- Severity escalation: error when strong/IRN match present, warning when only weak
- Metadata assertions (rule key, name, type unchanged)
- Builtin count unchanged (still 56)
- No key conflict with other validators
- Mock still works with new interface signature

### Frontend Integration

The existing `GET /documents/:id/validation` endpoint already returns per-rule results. The duplicate validator's result message will include match type and document names. The frontend reads this from the validation response and renders:

- Error-severity duplicates: red banner with link to duplicate document(s)
- Warning-severity duplicates: yellow banner with "possible duplicate" language

A React + TypeScript integration guide will be provided.
