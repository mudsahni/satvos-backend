package invoice

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Exported format patterns shared across merge, trust computation, and format validators.
var (
	GSTINPattern = regexp.MustCompile(`^\d{2}[A-Z]{5}\d{4}[A-Z][1-9A-Z]Z[0-9A-Z]$`)
	PANPattern   = regexp.MustCompile(`^[A-Z]{5}\d{4}[A-Z]$`)
	IFSCPattern  = regexp.MustCompile(`^[A-Z]{4}0[A-Z0-9]{6}$`)
	HSNPattern   = regexp.MustCompile(`^\d{4,8}$`)
	AcctPattern  = regexp.MustCompile(`^\d{9,18}$`)
	IRNPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	AckNoPattern = regexp.MustCompile(`^\d+$`)
)

// KnownCurrencies is the set of recognized ISO 4217 currency codes.
var KnownCurrencies = map[string]bool{
	"INR": true, "USD": true, "EUR": true, "GBP": true, "JPY": true,
	"AUD": true, "CAD": true, "CHF": true, "CNY": true, "SGD": true,
	"AED": true, "SAR": true, "HKD": true, "MYR": true, "THB": true,
	"NZD": true, "SEK": true, "NOK": true, "DKK": true, "ZAR": true,
}

// ParseDate tries common date formats and returns the parsed time.
func ParseDate(s string) (time.Time, error) {
	return parseDate(s)
}

// IsValidStateCode checks if a string is a valid 2-digit Indian GST state code (01-38).
func IsValidStateCode(code string) bool {
	if len(code) != 2 {
		return false
	}
	n, err := strconv.Atoi(code)
	return err == nil && n >= 1 && n <= 38
}

// IsValidCurrency checks if a string is a recognized ISO 4217 currency code.
func IsValidCurrency(s string) bool {
	return KnownCurrencies[strings.ToUpper(strings.TrimSpace(s))]
}

// IsParseableDate checks if a string can be parsed as a date.
func IsParseableDate(s string) bool {
	_, err := parseDate(s)
	return err == nil
}

// FieldFormatCheckers maps field paths to their format check functions.
// Used by trust computation to give format-match bonuses.
var FieldFormatCheckers = map[string]func(string) bool{
	"seller.gstin":                    func(s string) bool { return GSTINPattern.MatchString(s) },
	"buyer.gstin":                     func(s string) bool { return GSTINPattern.MatchString(s) },
	"seller.pan":                      func(s string) bool { return PANPattern.MatchString(s) },
	"buyer.pan":                       func(s string) bool { return PANPattern.MatchString(s) },
	"seller.state_code":               IsValidStateCode,
	"buyer.state_code":                IsValidStateCode,
	"invoice.invoice_date":            IsParseableDate,
	"invoice.due_date":                IsParseableDate,
	"invoice.acknowledgement_date":    IsParseableDate,
	"invoice.irn":                     func(s string) bool { return IRNPattern.MatchString(strings.ToLower(s)) },
	"invoice.acknowledgement_number":  func(s string) bool { return AckNoPattern.MatchString(s) },
	"invoice.currency":                IsValidCurrency,
	"payment.ifsc_code":               func(s string) bool { return IFSCPattern.MatchString(s) },
	"payment.account_number":          func(s string) bool { return AcctPattern.MatchString(s) },
}
