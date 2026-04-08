package invoice_test

import (
	"context"
	"encoding/json"
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
	matches    []port.DuplicateMatch
	err        error
	calledGSTIN  string
	calledInvNum string
	calledFY     string
	calledIRN    string
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

// --- Context helper tests ---

func TestValidationContext_RoundTrip(t *testing.T) {
	tenantID := uuid.New()
	docID := uuid.New()

	ctx := invoice.WithValidationContext(context.Background(), tenantID, docID)

	gotTenant, ok := invoice.TenantIDFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, tenantID, gotTenant)

	gotDoc, ok := invoice.DocumentIDFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, docID, gotDoc)
}

func TestValidationContext_Missing(t *testing.T) {
	ctx := context.Background()

	gotTenant, ok := invoice.TenantIDFromContext(ctx)
	assert.False(t, ok)
	assert.Equal(t, uuid.UUID{}, gotTenant)

	gotDoc, ok := invoice.DocumentIDFromContext(ctx)
	assert.False(t, ok)
	assert.Equal(t, uuid.UUID{}, gotDoc)
}

// --- Duplicate validator tests ---

func TestDuplicateValidator_NoDuplicates(t *testing.T) {
	finder := &mockDuplicateFinder{matches: nil}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed)
	assert.Contains(t, results[0].Message, "no duplicate")
	assert.Nil(t, results[0].Metadata)
}

func TestDuplicateValidator_ExactIRNMatch(t *testing.T) {
	docID := uuid.New()
	finder := &mockDuplicateFinder{
		matches: []port.DuplicateMatch{
			{
				DocumentID:   docID,
				DocumentName: "Invoice-IRN.pdf",
				MatchType:    "exact_irn",
				CreatedAt:    time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.False(t, results[0].Passed)
	assert.Contains(t, results[0].ActualValue, "error")
	assert.Contains(t, results[0].Message, "exact_irn")

	// Verify metadata contains document ID for frontend linking
	require.NotNil(t, results[0].Metadata)
	var meta map[string][]map[string]string
	require.NoError(t, json.Unmarshal(results[0].Metadata, &meta))
	require.Len(t, meta["duplicates"], 1)
	assert.Equal(t, docID.String(), meta["duplicates"][0]["document_id"])
	assert.Equal(t, "Invoice-IRN.pdf", meta["duplicates"][0]["document_name"])
	assert.Equal(t, "exact_irn", meta["duplicates"][0]["match_type"])
}

func TestDuplicateValidator_StrongMatch(t *testing.T) {
	finder := &mockDuplicateFinder{
		matches: []port.DuplicateMatch{
			{
				DocumentID:   uuid.New(),
				DocumentName: "Invoice-Strong.pdf",
				MatchType:    "strong",
				CreatedAt:    time.Date(2025, 2, 20, 0, 0, 0, 0, time.UTC),
			},
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
			{
				DocumentID:   uuid.New(),
				DocumentName: "Invoice-Weak.pdf",
				MatchType:    "weak",
				CreatedAt:    time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC),
			},
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
	strongDocID := uuid.New()
	weakDocID := uuid.New()
	finder := &mockDuplicateFinder{
		matches: []port.DuplicateMatch{
			{
				DocumentID:   strongDocID,
				DocumentName: "Invoice-Strong.pdf",
				MatchType:    "strong",
				CreatedAt:    time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC),
			},
			{
				DocumentID:   weakDocID,
				DocumentName: "Invoice-Weak.pdf",
				MatchType:    "weak",
				CreatedAt:    time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.False(t, results[0].Passed)
	assert.Equal(t, "2 duplicate(s) found [error]", results[0].ActualValue)

	// Verify metadata contains both document IDs
	require.NotNil(t, results[0].Metadata)
	var meta map[string][]map[string]string
	require.NoError(t, json.Unmarshal(results[0].Metadata, &meta))
	require.Len(t, meta["duplicates"], 2)
	assert.Equal(t, strongDocID.String(), meta["duplicates"][0]["document_id"])
	assert.Equal(t, "strong", meta["duplicates"][0]["match_type"])
	assert.Equal(t, weakDocID.String(), meta["duplicates"][1]["document_id"])
	assert.Equal(t, "weak", meta["duplicates"][1]["match_type"])
}

func TestDuplicateValidator_PassesFYToFinder(t *testing.T) {
	finder := &mockDuplicateFinder{matches: nil}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	inv.Invoice.InvoiceDate = "15-06-2025" // June 2025 → after April → FY 2025-26
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed)
	assert.Equal(t, "2025-26", finder.calledFY)
}

func TestDuplicateValidator_PassesFYBeforeApril(t *testing.T) {
	finder := &mockDuplicateFinder{matches: nil}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	inv.Invoice.InvoiceDate = "15-01-2025" // January 2025 → before April → FY 2024-25
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed)
	assert.Equal(t, "2024-25", finder.calledFY)
}

func TestDuplicateValidator_PassesIRNToFinder(t *testing.T) {
	finder := &mockDuplicateFinder{matches: nil}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	inv.Invoice.IRN = "ABCD1234"
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed)
	assert.Equal(t, "abcd1234", finder.calledIRN)
}

func TestDuplicateValidator_EmptyIRNPassedAsEmpty(t *testing.T) {
	finder := &mockDuplicateFinder{matches: nil}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	inv.Invoice.IRN = ""
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed)
	assert.Equal(t, "", finder.calledIRN)
}

func TestDuplicateValidator_UnparseableDate_EmptyFY(t *testing.T) {
	finder := &mockDuplicateFinder{matches: nil}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	inv.Invoice.InvoiceDate = "not-a-date"
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed)
	assert.Equal(t, "", finder.calledFY)
}

func TestDuplicateValidator_EmptyDate_EmptyFY(t *testing.T) {
	finder := &mockDuplicateFinder{matches: nil}
	v := invoice.DuplicateInvoiceValidator(finder)
	ctx := invoice.WithValidationContext(context.Background(), uuid.New(), uuid.New())

	inv := validInvoice()
	inv.Invoice.InvoiceDate = ""
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed)
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
	ctx := context.Background() // no WithValidationContext

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

func TestAllBuiltinValidators_StillReturns60(t *testing.T) {
	all := invoice.AllBuiltinValidators()
	assert.Len(t, all, 60, "Duplicate validator should NOT be in AllBuiltinValidators()")
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
