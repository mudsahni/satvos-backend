package invoice

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"satvos/internal/domain"
)

// Private aliases for the exported patterns in format_patterns.go.
var (
	gstinPattern = GSTINPattern
	panPattern   = PANPattern
	ifscPattern  = IFSCPattern
	hsnPattern   = HSNPattern
	acctPattern  = AcctPattern
)

// knownCurrencies aliases the exported KnownCurrencies map.
var knownCurrencies = KnownCurrencies

// formatValidator checks a field against a regex or format rule.
type formatValidator struct {
	ruleKey       string
	ruleName      string
	fieldPath     string
	severity      domain.ValidationSeverity
	reconCritical bool
	validate      func(*GSTInvoice) []ValidationResult
}

func (v *formatValidator) RuleKey() string                     { return v.ruleKey }
func (v *formatValidator) RuleName() string                    { return v.ruleName }
func (v *formatValidator) RuleType() domain.ValidationRuleType { return domain.ValidationRuleRegex }
func (v *formatValidator) Severity() domain.ValidationSeverity { return v.severity }
func (v *formatValidator) ReconciliationCritical() bool        { return v.reconCritical }

func (v *formatValidator) Validate(_ context.Context, data *GSTInvoice) []ValidationResult {
	return v.validate(data)
}

func regexCheck(fieldPath, value, pattern, ruleName string, re *regexp.Regexp) ValidationResult {
	if value == "" {
		return ValidationResult{
			Passed: true, FieldPath: fieldPath,
			ExpectedValue: pattern, ActualValue: value,
			Message: fmt.Sprintf("%s: field is empty, skipping format check", ruleName),
		}
	}
	passed := re.MatchString(value)
	msg := fmt.Sprintf("%s: %s matches expected format", ruleName, fieldPath)
	if !passed {
		msg = fmt.Sprintf("%s: %s does not match expected format", ruleName, fieldPath)
	}
	return ValidationResult{
		Passed: passed, FieldPath: fieldPath,
		ExpectedValue: pattern, ActualValue: value, Message: msg,
	}
}

func dateCheck(fieldPath, value, ruleName string) ValidationResult {
	if value == "" {
		return ValidationResult{
			Passed: true, FieldPath: fieldPath,
			ExpectedValue: "parseable date", ActualValue: value,
			Message: fmt.Sprintf("%s: field is empty, skipping date check", ruleName),
		}
	}
	_, err := parseDate(value)
	passed := err == nil
	msg := fmt.Sprintf("%s: %s is a valid date", ruleName, fieldPath)
	if !passed {
		msg = fmt.Sprintf("%s: %s is not a parseable date", ruleName, fieldPath)
	}
	return ValidationResult{
		Passed: passed, FieldPath: fieldPath,
		ExpectedValue: "parseable date", ActualValue: value, Message: msg,
	}
}

func stateCodeCheck(fieldPath, value, ruleName string) ValidationResult {
	if value == "" {
		return ValidationResult{
			Passed: true, FieldPath: fieldPath,
			ExpectedValue: "2-digit state code (01-38)", ActualValue: value,
			Message: fmt.Sprintf("%s: field is empty, skipping state code check", ruleName),
		}
	}
	passed := false
	if len(value) == 2 {
		code, err := strconv.Atoi(value)
		if err == nil && code >= 1 && code <= 38 {
			passed = true
		}
	}
	msg := fmt.Sprintf("%s: %s is a valid state code", ruleName, fieldPath)
	if !passed {
		msg = fmt.Sprintf("%s: %s is not a valid 2-digit state code (01-38)", ruleName, fieldPath)
	}
	return ValidationResult{
		Passed: passed, FieldPath: fieldPath,
		ExpectedValue: "2-digit state code (01-38)", ActualValue: value, Message: msg,
	}
}

// parseDate tries common date formats.
func parseDate(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02",
		"02-01-2006",
		"02/01/2006",
		"01-02-2006",
		"01/02/2006",
		"2006/01/02",
		"02 Jan 2006",
		"2 Jan 2006",
		"Jan 02, 2006",
		"January 02, 2006",
		"02-01-2006 15:04:05",
		"2006-01-02T15:04:05Z07:00",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, strings.TrimSpace(s)); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable date: %s", s)
}

