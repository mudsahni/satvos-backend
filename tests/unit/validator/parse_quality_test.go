package validator_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"satvos/internal/domain"
	"satvos/internal/validator"
	"satvos/internal/validator/invoice"
)

func TestParseQualityScore_HighTrust(t *testing.T) {
	trustMap := map[string]*validator.FieldTrustEntry{
		"invoice.invoice_number": {Trust: 0.95},
		"seller.gstin":          {Trust: 0.90},
		"buyer.gstin":           {Trust: 0.90},
		"totals.total":          {Trust: 0.95},
		"seller.name":           {Trust: 0.85},
	}
	inv := &invoice.GSTInvoice{}

	score := validator.ParseQualityScore(trustMap, inv, domain.ValidationStatusValid)

	assert.Greater(t, score, 80.0)
	assert.LessOrEqual(t, score, 100.0)
}

func TestParseQualityScore_MixedTrust(t *testing.T) {
	trustMap := map[string]*validator.FieldTrustEntry{
		"invoice.invoice_number": {Trust: 0.95},
		"seller.gstin":          {Trust: 0.4},
		"buyer.gstin":           {Trust: 0.3},
		"totals.total":          {Trust: 0.95},
	}
	inv := &invoice.GSTInvoice{}

	score := validator.ParseQualityScore(trustMap, inv, domain.ValidationStatusValid)

	assert.Greater(t, score, 40.0)
	assert.Less(t, score, 80.0)
}

func TestParseQualityScore_ValidationPenalty(t *testing.T) {
	trustMap := map[string]*validator.FieldTrustEntry{
		"invoice.invoice_number": {Trust: 0.95},
		"seller.gstin":          {Trust: 0.90},
	}
	inv := &invoice.GSTInvoice{}

	scoreValid := validator.ParseQualityScore(trustMap, inv, domain.ValidationStatusValid)
	scoreInvalid := validator.ParseQualityScore(trustMap, inv, domain.ValidationStatusInvalid)
	scoreWarning := validator.ParseQualityScore(trustMap, inv, domain.ValidationStatusWarning)

	assert.Greater(t, scoreValid, scoreWarning)
	assert.Greater(t, scoreWarning, scoreInvalid)
}

func TestParseQualityScore_Empty(t *testing.T) {
	score := validator.ParseQualityScore(nil, &invoice.GSTInvoice{}, domain.ValidationStatusValid)
	assert.Equal(t, float64(0), score)
}
