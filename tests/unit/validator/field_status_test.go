package validator_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"satvos/internal/domain"
	"satvos/internal/validator"
)

func TestComputeFieldStatuses_AllPassed(t *testing.T) {
	ruleID := uuid.New()
	rules := map[string]*domain.DocumentValidationRule{
		ruleID.String(): {
			ID:       ruleID,
			Severity: domain.ValidationSeverityError,
		},
	}

	results := []validator.ValidationResultEntry{
		{RuleID: ruleID, Passed: true, FieldPath: "seller.gstin", Message: "GSTIN valid"},
		{RuleID: ruleID, Passed: true, FieldPath: "buyer.gstin", Message: "GSTIN valid"},
	}

	statuses := validator.ComputeFieldStatuses(results, rules, nil)

	assert.Equal(t, domain.FieldStatusValid, statuses["seller.gstin"].Status)
	assert.Equal(t, domain.FieldStatusValid, statuses["buyer.gstin"].Status)
	assert.Empty(t, statuses["seller.gstin"].Messages)
}

func TestComputeFieldStatuses_ErrorField(t *testing.T) {
	ruleID := uuid.New()
	rules := map[string]*domain.DocumentValidationRule{
		ruleID.String(): {
			ID:       ruleID,
			Severity: domain.ValidationSeverityError,
		},
	}

	results := []validator.ValidationResultEntry{
		{RuleID: ruleID, Passed: false, FieldPath: "seller.gstin", Message: "GSTIN is missing"},
	}

	statuses := validator.ComputeFieldStatuses(results, rules, nil)

	assert.Equal(t, domain.FieldStatusInvalid, statuses["seller.gstin"].Status)
	assert.Contains(t, statuses["seller.gstin"].Messages, "GSTIN is missing")
}

func TestComputeFieldStatuses_WarningField(t *testing.T) {
	ruleID := uuid.New()
	rules := map[string]*domain.DocumentValidationRule{
		ruleID.String(): {
			ID:       ruleID,
			Severity: domain.ValidationSeverityWarning,
		},
	}

	results := []validator.ValidationResultEntry{
		{RuleID: ruleID, Passed: false, FieldPath: "invoice.currency", Message: "Currency is missing"},
	}

	statuses := validator.ComputeFieldStatuses(results, rules, nil)

	assert.Equal(t, domain.FieldStatusUnsure, statuses["invoice.currency"].Status)
	assert.Contains(t, statuses["invoice.currency"].Messages, "Currency is missing")
}

func TestComputeFieldStatuses_ErrorOverridesWarning(t *testing.T) {
	errorRuleID := uuid.New()
	warningRuleID := uuid.New()
	rules := map[string]*domain.DocumentValidationRule{
		errorRuleID.String(): {
			ID:       errorRuleID,
			Severity: domain.ValidationSeverityError,
		},
		warningRuleID.String(): {
			ID:       warningRuleID,
			Severity: domain.ValidationSeverityWarning,
		},
	}

	results := []validator.ValidationResultEntry{
		{RuleID: warningRuleID, Passed: false, FieldPath: "seller.gstin", Message: "warning msg"},
		{RuleID: errorRuleID, Passed: false, FieldPath: "seller.gstin", Message: "error msg"},
	}

	statuses := validator.ComputeFieldStatuses(results, rules, nil)

	assert.Equal(t, domain.FieldStatusInvalid, statuses["seller.gstin"].Status)
	assert.Len(t, statuses["seller.gstin"].Messages, 2)
}

func TestComputeFieldStatuses_TrustLow(t *testing.T) {
	trustMap := map[string]*validator.FieldTrustEntry{
		"seller.pan": {Trust: 0.3, Reasons: []string{"dual_parse_disagree"}},
	}

	statuses := validator.ComputeFieldStatuses(nil, nil, trustMap)

	assert.Equal(t, domain.FieldStatusUnsure, statuses["seller.pan"].Status)
	assert.Equal(t, 0.3, statuses["seller.pan"].Trust)
}

func TestComputeFieldStatuses_TrustHigh(t *testing.T) {
	trustMap := map[string]*validator.FieldTrustEntry{
		"seller.name": {Trust: 0.85, Reasons: []string{"dual_parse_agree"}},
	}

	statuses := validator.ComputeFieldStatuses(nil, nil, trustMap)

	assert.Equal(t, domain.FieldStatusValid, statuses["seller.name"].Status)
	assert.Equal(t, 0.85, statuses["seller.name"].Trust)
}

func TestComputeFieldStatuses_EmptyInput(t *testing.T) {
	statuses := validator.ComputeFieldStatuses(nil, nil, nil)

	assert.Empty(t, statuses)
}

func TestComputeFieldStatuses_TrustPopulatedOnValidatedField(t *testing.T) {
	ruleID := uuid.New()
	rules := map[string]*domain.DocumentValidationRule{
		ruleID.String(): {
			ID:       ruleID,
			Severity: domain.ValidationSeverityError,
		},
	}

	results := []validator.ValidationResultEntry{
		{RuleID: ruleID, Passed: true, FieldPath: "seller.gstin", Message: "GSTIN valid"},
	}

	trustMap := map[string]*validator.FieldTrustEntry{
		"seller.gstin": {Trust: 0.95, Reasons: []string{"dual_parse_agree", "validation_passed", "format_match"}},
	}

	statuses := validator.ComputeFieldStatuses(results, rules, trustMap)

	assert.Equal(t, domain.FieldStatusValid, statuses["seller.gstin"].Status)
	assert.Equal(t, 0.95, statuses["seller.gstin"].Trust)
	assert.Contains(t, statuses["seller.gstin"].Reasons, "dual_parse_agree")
}
