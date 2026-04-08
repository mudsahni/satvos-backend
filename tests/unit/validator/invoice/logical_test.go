package invoice_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"satvos/internal/domain"
	"satvos/internal/validator/invoice"
)

func TestLogicalValidators_Count(t *testing.T) {
	assert.Len(t, invoice.LogicalValidators(), 8)
}

func TestLogicalValidators_Metadata(t *testing.T) {
	for _, v := range invoice.LogicalValidators() {
		assert.NotEmpty(t, v.RuleKey())
		assert.NotEmpty(t, v.RuleName())
		assert.Equal(t, domain.ValidationRuleCustom, v.RuleType())
	}
}

func TestLogic_LineItemNonNegative(t *testing.T) {
	v := findLogicalValidator("logic.line_item.non_negative")
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_all_positive", func(t *testing.T) {
		inv := validInvoice()
		results := v.Validate(ctx, inv)
		// 7 amount fields per line item
		require.Len(t, results, 7)
		for _, r := range results {
			assert.True(t, r.Passed, "field %s should be non-negative", r.FieldPath)
		}
	})

	t.Run("pass_zero_amounts", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].Discount = 0
		inv.LineItems[0].IGSTAmount = 0
		results := v.Validate(ctx, inv)
		require.Len(t, results, 7)
		for _, r := range results {
			assert.True(t, r.Passed)
		}
	})

	t.Run("fail_negative_quantity", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].Quantity = -1
		results := v.Validate(ctx, inv)
		require.Len(t, results, 7)
		failed := false
		for _, r := range results {
			if r.FieldPath == "line_items[0].quantity" {
				assert.False(t, r.Passed)
				failed = true
			}
		}
		assert.True(t, failed, "should have a failed result for quantity")
	})

	t.Run("fail_negative_total", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].Total = -100
		results := v.Validate(ctx, inv)
		failed := false
		for _, r := range results {
			if r.FieldPath == "line_items[0].total" {
				assert.False(t, r.Passed)
				failed = true
			}
		}
		assert.True(t, failed)
	})

	t.Run("multiple_items", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems = append(inv.LineItems, invoice.LineItem{
			Quantity: 5, UnitPrice: 100, TaxableAmount: 500, Total: 500,
		})
		results := v.Validate(ctx, inv)
		require.Len(t, results, 14) // 7 * 2 items
	})
}

func TestLogic_ValidTaxRate(t *testing.T) {
	v := findLogicalValidator("logic.line_item.valid_tax_rate")
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_standard_rates", func(t *testing.T) {
		inv := validInvoice() // CGST=9, SGST=9, IGST=0 — all valid
		results := v.Validate(ctx, inv)
		require.Len(t, results, 3) // 3 rates per item
		for _, r := range results {
			assert.True(t, r.Passed, "field %s should have valid rate", r.FieldPath)
		}
	})

	t.Run("pass_zero_rate", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].CGSTRate = 0
		inv.LineItems[0].SGSTRate = 0
		inv.LineItems[0].IGSTRate = 0
		results := v.Validate(ctx, inv)
		for _, r := range results {
			assert.True(t, r.Passed)
		}
	})

	t.Run("pass_0.125_rate", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].CGSTRate = 0.125
		inv.LineItems[0].SGSTRate = 0.125
		results := v.Validate(ctx, inv)
		for _, r := range results {
			assert.True(t, r.Passed)
		}
	})

	t.Run("fail_non_standard", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].CGSTRate = 7 // non-standard
		results := v.Validate(ctx, inv)
		failed := false
		for _, r := range results {
			if r.FieldPath == "line_items[0].cgst_rate" {
				assert.False(t, r.Passed)
				failed = true
			}
		}
		assert.True(t, failed)
	})

	t.Run("severity_warning", func(t *testing.T) {
		assert.Equal(t, domain.ValidationSeverityWarning, v.Severity())
	})
}

func TestLogic_CGSTEqualsSGST(t *testing.T) {
	v := findLogicalValidator("logic.line_item.cgst_eq_sgst")
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_equal", func(t *testing.T) {
		inv := validInvoice() // CGST=9, SGST=9
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("pass_both_zero", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].CGSTRate = 0
		inv.LineItems[0].SGSTRate = 0
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("fail_unequal", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].CGSTRate = 9
		inv.LineItems[0].SGSTRate = 5
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
	})
}

func TestLogic_ExclusiveTax(t *testing.T) {
	v := findLogicalValidator("logic.line_item.exclusive_tax")
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_cgst_sgst_only", func(t *testing.T) {
		inv := validInvoice() // CGST+SGST used, IGST=0
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("pass_igst_only", func(t *testing.T) {
		inv := validInterstateInvoice() // IGST only
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("pass_all_zero", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].CGSTRate = 0
		inv.LineItems[0].CGSTAmount = 0
		inv.LineItems[0].SGSTRate = 0
		inv.LineItems[0].SGSTAmount = 0
		inv.LineItems[0].IGSTRate = 0
		inv.LineItems[0].IGSTAmount = 0
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("fail_both_present", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].IGSTRate = 18
		inv.LineItems[0].IGSTAmount = 180 // now both CGST/SGST and IGST
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
	})

	t.Run("is_reconciliation_critical", func(t *testing.T) {
		assert.True(t, v.ReconciliationCritical())
	})
}

