# Enhanced Duplicate Invoice Detection — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Upgrade duplicate invoice detection from a single-tier (seller GSTIN + invoice number) match to three-tier matching (IRN exact, strong FY-aware, weak cross-FY) with severity escalation.

**Architecture:** Enhance the existing `logic.invoice.duplicate` validator and its backing `DuplicateInvoiceFinder` interface/repo. The SQL query uses a single `UNION ALL` of three CTEs to find matches at different confidence tiers. The validator reads the tier from each match and escalates severity: IRN or strong matches produce error-level results, weak-only matches produce warnings.

**Tech Stack:** Go 1.24, PostgreSQL JSONB queries, testify, hand-written mocks

---

## Task 1: Update `DuplicateMatch` DTO and `DuplicateInvoiceFinder` interface

**Files:**
- Modify: `internal/port/duplicate_finder.go`

**Step 1: Update the DTO and interface**

Add `MatchType` and `DocumentID` fields to `DuplicateMatch`. Expand `FindDuplicates` with `invoiceDate` and `irn` parameters:

```go
package port

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// DuplicateMatch holds information about a matching document and the confidence tier.
type DuplicateMatch struct {
	DocumentID   uuid.UUID `db:"id"`
	DocumentName string    `db:"name"`
	MatchType    string    `db:"match_type"` // "exact_irn", "strong", or "weak"
	CreatedAt    time.Time `db:"created_at"`
}

// DuplicateInvoiceFinder checks for other documents that may be duplicates.
type DuplicateInvoiceFinder interface {
	FindDuplicates(ctx context.Context, tenantID, excludeDocID uuid.UUID,
		sellerGSTIN, invoiceNumber, invoiceDate, irn string) ([]DuplicateMatch, error)
}
```

