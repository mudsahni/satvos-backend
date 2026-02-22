package normalize_test

import (
	"testing"

	"satvos/internal/normalize"

	"github.com/stretchr/testify/assert"
)

func TestCompanyName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "pvt ltd mixed case",
			input:    "Facebook Pvt Ltd",
			expected: "FACEBOOK PVT LTD",
		},
		{
			name:     "private limited lowercase",
			input:    "facebook private limited",
			expected: "FACEBOOK PVT LTD",
		},
		{
			name:     "dots in abbreviations",
			input:    "facebook pvt. ltd.",
			expected: "FACEBOOK PVT LTD",
		},
		{
			name:     "extra whitespace",
			input:    "  Acme  Corp  ",
			expected: "ACME CORP",
		},
		{
			name:     "limited suffix",
			input:    "M/S RELIANCE INDUSTRIES LIMITED",
			expected: "M/S RELIANCE INDUSTRIES LTD",
		},
		{
			name:     "private limited suffix",
			input:    "TATA SONS PRIVATE LIMITED",
			expected: "TATA SONS PVT LTD",
		},
		{
			name:     "pvt limited mixed",
			input:    "ABC Pvt Limited",
			expected: "ABC PVT LTD",
		},
		{
			name:     "private ltd mixed",
			input:    "XYZ Private Ltd",
			expected: "XYZ PVT LTD",
		},
		{
			name:     "and and company",
			input:    "RAJ AND SONS COMPANY",
			expected: "RAJ & SONS CO",
		},
		{
			name:     "dots in name with corporation",
			input:    "P.Q.R. Corporation",
			expected: "PQR CORP",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			name:     "idempotent already normalized",
			input:    "ALREADY UPPER PVT LTD",
			expected: "ALREADY UPPER PVT LTD",
		},
		{
			name:     "incorporated suffix",
			input:    "Acme Incorporated",
			expected: "ACME INC",
		},
		{
			name:     "single word no suffix",
			input:    "Google",
			expected: "GOOGLE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalize.CompanyName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