// FormatValidators returns all format validators.
func FormatValidators() []*formatValidator {
	return []*formatValidator{
		{
			ruleKey: "fmt.seller.gstin", ruleName: "Format: Seller GSTIN",
			fieldPath: "seller.gstin", severity: domain.ValidationSeverityError, reconCritical: true,
			validate: func(d *GSTInvoice) []ValidationResult {
				return []ValidationResult{regexCheck("seller.gstin", d.Seller.GSTIN, "15-char GSTIN format", "Format: Seller GSTIN", gstinPattern)}
			},
		},
		{
			ruleKey: "fmt.buyer.gstin", ruleName: "Format: Buyer GSTIN",
			fieldPath: "buyer.gstin", severity: domain.ValidationSeverityError, reconCritical: true,
			validate: func(d *GSTInvoice) []ValidationResult {
				return []ValidationResult{regexCheck("buyer.gstin", d.Buyer.GSTIN, "15-char GSTIN format", "Format: Buyer GSTIN", gstinPattern)}
			},
		},
		{
			ruleKey: "fmt.seller.pan", ruleName: "Format: Seller PAN",
			fieldPath: "seller.pan", severity: domain.ValidationSeverityError,
			validate: func(d *GSTInvoice) []ValidationResult {
				return []ValidationResult{regexCheck("seller.pan", d.Seller.PAN, "10-char PAN format", "Format: Seller PAN", panPattern)}
			},
		},
		{
			ruleKey: "fmt.buyer.pan", ruleName: "Format: Buyer PAN",
			fieldPath: "buyer.pan", severity: domain.ValidationSeverityError,
			validate: func(d *GSTInvoice) []ValidationResult {
				return []ValidationResult{regexCheck("buyer.pan", d.Buyer.PAN, "10-char PAN format", "Format: Buyer PAN", panPattern)}
			},
		},
		{
			ruleKey: "fmt.seller.state_code", ruleName: "Format: Seller State Code",
			fieldPath: "seller.state_code", severity: domain.ValidationSeverityError, reconCritical: true,
			validate: func(d *GSTInvoice) []ValidationResult {
				return []ValidationResult{stateCodeCheck("seller.state_code", d.Seller.StateCode, "Format: Seller State Code")}
			},
		},
		{
			ruleKey: "fmt.buyer.state_code", ruleName: "Format: Buyer State Code",
			fieldPath: "buyer.state_code", severity: domain.ValidationSeverityError, reconCritical: true,
			validate: func(d *GSTInvoice) []ValidationResult {
				return []ValidationResult{stateCodeCheck("buyer.state_code", d.Buyer.StateCode, "Format: Buyer State Code")}
			},
		},
		{
			ruleKey: "fmt.invoice.date", ruleName: "Format: Invoice Date",
			fieldPath: "invoice.invoice_date", severity: domain.ValidationSeverityError,
			validate: func(d *GSTInvoice) []ValidationResult {
				return []ValidationResult{dateCheck("invoice.invoice_date", d.Invoice.InvoiceDate, "Format: Invoice Date")}
			},
		},
		{
			ruleKey: "fmt.invoice.due_date", ruleName: "Format: Due Date",
			fieldPath: "invoice.due_date", severity: domain.ValidationSeverityWarning,
			validate: func(d *GSTInvoice) []ValidationResult {
				return []ValidationResult{dateCheck("invoice.due_date", d.Invoice.DueDate, "Format: Due Date")}
			},
		},
		{
			ruleKey: "fmt.invoice.currency", ruleName: "Format: Currency",
			fieldPath: "invoice.currency", severity: domain.ValidationSeverityWarning,
			validate: func(d *GSTInvoice) []ValidationResult {
				val := strings.ToUpper(strings.TrimSpace(d.Invoice.Currency))
				if val == "" {
					return []ValidationResult{{
						Passed: true, FieldPath: "invoice.currency",
						ExpectedValue: "ISO 4217 code", ActualValue: val,
						Message: "Format: Currency: field is empty, skipping",
					}}
				}
				passed := knownCurrencies[val]
				msg := "Format: Currency: valid ISO 4217 code"
				if !passed {
					msg = "Format: Currency: not a recognized ISO 4217 code"
				}
				return []ValidationResult{{
					Passed: passed, FieldPath: "invoice.currency",
					ExpectedValue: "ISO 4217 code", ActualValue: val, Message: msg,
				}}
			},
		},
		{
			ruleKey: "fmt.payment.ifsc", ruleName: "Format: IFSC Code",
			fieldPath: "payment.ifsc_code", severity: domain.ValidationSeverityWarning,
			validate: func(d *GSTInvoice) []ValidationResult {
				return []ValidationResult{regexCheck("payment.ifsc_code", d.Payment.IFSCCode, "IFSC format (XXXX0XXXXXX)", "Format: IFSC Code", ifscPattern)}
			},
		},
		{
			ruleKey: "fmt.payment.account_no", ruleName: "Format: Account Number",
			fieldPath: "payment.account_number", severity: domain.ValidationSeverityWarning,
			validate: func(d *GSTInvoice) []ValidationResult {
				return []ValidationResult{regexCheck("payment.account_number", d.Payment.AccountNumber, "9-18 digit account number", "Format: Account Number", acctPattern)}
			},
		},
		{
			ruleKey: "fmt.line_item.hsn_sac", ruleName: "Format: HSN/SAC Code",
			fieldPath: "line_items[i].hsn_sac_code", severity: domain.ValidationSeverityWarning,
			validate: func(d *GSTInvoice) []ValidationResult {
				results := make([]ValidationResult, 0, len(d.LineItems))
				for i := range d.LineItems {
					fp := fmt.Sprintf("line_items[%d].hsn_sac_code", i)
					results = append(results, regexCheck(fp, d.LineItems[i].HSNSACCode, "4-8 digit HSN/SAC code", "Format: HSN/SAC Code", hsnPattern))
				}
				return results
			},
		},
	}
}
