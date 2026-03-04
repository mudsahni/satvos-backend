package validator_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"satvos/internal/domain"
	"satvos/internal/validator"
	"satvos/internal/validator/invoice"
)

func TestComputeFieldTrust_DualParseAgree(t *testing.T) {
	inv := &invoice.GSTInvoice{
		Seller: invoice.Party{GSTIN: "29ABCDE1234F1Z5"},
	}
	provenance := map[string]string{"seller.gstin": "agree"}

	trust := validator.ComputeFieldTrust(provenance, nil, nil, inv)

	assert.InDelta(t, 0.90, trust["seller.gstin"].Trust, 0.05) // 0.85 base + format bonus
	assert.Contains(t, trust["seller.gstin"].Reasons, "dual_parse_agree")
}

func TestComputeFieldTrust_DualParseDisagree(t *testing.T) {
	inv := &invoice.GSTInvoice{
		Seller: invoice.Party{Name: "Some Corp"},
	}
	provenance := map[string]string{"seller.name": "disagreement"}

	trust := validator.ComputeFieldTrust(provenance, nil, nil, inv)

	assert.Equal(t, 0.4, trust["seller.name"].Trust)
	assert.Contains(t, trust["seller.name"].Reasons, "dual_parse_disagree")
}

func TestComputeFieldTrust_SingleParse(t *testing.T) {
	inv := &invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{InvoiceNumber: "INV-001"},
	}

	trust := validator.ComputeFieldTrust(nil, nil, nil, inv)

	assert.Equal(t, 0.5, trust["invoice.invoice_number"].Trust)
	assert.Contains(t, trust["invoice.invoice_number"].Reasons, "single_parse")
}

func TestComputeFieldTrust_ManualEdit(t *testing.T) {
	inv := &invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{InvoiceNumber: "INV-001"},
	}
	provenance := map[string]string{"invoice.invoice_number": "manual_edit"}

	trust := validator.ComputeFieldTrust(provenance, nil, nil, inv)

	assert.Equal(t, 1.0, trust["invoice.invoice_number"].Trust)
	assert.Contains(t, trust["invoice.invoice_number"].Reasons, "manual_edit")
}

func TestComputeFieldTrust_EmptyField(t *testing.T) {
	inv := &invoice.GSTInvoice{
		Seller: invoice.Party{GSTIN: ""}, // empty
	}

	trust := validator.ComputeFieldTrust(nil, nil, nil, inv)

	assert.Equal(t, 0.0, trust["seller.gstin"].Trust)
	assert.Contains(t, trust["seller.gstin"].Reasons, "field_empty")
}

func TestComputeFieldTrust_ValidationErrorPenalty(t *testing.T) {
	ruleID := uuid.New()
	inv := &invoice.GSTInvoice{
		Seller: invoice.Party{GSTIN: "INVALID"},
	}
	provenance := map[string]string{"seller.gstin": "agree"}
	results := []validator.ValidationResultEntry{
		{RuleID: ruleID, Passed: false, FieldPath: "seller.gstin"},
	}
	rules := map[string]*domain.DocumentValidationRule{
		ruleID.String(): {ID: ruleID, Severity: domain.ValidationSeverityError},
	}

	trust := validator.ComputeFieldTrust(provenance, results, rules, inv)

	assert.Equal(t, 0.1, trust["seller.gstin"].Trust)
	assert.Contains(t, trust["seller.gstin"].Reasons, "validation_error")
}

func TestComputeFieldTrust_ValidationPassBonus(t *testing.T) {
	ruleID := uuid.New()
	inv := &invoice.GSTInvoice{
		Seller: invoice.Party{GSTIN: "29ABCDE1234F1Z5"},
	}
	provenance := map[string]string{"seller.gstin": "agree"}
	results := []validator.ValidationResultEntry{
		{RuleID: ruleID, Passed: true, FieldPath: "seller.gstin"},
	}
	rules := map[string]*domain.DocumentValidationRule{
		ruleID.String(): {ID: ruleID, Severity: domain.ValidationSeverityError},
	}

	trust := validator.ComputeFieldTrust(provenance, results, rules, inv)

	// 0.85 (agree) + 0.1 (pass) + 0.05 (format match) = 1.0 (clamped)
	assert.Equal(t, 1.0, trust["seller.gstin"].Trust)
	assert.Contains(t, trust["seller.gstin"].Reasons, "validation_passed")
}

func TestComputeFieldTrust_FormatMatchBonus(t *testing.T) {
	inv := &invoice.GSTInvoice{
		Seller: invoice.Party{GSTIN: "29ABCDE1234F1Z5"},
	}
	provenance := map[string]string{"seller.gstin": "primary"}

	trust := validator.ComputeFieldTrust(provenance, nil, nil, inv)

	// 0.6 (primary/one empty) + 0.05 (format match) = 0.65
	assert.InDelta(t, 0.65, trust["seller.gstin"].Trust, 0.01)
	assert.Contains(t, trust["seller.gstin"].Reasons, "format_match")
}

func TestComputeFieldTrust_NilInvoice(t *testing.T) {
	trust := validator.ComputeFieldTrust(nil, nil, nil, nil)
	assert.Empty(t, trust)
}
