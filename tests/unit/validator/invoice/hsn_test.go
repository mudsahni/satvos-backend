package invoice_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"satvos/internal/domain"
	"satvos/internal/port"
	"satvos/internal/validator/invoice"
)

// testHSNLookup returns a small HSN lookup for testing.
// Contains:
//   - "8471" at 18% (computers — matches validInvoice)
//   - "8471" also at 12% (conditional rate)
//   - "84714100" at 18% (8-digit code, parent 8471)
//   - "1006" at 5% (rice)
//   - "100630" at 5% (6-digit rice)
//   - "0101" at 0% (live animals, exempt)
func testHSNLookup() *invoice.HSNLookup {
	return invoice.NewHSNLookup([]port.HSNEntry{
		{Code: "8471", Description: "Automatic data processing machines", GSTRate: 18},
		{Code: "8471", Description: "Automatic data processing machines (conditional)", GSTRate: 12, ConditionDesc: "used/refurbished"},
		{Code: "84714100", Description: "Digital computers", GSTRate: 18},
		{Code: "1006", Description: "Rice", GSTRate: 5},
		{Code: "100630", Description: "Semi-milled or wholly milled rice", GSTRate: 5},
		{Code: "0101", Description: "Live horses, asses, mules", GSTRate: 0},
	})
}

func findHSNValidator(key string, lookup *invoice.HSNLookup) *invoice.BuiltinValidator {
	for _, v := range invoice.HSNValidators(lookup) {
		if v.RuleKey() == key {
			return v
		}
	}
	return nil
}

// --- HSNLookup tests ---

func TestHSNLookup_Exists(t *testing.T) {
	lookup := testHSNLookup()

	t.Run("exact_match", func(t *testing.T) {
		assert.True(t, lookup.Exists("8471"))
		assert.True(t, lookup.Exists("84714100"))
		assert.True(t, lookup.Exists("1006"))
		assert.True(t, lookup.Exists("0101"))
	})

	t.Run("not_found", func(t *testing.T) {
		assert.False(t, lookup.Exists("9999"))
		assert.False(t, lookup.Exists("1234"))
	})

	t.Run("prefix_fallback_8_to_4", func(t *testing.T) {
		// "84710000" is not in the map, but "8471" (4-digit prefix) is
		assert.True(t, lookup.Exists("84710000"))
	})

	t.Run("prefix_fallback_8_to_6", func(t *testing.T) {
		// "10063010" is not in the map, but "100630" (6-digit prefix) is
		assert.True(t, lookup.Exists("10063010"))
	})

	t.Run("empty_code", func(t *testing.T) {
		assert.False(t, lookup.Exists(""))
	})

	t.Run("empty_lookup", func(t *testing.T) {
		empty := invoice.NewHSNLookup(nil)
		assert.False(t, empty.Exists("8471"))
	})
}

func TestHSNLookup_Rates(t *testing.T) {
	lookup := testHSNLookup()

	t.Run("single_rate", func(t *testing.T) {
		rates := lookup.Rates("1006")
		require.Len(t, rates, 1)
		assert.InDelta(t, 5.0, rates[0].Rate, 0.01)
	})

	t.Run("multiple_rates", func(t *testing.T) {
		rates := lookup.Rates("8471")
		require.Len(t, rates, 2)
	})

	t.Run("prefix_fallback", func(t *testing.T) {
		rates := lookup.Rates("84710000")
		require.Len(t, rates, 2) // falls back to "8471"
	})

	t.Run("not_found", func(t *testing.T) {
		rates := lookup.Rates("9999")
		assert.Nil(t, rates)
	})
}

func TestHSNLookup_RateMatches(t *testing.T) {
	lookup := testHSNLookup()

	t.Run("exact_match", func(t *testing.T) {
		matched, rates := lookup.RateMatches("8471", 18)
		assert.True(t, matched)
		require.Len(t, rates, 2)
	})

	t.Run("conditional_rate_match", func(t *testing.T) {
		matched, _ := lookup.RateMatches("8471", 12)
		assert.True(t, matched)
	})

	t.Run("rate_mismatch", func(t *testing.T) {
		matched, rates := lookup.RateMatches("8471", 28)
		assert.False(t, matched)
		require.Len(t, rates, 2)
	})

	t.Run("code_not_found", func(t *testing.T) {
		matched, rates := lookup.RateMatches("9999", 18)
		assert.False(t, matched)
		assert.Nil(t, rates)
	})

	t.Run("zero_rate", func(t *testing.T) {
		matched, _ := lookup.RateMatches("0101", 0)
		assert.True(t, matched)
	})
}

// --- HSN Exists validator tests ---

func TestHSN_ExistsValidator_Count(t *testing.T) {
	lookup := testHSNLookup()
	validators := invoice.HSNValidators(lookup)
	assert.Len(t, validators, 2)
}

