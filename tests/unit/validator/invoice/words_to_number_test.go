package invoice_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"satvos/internal/validator/invoice"
)

func TestParseIndianAmountWords_Basic(t *testing.T) {
	result, err := invoice.ParseIndianAmountWords("One Hundred Rupees Only")
	assert.NoError(t, err)
	assert.InDelta(t, 100.0, result, 0.01)
}

func TestParseIndianAmountWords_Lakhs(t *testing.T) {
	result, err := invoice.ParseIndianAmountWords("Two Lakh Eighty Nine Thousand One Hundred Rupees Only")
	assert.NoError(t, err)
	assert.InDelta(t, 289100.0, result, 0.01)
}

func TestParseIndianAmountWords_Thousands(t *testing.T) {
	result, err := invoice.ParseIndianAmountWords("Fifty Thousand Rupees")
	assert.NoError(t, err)
	assert.InDelta(t, 50000.0, result, 0.01)
}

func TestParseIndianAmountWords_Crore(t *testing.T) {
	result, err := invoice.ParseIndianAmountWords("One Crore Twenty Lakh Rupees Only")
	assert.NoError(t, err)
	assert.InDelta(t, 12000000.0, result, 0.01)
}

func TestParseIndianAmountWords_Simple(t *testing.T) {
	result, err := invoice.ParseIndianAmountWords("Fifteen")
	assert.NoError(t, err)
	assert.InDelta(t, 15.0, result, 0.01)
}

func TestParseIndianAmountWords_WithAnd(t *testing.T) {
	result, err := invoice.ParseIndianAmountWords("One Hundred and Fifty Rupees Only")
	assert.NoError(t, err)
	assert.InDelta(t, 150.0, result, 0.01)
}

func TestParseIndianAmountWords_Empty(t *testing.T) {
	_, err := invoice.ParseIndianAmountWords("")
	assert.Error(t, err)
}

func TestParseIndianAmountWords_Unparseable(t *testing.T) {
	_, err := invoice.ParseIndianAmountWords("xyz abc def")
	assert.Error(t, err)
}

func TestParseIndianAmountWords_Mixed(t *testing.T) {
	result, err := invoice.ParseIndianAmountWords("Twenty Three Thousand Four Hundred Fifty Six Rupees Only")
	assert.NoError(t, err)
	assert.InDelta(t, 23456.0, result, 0.01)
}
