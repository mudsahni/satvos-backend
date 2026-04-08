package validator

import (
	"satvos/internal/domain"
)

// FieldStatus represents the computed validation state for a single field path.
type FieldStatus struct {
	Status   domain.FieldValidationStatus `json:"status"`
	Messages []string                     `json:"messages"`
	Trust    float64                      `json:"trust"`
	Reasons  []string                     `json:"reasons"`
}

// resultWithSeverity pairs a validation result with its rule's severity.
type resultWithSeverity struct {
	Passed   bool
	Severity domain.ValidationSeverity
	Message  string
}

// ComputeFieldStatuses derives per-field validation statuses from results and trust scores.
// trustMap maps field paths to computed FieldTrustEntry values. Pass nil for backward compat.
func ComputeFieldStatuses(
	results []ValidationResultEntry,
	rules map[string]*domain.DocumentValidationRule,
	trustMap map[string]*FieldTrustEntry,
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
		// Populate trust info if available
		if trustMap != nil {
			if te, ok := trustMap[fieldPath]; ok {
				fs.Trust = te.Trust
				fs.Reasons = te.Reasons
			}
		}
		if fs.Reasons == nil {
			fs.Reasons = []string{}
		}
		statuses[fieldPath] = fs
	}

	// For fields with trust scores but no validation results,
	// derive status from trust.
	for fieldPath, te := range trustMap {
		if _, exists := statuses[fieldPath]; exists {
			continue
		}
		fs := &FieldStatus{
			Trust:    te.Trust,
			Reasons:  te.Reasons,
			Messages: []string{},
		}
		if fs.Reasons == nil {
			fs.Reasons = []string{}
		}
		if te.Trust < 0.5 {
			fs.Status = domain.FieldStatusUnsure
		} else {
			fs.Status = domain.FieldStatusValid
		}
		statuses[fieldPath] = fs
	}

	return statuses
}