**Step 2: Verify the project compiles (it won't — callers need updating)**

Run: `go build ./internal/port/...`
Expected: SUCCESS (port package compiles on its own)

**Step 3: Commit**

```
feat: expand DuplicateMatch DTO with match tiers and document ID
```

---

## Task 2: Update the SQL query for tiered matching

**Files:**
- Modify: `internal/repository/postgres/duplicate_finder_repo.go`

**Step 1: Rewrite `FindDuplicates` with the new signature and tiered query**

```go
package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"satvos/internal/port"
)

type duplicateFinderRepo struct {
	db *sqlx.DB
}

func NewDuplicateFinderRepo(db *sqlx.DB) port.DuplicateInvoiceFinder {
	return &duplicateFinderRepo{db: db}
}

func (r *duplicateFinderRepo) FindDuplicates(
	ctx context.Context,
	tenantID, excludeDocID uuid.UUID,
	sellerGSTIN, invoiceNumber, invoiceDate, irn string,
) ([]port.DuplicateMatch, error) {

	// Derive financial year from invoiceDate for tier-2 matching.
	// Indian FY: April (month 4) onwards = current year; before April = previous year.
	// If invoiceDate is empty or unparseable, we skip tier 2 (strong) matches.
	fy := deriveFY(invoiceDate)

	query, args := r.buildQuery(tenantID, excludeDocID, sellerGSTIN, invoiceNumber, fy, irn)

	var matches []port.DuplicateMatch
	if err := r.db.SelectContext(ctx, &matches, query, args...); err != nil {
		return nil, fmt.Errorf("duplicate_finder_repo.FindDuplicates: %w", err)
	}
	return matches, nil
}

// buildQuery constructs a tiered duplicate detection query.
// Tier 1 (exact_irn): same IRN — only when irn is non-empty.
// Tier 2 (strong): same seller GSTIN + invoice number + same financial year — only when fy is non-empty.
// Tier 3 (weak): same seller GSTIN + invoice number (any FY).
// Results are deduplicated: a doc matched by tier 1 won't appear in tier 2 or 3.
func (r *duplicateFinderRepo) buildQuery(
	tenantID, excludeDocID uuid.UUID,
	sellerGSTIN, invoiceNumber, fy, irn string,
) (string, []interface{}) {
	// We always have the base parameters.
	// $1 = tenantID, $2 = excludeDocID, $3 = sellerGSTIN, $4 = invoiceNumber
	args := []interface{}{tenantID, excludeDocID, sellerGSTIN, invoiceNumber}
	argIdx := 5

	baseWhere := `tenant_id = $1 AND id != $2 AND parsing_status = 'completed'`
	gstinInvMatch := `structured_data @> jsonb_build_object(
		'seller', jsonb_build_object('gstin', $3),
		'invoice', jsonb_build_object('invoice_number', $4)
	)`

	// Build CTEs conditionally
	var ctes []string
	var selects []string

	// Tier 1: exact IRN match
	if irn != "" {
		args = append(args, irn)
		irnArg := fmt.Sprintf("$%d", argIdx)
		argIdx++

		ctes = append(ctes, fmt.Sprintf(
			`tier1 AS (
				SELECT id, name, 'exact_irn'::text AS match_type, created_at
				FROM documents
				WHERE %s
				  AND structured_data->'invoice'->>'irn' = %s
			)`, baseWhere, irnArg))
		selects = append(selects, `SELECT * FROM tier1`)
	}

	// Tier 2: strong match (seller GSTIN + invoice number + same financial year)
	if fy != "" {
		args = append(args, fy)
		fyArg := fmt.Sprintf("$%d", argIdx)
		argIdx++

		// FY derivation in SQL: extract month/year from the stored invoice_date,
		// then compare the derived FY string.
		// We use a simpler approach: compute the FY string in Go, pass it,
		// and do the same derivation in SQL for the candidate docs.
		fyExpr := `CASE
			WHEN EXTRACT(MONTH FROM TO_DATE(
				REGEXP_REPLACE(structured_data->'invoice'->>'invoice_date', '(\d{2})[-/](\d{2})[-/](\d{4})', '\3-\2-\1'),
				'YYYY-MM-DD'
			)) >= 4
			THEN CONCAT(
				EXTRACT(YEAR FROM TO_DATE(REGEXP_REPLACE(structured_data->'invoice'->>'invoice_date', '(\d{2})[-/](\d{2})[-/](\d{4})', '\3-\2-\1'), 'YYYY-MM-DD'))::int,
				'-',
				LPAD(((EXTRACT(YEAR FROM TO_DATE(REGEXP_REPLACE(structured_data->'invoice'->>'invoice_date', '(\d{2})[-/](\d{2})[-/](\d{4})', '\3-\2-\1'), 'YYYY-MM-DD'))::int + 1) % 100)::text, 2, '0')
			)
			ELSE CONCAT(
				(EXTRACT(YEAR FROM TO_DATE(REGEXP_REPLACE(structured_data->'invoice'->>'invoice_date', '(\d{2})[-/](\d{2})[-/](\d{4})', '\3-\2-\1'), 'YYYY-MM-DD'))::int - 1),
				'-',
				LPAD((EXTRACT(YEAR FROM TO_DATE(REGEXP_REPLACE(structured_data->'invoice'->>'invoice_date', '(\d{2})[-/](\d{2})[-/](\d{4})', '\3-\2-\1'), 'YYYY-MM-DD'))::int % 100)::text, 2, '0')
			)
		END`

		// Exclude tier1 IDs if tier1 exists
		excludeTier1 := ""
		if irn != "" {
			excludeTier1 = ` AND id NOT IN (SELECT id FROM tier1)`
		}

		ctes = append(ctes, fmt.Sprintf(
			`tier2 AS (
				SELECT id, name, 'strong'::text AS match_type, created_at
				FROM documents
				WHERE %s AND %s%s
				  AND structured_data->'invoice'->>'invoice_date' IS NOT NULL
				  AND structured_data->'invoice'->>'invoice_date' != ''
				  AND (%s) = %s
			)`, baseWhere, gstinInvMatch, excludeTier1, fyExpr, fyArg))
		selects = append(selects, `SELECT * FROM tier2`)
	}

	// Tier 3: weak match (seller GSTIN + invoice number only, excluding higher tiers)
	excludeHigher := ""
	if irn != "" {
		excludeHigher += ` AND id NOT IN (SELECT id FROM tier1)`
	}
	if fy != "" {
		excludeHigher += ` AND id NOT IN (SELECT id FROM tier2)`
	}

	ctes = append(ctes, fmt.Sprintf(
		`tier3 AS (
			SELECT id, name, 'weak'::text AS match_type, created_at
			FROM documents
			WHERE %s AND %s%s
		)`, baseWhere, gstinInvMatch, excludeHigher))
	selects = append(selects, `SELECT * FROM tier3`)

	// Assemble the full query
	query := "WITH "
	for i, cte := range ctes {
		if i > 0 {
			query += ", "
		}
		query += cte
	}
	query += "\n"
	for i, sel := range selects {
		if i > 0 {
			query += " UNION ALL "
		}
		query += sel
	}
	query += `
		ORDER BY
			CASE match_type WHEN 'exact_irn' THEN 1 WHEN 'strong' THEN 2 ELSE 3 END,
			created_at DESC
		LIMIT 10`

	return query, args
}

// deriveFY extracts the Indian financial year from an invoice date string.
// Returns empty string if the date is empty or unparseable.
// Tries common formats: DD-MM-YYYY, DD/MM/YYYY, YYYY-MM-DD.
func deriveFY(invoiceDate string) string {
	if invoiceDate == "" {
		return ""
	}

	formats := []string{
		"02-01-2006", "02/01/2006",     // DD-MM-YYYY, DD/MM/YYYY
		"2006-01-02", "2006/01/02",     // YYYY-MM-DD, YYYY/MM/DD
		"01-02-2006", "01/02/2006",     // MM-DD-YYYY, MM/DD/YYYY
		"02-Jan-2006", "02 Jan 2006",   // DD-Mon-YYYY
		"January 2, 2006",              // Month D, YYYY
		"2 January 2006",               // D Month YYYY
	}

	for _, fmt := range formats {
		t, err := time.Parse(fmt, invoiceDate)
		if err == nil {
			year := t.Year()
			month := t.Month()
			if month >= 4 { // April onwards
				return fmt.Sprintf("%d-%02d", year, (year+1)%100)
			}
			return fmt.Sprintf("%d-%02d", year-1, year%100)
		}
	}
	return ""
}
```

**IMPORTANT:** The FY-in-SQL approach above is complex and fragile due to variable date formats in JSONB. A simpler, more robust alternative: **derive FY in Go for the current document, then do FY derivation in SQL only for candidate documents using the same REGEXP approach, or alternatively use the `document_summaries` table which has a parsed `invoice_date` timestamp column.** The implementer should consider using `document_summaries.invoice_date` for tier-2 matching since it's already a proper timestamp — this avoids all date parsing in SQL. The query for tier 2 would become:

```sql
tier2 AS (
    SELECT d.id, d.name, 'strong'::text AS match_type, d.created_at
    FROM documents d
    JOIN document_summaries ds ON ds.document_id = d.id
    WHERE d.tenant_id = $1 AND d.id != $2 AND d.parsing_status = 'completed'
      AND d.structured_data @> jsonb_build_object(
          'seller', jsonb_build_object('gstin', $3),
          'invoice', jsonb_build_object('invoice_number', $4)
      )
      AND ds.invoice_date IS NOT NULL
      AND CASE
          WHEN EXTRACT(MONTH FROM ds.invoice_date) >= 4
          THEN CONCAT(EXTRACT(YEAR FROM ds.invoice_date)::int, '-',
               LPAD(((EXTRACT(YEAR FROM ds.invoice_date)::int + 1) % 100)::text, 2, '0'))
          ELSE CONCAT((EXTRACT(YEAR FROM ds.invoice_date)::int - 1), '-',
               LPAD((EXTRACT(YEAR FROM ds.invoice_date)::int % 100)::text, 2, '0'))
          END = $5
      AND d.id NOT IN (SELECT id FROM tier1)
)
```

Use the `document_summaries` approach — it's cleaner and the summary table already exists for this purpose.

**Note on `deriveFY` in Go:** There's already `invoice.DeriveFinancialYear()` in `internal/validator/invoice/irn.go` that does this. However, it's in the `invoice` package which the `postgres` package shouldn't import. The repo needs its own lightweight `deriveFY` helper, or we pass the FY string from the validator (which already has access to `DeriveFinancialYear`). **Recommended approach: derive FY in the validator and pass it to `FindDuplicates` instead of passing raw `invoiceDate`.** This changes the interface signature:

```go
FindDuplicates(ctx context.Context, tenantID, excludeDocID uuid.UUID,
    sellerGSTIN, invoiceNumber, financialYear, irn string) ([]DuplicateMatch, error)
```

This is cleaner — the repo doesn't need to know about date parsing at all. The validator already has `DeriveFinancialYear()`.

**Step 2: Verify repo compiles**

Run: `go build ./internal/repository/postgres/...`
Expected: SUCCESS

**Step 3: Commit**

```
feat: tiered duplicate detection SQL with IRN, strong, and weak matching
```

---

## Task 3: Update the duplicate validator

**Files:**
- Modify: `internal/validator/invoice/duplicate.go`

**Step 1: Update the validator to pass FY and IRN, handle tiered results**

```go
package invoice

import (
	"context"
	"fmt"
	"strings"

	"satvos/internal/domain"
	"satvos/internal/port"
)

// DuplicateInvoiceValidator returns a validator that checks whether another document in
// the same tenant is a duplicate, using three match tiers:
//   - exact_irn: same IRN (guaranteed duplicate)
//   - strong: same seller GSTIN + invoice number + financial year (very likely duplicate)
//   - weak: same seller GSTIN + invoice number only (possible cross-FY duplicate)
func DuplicateInvoiceValidator(finder port.DuplicateInvoiceFinder) *BuiltinValidator {
	return &BuiltinValidator{
		key:      "logic.invoice.duplicate",
		name:     "Logical: Duplicate Invoice Detection",
		ruleType: domain.ValidationRuleCustom,
		sev:      domain.ValidationSeverityWarning,
		fn:       duplicateInvoiceValidator(finder),
	}
}

func duplicateInvoiceValidator(finder port.DuplicateInvoiceFinder) func(context.Context, *GSTInvoice) []ValidationResult {
	return func(ctx context.Context, inv *GSTInvoice) []ValidationResult {
		gstin := inv.Seller.GSTIN
		invoiceNum := inv.Invoice.InvoiceNumber

		if gstin == "" || invoiceNum == "" {
			return []ValidationResult{{
				Passed:    true,
				FieldPath: "invoice",
				Message:   "Logical: Duplicate Invoice Detection: seller GSTIN or invoice number is empty, skipping duplicate check",
			}}
		}

		tenantID, ok := TenantIDFromContext(ctx)
		if !ok {
			return []ValidationResult{{
				Passed:    true,
				FieldPath: "invoice",
				Message:   "Logical: Duplicate Invoice Detection: validation context missing, skipping duplicate check",
			}}
		}
		docID, ok := DocumentIDFromContext(ctx)
		if !ok {
			return []ValidationResult{{
				Passed:    true,
				FieldPath: "invoice",
				Message:   "Logical: Duplicate Invoice Detection: validation context missing, skipping duplicate check",
			}}
		}

		// Derive financial year for tier-2 (strong) matching.
		// Empty string if invoice date is missing or unparseable — tier 2 will be skipped.
		fy, _ := DeriveFinancialYear(inv.Invoice.InvoiceDate)

		irn := strings.ToLower(inv.Invoice.IRN)

		matches, err := finder.FindDuplicates(ctx, tenantID, docID, gstin, invoiceNum, fy, irn)
		if err != nil {
			return []ValidationResult{{
				Passed:    true,
				FieldPath: "invoice",
				Message:   "Logical: Duplicate Invoice Detection: duplicate check unavailable",
			}}
		}

		if len(matches) == 0 {
			return []ValidationResult{{
				Passed:        true,
				FieldPath:     "invoice",
				ExpectedValue: "no duplicate invoices",
				ActualValue:   "none found",
				Message:       "Logical: Duplicate Invoice Detection: no duplicate invoices found",
			}}
		}

		// Determine the highest confidence tier among matches.
		hasStrong := false
		for idx := range matches {
			if matches[idx].MatchType == "exact_irn" || matches[idx].MatchType == "strong" {
				hasStrong = true
				break
			}
		}

		// Build descriptive message.
		names := make([]string, 0, len(matches))
		for idx := range matches {
			m := &matches[idx]
			names = append(names, fmt.Sprintf("%q [%s] (uploaded %s)",
				m.DocumentName, m.MatchType, m.CreatedAt.Format("2006-01-02")))
		}

		severity := "warning"
		if hasStrong {
			severity = "error"
		}

		return []ValidationResult{{
			Passed:        false,
			FieldPath:     "invoice",
			ExpectedValue: "no duplicate invoices",
			ActualValue:   fmt.Sprintf("%d duplicate(s) found [%s]", len(matches), severity),
			Message: fmt.Sprintf(
				"Logical: Duplicate Invoice Detection: invoice %s from seller %s has %d duplicate(s): %s",
				invoiceNum, gstin, len(matches), strings.Join(names, ", "),
			),
		}}
	}
}
```

**Step 2: Verify package compiles**

Run: `go build ./internal/validator/invoice/...`
Expected: SUCCESS

**Step 3: Commit**

```
feat: upgrade duplicate validator with FY-aware and IRN-based tiered matching
```

---

## Task 4: Write comprehensive tests for the duplicate validator

**Files:**
- Modify: `tests/unit/validator/invoice/duplicate_test.go`

**Step 1: Rewrite the test file with comprehensive tier-based tests**

The hand-written mock needs updating for the new `FindDuplicates` signature. Then add tests for all tiers and edge cases:

```go
package invoice_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"satvos/internal/domain"
	"satvos/internal/port"
	"satvos/internal/validator/invoice"
)

// mockDuplicateFinder is a hand-written mock for port.DuplicateInvoiceFinder.
type mockDuplicateFinder struct {
	matches []port.DuplicateMatch
	err     error
	// Capture args for assertion.
	calledGSTIN     string
	calledInvNum    string
	calledFY        string
	calledIRN       string
}

func (m *mockDuplicateFinder) FindDuplicates(
	_ context.Context, _, _ uuid.UUID,
	sellerGSTIN, invoiceNumber, financialYear, irn string,
) ([]port.DuplicateMatch, error) {
	m.calledGSTIN = sellerGSTIN
	m.calledInvNum = invoiceNumber
	m.calledFY = financialYear
	m.calledIRN = irn
	return m.matches, m.err
}

// --- Context helper tests (unchanged) ---

func TestValidationContext_RoundTrip(t *testing.T) { /* ... existing ... */ }
func TestValidationContext_Missing(t *testing.T)   { /* ... existing ... */ }

// --- Tier-based duplicate validator tests ---

func TestDuplicateValidator_NoDuplicates(t *testing.T) {
	finder := &mockDuplicateFinder{matches: nil}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed)
	assert.Contains(t, results[0].Message, "no duplicate")
}

func TestDuplicateValidator_ExactIRNMatch(t *testing.T) {
	finder := &mockDuplicateFinder{
		matches: []port.DuplicateMatch{
			{DocumentID: uuid.New(), DocumentName: "Invoice-IRN.pdf", MatchType: "exact_irn",
				CreatedAt: time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)},
		},
	}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	inv.Invoice.IRN = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.False(t, results[0].Passed)
	assert.Contains(t, results[0].ActualValue, "error")
	assert.Contains(t, results[0].Message, "exact_irn")
	assert.Contains(t, results[0].Message, "Invoice-IRN.pdf")
}

func TestDuplicateValidator_StrongMatch(t *testing.T) {
	finder := &mockDuplicateFinder{
		matches: []port.DuplicateMatch{
			{DocumentID: uuid.New(), DocumentName: "Invoice-Strong.pdf", MatchType: "strong",
				CreatedAt: time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC)},
		},
	}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.False(t, results[0].Passed)
	assert.Contains(t, results[0].ActualValue, "error")
	assert.Contains(t, results[0].Message, "strong")
}

func TestDuplicateValidator_WeakMatchOnly(t *testing.T) {
	finder := &mockDuplicateFinder{
		matches: []port.DuplicateMatch{
			{DocumentID: uuid.New(), DocumentName: "Invoice-Weak.pdf", MatchType: "weak",
				CreatedAt: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		},
	}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.False(t, results[0].Passed)
	assert.Contains(t, results[0].ActualValue, "warning")
	assert.Contains(t, results[0].Message, "weak")
}

func TestDuplicateValidator_MixedTiers_EscalatesToError(t *testing.T) {
	finder := &mockDuplicateFinder{
		matches: []port.DuplicateMatch{
			{DocumentID: uuid.New(), DocumentName: "Invoice-Strong.pdf", MatchType: "strong",
				CreatedAt: time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)},
			{DocumentID: uuid.New(), DocumentName: "Invoice-Weak.pdf", MatchType: "weak",
				CreatedAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
		},
	}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.False(t, results[0].Passed)
	assert.Contains(t, results[0].ActualValue, "error")
	assert.Equal(t, "2 duplicate(s) found [error]", results[0].ActualValue)
}

func TestDuplicateValidator_PassesFYToFinder(t *testing.T) {
	finder := &mockDuplicateFinder{matches: nil}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	inv.Invoice.InvoiceDate = "15-06-2025" // June 2025 → FY 2025-26
	v.Validate(ctx, inv)
	assert.Equal(t, "2025-26", finder.calledFY)
}

func TestDuplicateValidator_PassesIRNToFinder(t *testing.T) {
	finder := &mockDuplicateFinder{matches: nil}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	inv.Invoice.IRN = "ABCD1234" // uppercase gets lowered
	v.Validate(ctx, inv)
	assert.Equal(t, "abcd1234", finder.calledIRN)
}

func TestDuplicateValidator_EmptyIRNPassedAsEmpty(t *testing.T) {
	finder := &mockDuplicateFinder{matches: nil}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	inv.Invoice.IRN = ""
	v.Validate(ctx, inv)
	assert.Equal(t, "", finder.calledIRN)
}

func TestDuplicateValidator_UnparseableDate_EmptyFY(t *testing.T) {
	finder := &mockDuplicateFinder{matches: nil}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	inv.Invoice.InvoiceDate = "not-a-date"
	v.Validate(ctx, inv)
	assert.Equal(t, "", finder.calledFY)
}

func TestDuplicateValidator_EmptyDate_EmptyFY(t *testing.T) {
	finder := &mockDuplicateFinder{matches: nil}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	inv.Invoice.InvoiceDate = ""
	v.Validate(ctx, inv)
	assert.Equal(t, "", finder.calledFY)
}

func TestDuplicateValidator_EmptySellerGSTIN(t *testing.T) {
	finder := &mockDuplicateFinder{}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	inv.Seller.GSTIN = ""
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed)
	assert.Contains(t, results[0].Message, "skipping")
}

func TestDuplicateValidator_EmptyInvoiceNumber(t *testing.T) {
	finder := &mockDuplicateFinder{}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	inv.Invoice.InvoiceNumber = ""
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed)
	assert.Contains(t, results[0].Message, "skipping")
}

func TestDuplicateValidator_MissingContext(t *testing.T) {
	finder := &mockDuplicateFinder{}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := context.Background()

	inv := validInvoice()
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed)
	assert.Contains(t, results[0].Message, "context missing")
}

func TestDuplicateValidator_FinderError(t *testing.T) {
	finder := &mockDuplicateFinder{err: errors.New("db connection failed")}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed)
	assert.Contains(t, results[0].Message, "unavailable")
}

func TestDuplicateValidator_Metadata(t *testing.T) {
	finder := &mockDuplicateFinder{}
	v := invoice.DuplicateInvoiceValidator(finder)

	assert.Equal(t, "logic.invoice.duplicate", v.RuleKey())
	assert.Equal(t, "Logical: Duplicate Invoice Detection", v.RuleName())
	assert.Equal(t, domain.ValidationRuleCustom, v.RuleType())
	assert.Equal(t, domain.ValidationSeverityWarning, v.Severity())
	assert.False(t, v.ReconciliationCritical())
}

// --- Integration checks ---

func TestAllBuiltinValidators_StillReturns56(t *testing.T) {
	all := invoice.AllBuiltinValidators()
	assert.Len(t, all, 56, "Duplicate validator should NOT be in AllBuiltinValidators()")
}

func TestDuplicateValidator_NoKeyConflict(t *testing.T) {
	builtinKeys := make(map[string]bool)
	for _, v := range invoice.AllBuiltinValidators() {
		builtinKeys[v.RuleKey()] = true
	}

	finder := &mockDuplicateFinder{}
	dupKey := invoice.DuplicateInvoiceValidator(finder).RuleKey()
	assert.False(t, builtinKeys[dupKey],
		"duplicate validator key %q conflicts with an existing builtin validator", dupKey)
}
```

**Step 2: Run all tests**

Run: `go test ./tests/unit/validator/invoice/... -v -count=1 -race`
Expected: ALL PASS

**Step 3: Run full test suite to verify no regressions**

Run: `go test ./... -v -count=1 -race`
Expected: ALL PASS

**Step 4: Commit**

```
test: comprehensive tiered duplicate detection tests
```

---

## Task 5: Run lint and fix any issues

**Step 1: Run linter**

Run: `make lint`
Expected: PASS (fix any issues that arise from the changes)

**Step 2: Commit if any fixes were needed**

```
chore: lint fixes for duplicate detection
```

---

## Task 6: Update CLAUDE.md documentation

**Files:**
- Modify: `CLAUDE.md`

**Step 1: Update the Validation Engine section**

In the Validation Engine table, update the Duplicate row:

```
| Duplicate | 1 | `logic.invoice.duplicate` | `invoice/duplicate.go` |
```

Add a note below the table:

```markdown
### Duplicate Detection Tiers

The `logic.invoice.duplicate` validator uses three match tiers:

| Tier | Match Criteria | Confidence | Effective Severity |
|------|---------------|------------|-------------------|
| `exact_irn` | Same IRN (64-char hex) | Guaranteed | Error |
| `strong` | Seller GSTIN + Invoice Number + same Financial Year | Very high | Error |
| `weak` | Seller GSTIN + Invoice Number (cross-FY) | Possible | Warning |

The `DuplicateInvoiceFinder.FindDuplicates` interface accepts `sellerGSTIN`, `invoiceNumber`, `financialYear`, and `irn` parameters. The repo query uses `document_summaries.invoice_date` for FY matching (avoids JSONB date parsing). Results include `MatchType` and `DocumentID` for frontend linking.
```

**Step 2: Update Gotchas section if needed**

Add under Gotchas:
```
- **Duplicate detection uses document_summaries for FY matching**: Tier-2 (strong) matching joins `document_summaries` to get the parsed `invoice_date` timestamp, avoiding fragile JSONB date parsing in SQL. Summary must exist for tier-2 to match (non-blocking upsert means brief window where strong match may not fire for just-parsed docs)
```

**Step 3: Commit**

```
docs: update CLAUDE.md with tiered duplicate detection details
```

---

## Task 7: Write frontend integration prompt

**Files:**
- Create: `docs/frontend-duplicate-detection-integration.md`

**Step 1: Write the React + TypeScript integration guide**

This should cover:
1. How the validation API response includes duplicate results
2. How to parse the `ActualValue` field for severity (`[error]` vs `[warning]`)
3. How to extract document names and match types from the `Message` field
4. Example React component for rendering duplicate alerts
5. UI/UX recommendations (banner placement, severity colors, link to duplicate docs)

The guide should reference the existing `GET /documents/:id/validation` endpoint and explain how to filter for the `logic.invoice.duplicate` rule in the results array.

**Step 2: Commit**

```
docs: add React frontend integration guide for duplicate detection
```

---

## Task 8: Final verification

**Step 1: Run full test suite**

Run: `make test`
Expected: ALL PASS

**Step 2: Run linter**

Run: `make lint`
Expected: PASS

**Step 3: Verify build**

Run: `make build`
Expected: SUCCESS

---

## Dependency Order

```
Task 1 (port interface) → Task 2 (repo query) → Task 3 (validator) → Task 4 (tests) → Task 5 (lint) → Task 6 (docs) → Task 7 (frontend guide) → Task 8 (verification)
```

All tasks are sequential — each depends on the previous.
