package invoice_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"satvos/internal/domain"
	"satvos/internal/validator/invoice"
)

func TestCrossFieldValidators_Count(t *testing.T) {
	assert.Len(t, invoice.CrossFieldValidators(), 11)
}

func TestCrossFieldValidators_Metadata(t *testing.T) {
	for _, v := range invoice.CrossFieldValidators() {
		assert.NotEmpty(t, v.RuleKey())
		assert.NotEmpty(t, v.RuleName())
		assert.Equal(t, domain.ValidationRuleCrossField, v.RuleType())
	}
}

func TestXF_SellerGSTINState(t *testing.T) {
	v := findCrossFieldValidator("xf.seller.gstin_state")
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_matching", func(t *testing.T) {
		inv := validInvoice() // GSTIN starts with "29", state_code "29"
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("fail_mismatch", func(t *testing.T) {
		inv := validInvoice()
		inv.Seller.GSTIN = "27ABCDE1234F1Z5" // prefix 27
		inv.Seller.StateCode = "29"            // but state is 29
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
	})

	t.Run("skip_empty_gstin", func(t *testing.T) {
		inv := validInvoice()
		inv.Seller.GSTIN = ""
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("skip_empty_state_code", func(t *testing.T) {
		inv := validInvoice()
		inv.Seller.StateCode = ""
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("fail_short_gstin", func(t *testing.T) {
		inv := validInvoice()
		inv.Seller.GSTIN = "2" // too short
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
	})

	t.Run("pass_single_digit_state_code_padded", func(t *testing.T) {
		inv := validInvoice()
		inv.Seller.GSTIN = "07ABCDE1234F1Z5"
		inv.Seller.StateCode = "7" // should be padded to "07"
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("is_reconciliation_critical", func(t *testing.T) {
		assert.True(t, v.ReconciliationCritical())
	})
}

func TestXF_BuyerGSTINState(t *testing.T) {
	v := findCrossFieldValidator("xf.buyer.gstin_state")
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_matching", func(t *testing.T) {
		results := v.Validate(ctx, validInvoice())
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("fail_mismatch", func(t *testing.T) {
		inv := validInvoice()
		inv.Buyer.GSTIN = "27FGHIJ5678K1Z2"
		inv.Buyer.StateCode = "29"
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
	})

	t.Run("skip_empty", func(t *testing.T) {
		inv := validInvoice()
		inv.Buyer.GSTIN = ""
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})
}

func TestXF_SellerGSTINPAN(t *testing.T) {
	v := findCrossFieldValidator("xf.seller.gstin_pan")
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_matching", func(t *testing.T) {
		// GSTIN "29ABCDE1234F1Z5"[2:12] == "ABCDE1234F" == PAN
		results := v.Validate(ctx, validInvoice())
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("fail_mismatch", func(t *testing.T) {
		inv := validInvoice()
		inv.Seller.PAN = "ZZZZZ9999Z" // doesn't match GSTIN[2:12]
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
	})

	t.Run("skip_empty_gstin", func(t *testing.T) {
		inv := validInvoice()
		inv.Seller.GSTIN = ""
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("skip_empty_pan", func(t *testing.T) {
		inv := validInvoice()
		inv.Seller.PAN = ""
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("fail_short_gstin", func(t *testing.T) {
		inv := validInvoice()
		inv.Seller.GSTIN = "29ABCDE" // length 7, need >= 12
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
	})
}

func TestXF_BuyerGSTINPAN(t *testing.T) {
	v := findCrossFieldValidator("xf.buyer.gstin_pan")
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_matching", func(t *testing.T) {
		results := v.Validate(ctx, validInvoice())
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("fail_mismatch", func(t *testing.T) {
		inv := validInvoice()
		inv.Buyer.PAN = "ZZZZZ9999Z"
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
	})

	t.Run("skip_empty", func(t *testing.T) {
		inv := validInvoice()
		inv.Buyer.PAN = ""
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})
}

func TestXF_IntrastateTaxType(t *testing.T) {
	v := findCrossFieldValidator("xf.tax_type.intrastate")
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_same_state_cgst_sgst", func(t *testing.T) {
		inv := validInvoice() // same state, CGST+SGST used
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("fail_same_state_igst_used", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].IGSTRate = 18
		inv.LineItems[0].IGSTAmount = 180
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
	})

	t.Run("skip_different_states", func(t *testing.T) {
		inv := validInterstateInvoice()
		results := v.Validate(ctx, inv)
		assert.Nil(t, results) // returns nil for non-intrastate
	})

	t.Run("skip_missing_state_codes", func(t *testing.T) {
		inv := validInvoice()
		inv.Seller.StateCode = ""
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("is_reconciliation_critical", func(t *testing.T) {
		assert.True(t, v.ReconciliationCritical())
	})
}

func TestXF_InterstateTaxType(t *testing.T) {
	v := findCrossFieldValidator("xf.tax_type.interstate")
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_different_state_igst", func(t *testing.T) {
		inv := validInterstateInvoice()
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("fail_different_state_cgst_used", func(t *testing.T) {
		inv := validInterstateInvoice()
		inv.LineItems[0].CGSTRate = 9
		inv.LineItems[0].CGSTAmount = 90
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
	})

	t.Run("skip_same_state", func(t *testing.T) {
		inv := validInvoice() // same state
		results := v.Validate(ctx, inv)
		assert.Nil(t, results) // returns nil for non-interstate
	})

	t.Run("skip_missing_state_codes", func(t *testing.T) {
		inv := validInterstateInvoice()
		inv.Buyer.StateCode = ""
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})
}

func TestXF_DueAfterDate(t *testing.T) {
	v := findCrossFieldValidator("xf.invoice.due_after_date")
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_due_after_invoice", func(t *testing.T) {
		inv := validInvoice() // invoice=15/01/2025, due=15/02/2025
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("pass_same_date", func(t *testing.T) {
		inv := validInvoice()
		inv.Invoice.DueDate = "15/01/2025" // same as invoice date
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("fail_due_before_invoice", func(t *testing.T) {
		inv := validInvoice()
		inv.Invoice.DueDate = "14/01/2025" // before invoice date
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
	})

	t.Run("skip_missing_invoice_date", func(t *testing.T) {
		inv := validInvoice()
		inv.Invoice.InvoiceDate = ""
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("skip_missing_due_date", func(t *testing.T) {
		inv := validInvoice()
		inv.Invoice.DueDate = ""
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("skip_unparseable_dates", func(t *testing.T) {
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

func TestXF_DifferentGSTIN(t *testing.T) {
	v := findCrossFieldValidator("xf.parties.different_gstin")
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_different", func(t *testing.T) {
		results := v.Validate(ctx, validInvoice())
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("fail_same", func(t *testing.T) {
		inv := validInvoice()
		inv.Buyer.GSTIN = inv.Seller.GSTIN
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
	})

	t.Run("skip_empty_seller", func(t *testing.T) {
		inv := validInvoice()
		inv.Seller.GSTIN = ""
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("skip_empty_buyer", func(t *testing.T) {
		inv := validInvoice()
		inv.Buyer.GSTIN = ""
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("severity_warning", func(t *testing.T) {
		assert.Equal(t, domain.ValidationSeverityWarning, v.Severity())
	})
}
