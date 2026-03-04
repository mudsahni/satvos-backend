package validator_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"satvos/internal/domain"
	"satvos/internal/validator"
)

func TestBuildReviewSuggestions_ErrorsGetPriority1(t *testing.T) {
	fieldStatuses := map[string]*validator.FieldStatus{
		"seller.gstin": {Status: domain.FieldStatusInvalid, Messages: []string{"GSTIN invalid"}},
	}
	trustMap := map[string]*validator.FieldTrustEntry{
		"seller.gstin": {Trust: 0.1, Reasons: []string{"validation_error"}},
	}

	suggestions := validator.BuildReviewSuggestions(fieldStatuses, trustMap, nil, nil)

	assert.NotEmpty(t, suggestions)
	assert.Equal(t, 1, suggestions[0].Priority)
	assert.Equal(t, "seller.gstin", suggestions[0].FieldPath)
	assert.Equal(t, "validation_error", suggestions[0].Category)
}

func TestBuildReviewSuggestions_DisagreementsAppear(t *testing.T) {
	fieldStatuses := map[string]*validator.FieldStatus{}
	trustMap := map[string]*validator.FieldTrustEntry{
		"seller.name": {Trust: 0.4, Reasons: []string{"dual_parse_disagree"}},
	}

	suggestions := validator.BuildReviewSuggestions(fieldStatuses, trustMap, nil, nil)

	assert.NotEmpty(t, suggestions)
	found := false
	for _, s := range suggestions {
		if s.FieldPath == "seller.name" && s.Category == "disagreement" {
			found = true
			assert.Equal(t, 2, s.Priority)
		}
	}
	assert.True(t, found)
}

func TestBuildReviewSuggestions_Max10(t *testing.T) {
	fieldStatuses := make(map[string]*validator.FieldStatus)
	trustMap := make(map[string]*validator.FieldTrustEntry)

	ruleID := uuid.New()
	rules := map[string]*domain.DocumentValidationRule{
		ruleID.String(): {ID: ruleID, Severity: domain.ValidationSeverityError},
	}

	// Create 15 fields with errors
	fields := []string{
		"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10",
		"f11", "f12", "f13", "f14", "f15",
	}
	results := make([]validator.ValidationResultEntry, 0, len(fields))
	for _, f := range fields {
		fieldStatuses[f] = &validator.FieldStatus{
			Status:   domain.FieldStatusInvalid,
			Messages: []string{"error"},
		}
		trustMap[f] = &validator.FieldTrustEntry{Trust: 0.1, Reasons: []string{"validation_error"}}
		results = append(results, validator.ValidationResultEntry{
			RuleID: ruleID, Passed: false, FieldPath: f,
		})
	}

	suggestions := validator.BuildReviewSuggestions(fieldStatuses, trustMap, results, rules)

	assert.LessOrEqual(t, len(suggestions), 10)
}

func TestBuildReviewSuggestions_NothingToReview(t *testing.T) {
	fieldStatuses := map[string]*validator.FieldStatus{
		"seller.gstin": {Status: domain.FieldStatusValid, Messages: []string{}},
	}
	trustMap := map[string]*validator.FieldTrustEntry{
		"seller.gstin": {Trust: 0.95, Reasons: []string{"dual_parse_agree"}},
	}

	suggestions := validator.BuildReviewSuggestions(fieldStatuses, trustMap, nil, nil)

	assert.Empty(t, suggestions)
}
