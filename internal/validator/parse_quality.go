package validator

import (
	"satvos/internal/domain"
	"satvos/internal/validator/invoice"
)

// Field weight categories for quality score computation.
var (
	criticalFields = map[string]bool{
		"invoice.invoice_number": true,
		"seller.gstin":          true,
		"buyer.gstin":           true,
		"totals.total":          true,
	}
	importantFields = map[string]bool{
		"seller.name":       true,
		"buyer.name":        true,
		"seller.state_code": true,
		"buyer.state_code":  true,
		"invoice.invoice_date": true,
	}
	optionalFields = map[string]bool{
		"notes":                       true,
		"payment.payment_terms":       true,
		"invoice.qr_code_data":        true,
		"totals.amount_in_words":      true,
	}
)

func fieldWeight(path string) float64 {
	if criticalFields[path] {
		return 3.0
	}
	if importantFields[path] {
		return 2.0
	}
	if optionalFields[path] {
		return 0.5
	}
	return 1.0
}

// ParseQualityScore computes a 0-100 quality score from field trust values
// and validation status.
func ParseQualityScore(
	trustMap map[string]*FieldTrustEntry,
	inv *invoice.GSTInvoice,
	validationStatus domain.ValidationStatus,
) float64 {
	if len(trustMap) == 0 {
		return 0
	}

	var weightedSum, totalWeight float64
	for path, entry := range trustMap {
		w := fieldWeight(path)
		weightedSum += entry.Trust * w
		totalWeight += w
	}

	if totalWeight == 0 {
		return 0
	}

	score := (weightedSum / totalWeight) * 100

	// Validation penalty
	switch validationStatus {
	case domain.ValidationStatusInvalid:
		score -= 20
	case domain.ValidationStatusWarning:
		score -= 10
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score
}
