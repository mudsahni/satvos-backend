package validator

import (
	"sort"

	"satvos/internal/domain"
)

// ReviewSuggestion is a prioritized suggestion for manual review.
type ReviewSuggestion struct {
	Priority  int    `json:"priority"`
	FieldPath string `json:"field_path"`
	Reason    string `json:"reason"`
	Category  string `json:"category"`
}

const maxSuggestions = 10

// BuildReviewSuggestions generates prioritized review suggestions from field statuses,
// trust entries, validation results, and rules.
func BuildReviewSuggestions(
	fieldStatuses map[string]*FieldStatus,
	trustMap map[string]*FieldTrustEntry,
	results []ValidationResultEntry,
	rules map[string]*domain.DocumentValidationRule,
) []ReviewSuggestion {
	var suggestions []ReviewSuggestion

	// Collect validation errors (priority 1)
	for fieldPath, fs := range fieldStatuses {
		if fs.Status == domain.FieldStatusInvalid {
			reason := "validation error"
			if len(fs.Messages) > 0 {
				reason = fs.Messages[0]
			}
			suggestions = append(suggestions, ReviewSuggestion{
				Priority:  1,
				FieldPath: fieldPath,
				Reason:    reason,
				Category:  "validation_error",
			})
		}
	}

	// Collect disagreements with low trust (priority 2)
	for fieldPath, te := range trustMap {
		if te.Trust <= 0.4 && containsReason(te.Reasons, "dual_parse_disagree") {
			suggestions = append(suggestions, ReviewSuggestion{
				Priority:  2,
				FieldPath: fieldPath,
				Reason:    "parsers disagreed on this field",
				Category:  "disagreement",
			})
		}
	}

	// Collect warnings (priority 3)
	for fieldPath, fs := range fieldStatuses {
		if fs.Status == domain.FieldStatusUnsure {
			// Skip if already added as validation error or disagreement
			if hasSuggestionForField(suggestions, fieldPath) {
				continue
			}
			reason := "validation warning"
			if len(fs.Messages) > 0 {
				reason = fs.Messages[0]
			}
			suggestions = append(suggestions, ReviewSuggestion{
				Priority:  3,
				FieldPath: fieldPath,
				Reason:    reason,
				Category:  "warning",
			})
		}
	}

	// Collect low trust with no validation issues (priority 4)
	for fieldPath, te := range trustMap {
		if te.Trust <= 0.4 {
			if hasSuggestionForField(suggestions, fieldPath) {
				continue
			}
			suggestions = append(suggestions, ReviewSuggestion{
				Priority:  4,
				FieldPath: fieldPath,
				Reason:    "low confidence in extracted value",
				Category:  "low_trust",
			})
		}
	}

	// Sort by priority then field path
	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].Priority != suggestions[j].Priority {
			return suggestions[i].Priority < suggestions[j].Priority
		}
		return suggestions[i].FieldPath < suggestions[j].FieldPath
	})

	// Cap at max
	if len(suggestions) > maxSuggestions {
		suggestions = suggestions[:maxSuggestions]
	}

	if suggestions == nil {
		suggestions = []ReviewSuggestion{}
	}

	return suggestions
}

func containsReason(reasons []string, target string) bool {
	for _, r := range reasons {
		if r == target {
			return true
		}
	}
	return false
}

func hasSuggestionForField(suggestions []ReviewSuggestion, fieldPath string) bool {
	for i := range suggestions {
		if suggestions[i].FieldPath == fieldPath {
			return true
		}
	}
	return false
}
