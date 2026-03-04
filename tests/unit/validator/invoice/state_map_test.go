package invoice_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"satvos/internal/validator/invoice"
)

func TestLookupStateCode_FullName(t *testing.T) {
	code, ok := invoice.LookupStateCode("Karnataka")
	assert.True(t, ok)
	assert.Equal(t, "29", code)
}

func TestLookupStateCode_Abbreviation(t *testing.T) {
	code, ok := invoice.LookupStateCode("UP")
	assert.True(t, ok)
	assert.Equal(t, "09", code)
}

func TestLookupStateCode_CaseInsensitive(t *testing.T) {
	code, ok := invoice.LookupStateCode("karnataka")
	assert.True(t, ok)
	assert.Equal(t, "29", code)
}

func TestLookupStateCode_NotFound(t *testing.T) {
	_, ok := invoice.LookupStateCode("Atlantis")
	assert.False(t, ok)
}

func TestLookupStateCode_Empty(t *testing.T) {
	_, ok := invoice.LookupStateCode("")
	assert.False(t, ok)
}

func TestIndianStateCodeToName_Coverage(t *testing.T) {
	// Ensure all 38 codes are present
	assert.GreaterOrEqual(t, len(invoice.IndianStateCodeToName), 38)
	assert.Equal(t, "Karnataka", invoice.IndianStateCodeToName["29"])
	assert.Equal(t, "Delhi", invoice.IndianStateCodeToName["07"])
}