func TestHSN_Exists(t *testing.T) {
	lookup := testHSNLookup()
	v := findHSNValidator("logic.line_item.hsn_exists", lookup)
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_known_code", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].HSNSACCode = "8471"
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
		assert.Equal(t, "line_items[0].hsn_sac_code", results[0].FieldPath)
	})

	t.Run("fail_unknown_code", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].HSNSACCode = "9999"
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
		assert.Contains(t, results[0].Message, "not found")
	})

	t.Run("skip_empty_code", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].HSNSACCode = ""
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
		assert.Contains(t, results[0].Message, "empty")
	})

	t.Run("pass_prefix_fallback", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].HSNSACCode = "84710000" // not exact, but 8471 exists
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("multiple_items", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].HSNSACCode = "8471"
		inv.LineItems = append(inv.LineItems, invoice.LineItem{
			HSNSACCode: "9999", Quantity: 1, UnitPrice: 100, TaxableAmount: 100, Total: 100,
		})
		results := v.Validate(ctx, inv)
		require.Len(t, results, 2)
		assert.True(t, results[0].Passed)
		assert.False(t, results[1].Passed)
	})

	t.Run("severity_warning", func(t *testing.T) {
		assert.Equal(t, domain.ValidationSeverityWarning, v.Severity())
	})

	t.Run("not_reconciliation_critical", func(t *testing.T) {
		assert.False(t, v.ReconciliationCritical())
	})

	t.Run("rule_type_custom", func(t *testing.T) {
		assert.Equal(t, domain.ValidationRuleCustom, v.RuleType())
	})
}

// --- HSN Rate validator tests ---

func TestHSN_Rate(t *testing.T) {
	lookup := testHSNLookup()
	v := findHSNValidator("xf.line_item.hsn_rate", lookup)
	require.NotNil(t, v)
	ctx := context.Background()

	t.Run("pass_correct_intrastate_rate", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].HSNSACCode = "8471"
		// CGST=9 + SGST=9 = 18% (matches 8471's rate)
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("pass_correct_interstate_rate", func(t *testing.T) {
		inv := validInterstateInvoice()
		inv.LineItems[0].HSNSACCode = "8471"
		// IGST=18% (matches 8471's rate)
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("fail_wrong_rate", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].HSNSACCode = "1006" // expects 5%
		// CGST=9 + SGST=9 = 18%, but 1006 expects 5%
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.False(t, results[0].Passed)
		assert.Contains(t, results[0].Message, "does not match")
	})

	t.Run("pass_conditional_rate", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].HSNSACCode = "8471"
		inv.LineItems[0].CGSTRate = 6
		inv.LineItems[0].SGSTRate = 6
		// 6+6=12%, which is a conditional rate for 8471
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("skip_empty_code", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].HSNSACCode = ""
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
		assert.Contains(t, results[0].Message, "empty")
	})

	t.Run("skip_unknown_code", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].HSNSACCode = "9999"
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
		assert.Contains(t, results[0].Message, "not in master")
	})

	t.Run("pass_zero_rate_exempt", func(t *testing.T) {
		inv := validInvoice()
		inv.LineItems[0].HSNSACCode = "0101" // 0% exempt
		inv.LineItems[0].CGSTRate = 0
		inv.LineItems[0].SGSTRate = 0
		inv.LineItems[0].IGSTRate = 0
		results := v.Validate(ctx, inv)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})

	t.Run("severity_warning", func(t *testing.T) {
		assert.Equal(t, domain.ValidationSeverityWarning, v.Severity())
	})

	t.Run("not_reconciliation_critical", func(t *testing.T) {
		assert.False(t, v.ReconciliationCritical())
	})

	t.Run("rule_type_cross_field", func(t *testing.T) {
		assert.Equal(t, domain.ValidationRuleCrossField, v.RuleType())
	})
}

// --- Verify HSN validators don't affect existing counts ---

func TestHSNValidators_SeparateFromBuiltin(t *testing.T) {
	// AllBuiltinValidators() should still return exactly 60
	all := invoice.AllBuiltinValidators()
	assert.Len(t, all, 60, "HSN validators should not be included in AllBuiltinValidators()")
}

func TestHSNValidators_UniqueKeys(t *testing.T) {
	lookup := testHSNLookup()
	validators := invoice.HSNValidators(lookup)
	keys := make(map[string]bool)
	for _, v := range validators {
		key := v.RuleKey()
		assert.False(t, keys[key], "duplicate HSN validator key: %s", key)
		keys[key] = true
	}
}

func TestHSNValidators_NoKeyConflictWithBuiltin(t *testing.T) {
	builtinKeys := make(map[string]bool)
	for _, v := range invoice.AllBuiltinValidators() {
		builtinKeys[v.RuleKey()] = true
	}

	lookup := testHSNLookup()
	for _, v := range invoice.HSNValidators(lookup) {
		assert.False(t, builtinKeys[v.RuleKey()],
			"HSN validator key %q conflicts with an existing builtin validator", v.RuleKey())
	}
}