func TestLogic_AtLeastOneLineItem(t *testing.T) {
	v := findLogicalValidator("logic.line_items.at_least_one")
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_has_items", func(t *testing.T) {
		results := v.Validate(ctx, validInvoice())
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("fail_empty_slice", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems = nil
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
		assert.Equal(t, "line_items", results[0].FieldPath)
	})

	t.Run("fail_empty_slice_not_nil", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems = []invoice.LineItem{}
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
	})

	t.Run("is_reconciliation_critical", func(t *testing.T) {
		assert.True(t, v.ReconciliationCritical())
	})
}

func TestLogic_DateNotFuture(t *testing.T) {
	v := findLogicalValidator("logic.invoice.date_not_future")
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_past_date", func(t *testing.T) {
		results := v.Validate(ctx, validInvoice()) // "15/01/2025"
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("pass_today", func(t *testing.T) {
		inv := validInvoice()
		inv.Invoice.InvoiceDate = time.Now().Format("02/01/2006")
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("fail_future_date", func(t *testing.T) {
		inv := validInvoice()
		future := time.Now().AddDate(1, 0, 0)
		inv.Invoice.InvoiceDate = future.Format("02/01/2006")
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
	})

	t.Run("skip_empty", func(t *testing.T) {
		inv := validInvoice()
		inv.Invoice.InvoiceDate = ""
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("skip_unparseable", func(t *testing.T) {
		inv := validInvoice()
		inv.Invoice.InvoiceDate = "not-a-date"
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("severity_warning", func(t *testing.T) {
		assert.Equal(t, domain.ValidationSeverityWarning, v.Severity())
	})
}

func TestLogic_TotalsNonNegative(t *testing.T) {
	v := findLogicalValidator("logic.totals.non_negative")
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_all_positive", func(t *testing.T) {
		inv := validInvoice()
		results := v.Validate(ctx, inv)
		require.Len(t, results, 6) // 6 total fields checked
		for _, r := range results {
			assert.True(t, r.Passed, "field %s should be non-negative", r.FieldPath)
		}
	})

	t.Run("pass_zero_values", func(t *testing.T) {
		inv := validInvoice()
		inv.Totals.IGST = 0
		results := v.Validate(ctx, inv)
		for _, r := range results {
			if r.FieldPath == "totals.igst" {
				assert.True(t, r.Passed)
			}
		}
	})

	t.Run("fail_negative_subtotal", func(t *testing.T) {
		inv := validInvoice()
		inv.Totals.Subtotal = -100
		results := v.Validate(ctx, inv)
		require.Len(t, results, 6)
		failed := false
		for _, r := range results {
			if r.FieldPath == "totals.subtotal" {
				assert.False(t, r.Passed)
				failed = true
			}
		}
		assert.True(t, failed)
	})

	t.Run("fail_negative_total", func(t *testing.T) {
		inv := validInvoice()
		inv.Totals.Total = -1
		results := v.Validate(ctx, inv)
		failed := false
		for _, r := range results {
			if r.FieldPath == "totals.total" {
				assert.False(t, r.Passed)
				failed = true
			}
		}
		assert.True(t, failed)
	})

	t.Run("checks_six_fields", func(t *testing.T) {
		inv := validInvoice()
		results := v.Validate(ctx, inv)
		fields := make(map[string]bool)
		for _, r := range results {
			fields[r.FieldPath] = true
		}
		expectedFields := []string{
			"totals.subtotal", "totals.taxable_amount",
			"totals.cgst", "totals.sgst", "totals.igst", "totals.total",
		}
		for _, f := range expectedFields {
			assert.True(t, fields[f], "should check field %s", f)
		}
	})
}

func TestAllValidInvoice_PassesAllValidators(t *testing.T) {
	ctx := context.Background()
	inv := validInvoice()

	for _, v := range invoice.AllBuiltinValidators() {
		results := v.Validate(ctx, inv)
		if results == nil {
			continue // cross-field validators return nil when not applicable
		}
		for _, r := range results {
			assert.True(t, r.Passed, "validator %s failed for valid invoice: field=%s msg=%s",
				v.RuleKey(), r.FieldPath, r.Message)
		}
	}
}

func TestAllBuiltinValidators_Count(t *testing.T) {
	all := invoice.AllBuiltinValidators()
	assert.Len(t, all, 60, "expected 60 built-in validators, got %d", len(all))
}

func TestAllBuiltinValidators_UniqueKeys(t *testing.T) {
	all := invoice.AllBuiltinValidators()
	keys := make(map[string]bool)
	for _, v := range all {
		key := v.RuleKey()
		assert.False(t, keys[key], "duplicate rule key: %s", key)
		keys[key] = true
	}
}

func TestReconciliationCriticalCount(t *testing.T) {
	count := 0
	for _, v := range invoice.AllBuiltinValidators() {
		if v.ReconciliationCritical() {
			count++
		}
	}
	assert.Equal(t, 22, count, "expected 22 reconciliation-critical validators")
}

func TestValidInterstateInvoice_PassesAllValidators(t *testing.T) {
	ctx := context.Background()
	inv := validInterstateInvoice()

	for _, v := range invoice.AllBuiltinValidators() {
		results := v.Validate(ctx, inv)
		if results == nil {
			continue
		}
		for _, r := range results {
			assert.True(t, r.Passed, "validator %s failed for interstate invoice: field=%s msg=%s",
				v.RuleKey(), r.FieldPath, r.Message)
		}
	}
}

func TestFmtfHelper(t *testing.T) {
	// Verify fmtf is used in result messages (indirect test via math validator)
	v := findMathValidator("math.totals.cgst")
	require.NotNil(t, v)
	ctx := context.Background()

	inv := validInvoice()
	inv.Totals.CGST = 999.99
	results := v.Validate(ctx, inv)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].ActualValue, fmt.Sprintf("%.2f", 999.99))
}
