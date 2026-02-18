package validator

import (
	"satvos/internal/domain"
)

// FieldStatus represents the computed validation state for a single field path.
type FieldStatus struct {
	Status   domain.FieldValidationStatus `json:"status"`
	Messages []string                     `json:"messages"`
}

// resultWithSeverity pairs a validation result with its rule's severity.
type resultWithSeverity struct {
	Passed   bool
	Severity domain.ValidationSeverity
	Message  string
}

// ComputeFieldStatuses derives per-field validation statuses from results and confidence scores.
// confidenceMap maps field paths (e.g. "seller.gstin") to confidence float64 values.
func ComputeFieldStatuses(
	results []ValidationResultEntry,
	rules map[string]*domain.DocumentValidationRule,
	confidenceMap map[string]float64,
) map[string]*FieldStatus {
	// Group results by field path
	fieldResults := make(map[string][]resultWithSeverity)
	for idx := range results {
		r := &results[idx]
		rule := rules[r.RuleID.String()]
		if rule == nil {
			continue
		}
		fieldResults[r.FieldPath] = append(fieldResults[r.FieldPath], resultWithSeverity{
			Passed:   r.Passed,
			Severity: rule.Severity,
			Message:  r.Message,
		})
	}

	statuses := make(map[string]*FieldStatus)

	// Compute status for fields that have validation results
	for fieldPath, rws := range fieldResults {
		fs := &FieldStatus{Status: domain.FieldStatusValid}
		for _, rw := range rws {
			if !rw.Passed {
				if rw.Severity == domain.ValidationSeverityError {
					fs.Status = domain.FieldStatusInvalid
				} else if fs.Status != domain.FieldStatusInvalid {
					fs.Status = domain.FieldStatusUnsure
				}
				fs.Messages = append(fs.Messages, rw.Message)
			}
		}
		if fs.Messages == nil {
			fs.Messages = []string{}
		}
		statuses[fieldPath] = fs
	}

	// For fields with confidence scores but no validation results,
	// derive status from confidence alone.
	for fieldPath, confidence := range confidenceMap {
		if _, exists := statuses[fieldPath]; exists {
			continue
		}
		if confidence <= 0.5 {
			statuses[fieldPath] = &FieldStatus{
				Status:   domain.FieldStatusUnsure,
				Messages: []string{},
			}
		} else {
			statuses[fieldPath] = &FieldStatus{
				Status:   domain.FieldStatusValid,
				Messages: []string{},
			}
		}
	}

	return statuses
}
