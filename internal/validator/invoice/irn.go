package invoice

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"satvos/internal/domain"
)

// Private aliases for the exported patterns in format_patterns.go.
var (
	irnPattern   = IRNPattern
	ackNoPattern = AckNoPattern
)

// DeriveFinancialYear returns the Indian financial year string (e.g., "2024-25")
// for a given invoice date string parsed via parseDate().
func DeriveFinancialYear(invoiceDate string) (string, error) {
	t, err := parseDate(invoiceDate)
	if err != nil {
		return "", err
	}
	year := t.Year()
	month := t.Month()
	if month >= 4 { // April onwards
		return fmt.Sprintf("%d-%02d", year, (year+1)%100), nil
	}
	return fmt.Sprintf("%d-%02d", year-1, year%100), nil
}

// ComputeIRNHash computes the expected IRN as SHA-256(sellerGSTIN + invoiceNumber + fy).
func ComputeIRNHash(sellerGSTIN, invoiceNumber, fy string) string {
	input := sellerGSTIN + invoiceNumber + fy
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash)
}

// IRNFormatValidators returns format validators for IRN-related fields.
func IRNFormatValidators() []*formatValidator {
	return []*formatValidator{
		{
			ruleKey: "fmt.invoice.irn", ruleName: "Format: IRN",
			fieldPath: "invoice.irn", severity: domain.ValidationSeverityError,
			validate: func(d *GSTInvoice) []ValidationResult {
				return []ValidationResult{regexCheck("invoice.irn", strings.ToLower(d.Invoice.IRN), "64-char lowercase hex SHA-256", "Format: IRN", irnPattern)}
			},
		},
		{
			ruleKey: "fmt.invoice.ack_number", ruleName: "Format: Acknowledgement Number",
			fieldPath: "invoice.acknowledgement_number", severity: domain.ValidationSeverityWarning,
			validate: func(d *GSTInvoice) []ValidationResult {
				return []ValidationResult{regexCheck("invoice.acknowledgement_number", d.Invoice.AcknowledgementNumber, "numeric string", "Format: Acknowledgement Number", ackNoPattern)}
			},
		},
		{
			ruleKey: "fmt.invoice.ack_date", ruleName: "Format: Acknowledgement Date",
			fieldPath: "invoice.acknowledgement_date", severity: domain.ValidationSeverityWarning,
			validate: func(d *GSTInvoice) []ValidationResult {
				return []ValidationResult{dateCheck("invoice.acknowledgement_date", d.Invoice.AcknowledgementDate, "Format: Acknowledgement Date")}
			},
		},
	}
}

// IRNCrossFieldValidators returns cross-field validators for IRN hash verification.
func IRNCrossFieldValidators() []*crossFieldValidator {
	return []*crossFieldValidator{
		{
			ruleKey: "xf.invoice.irn_hash", ruleName: "Cross-field: IRN Hash Verification",
			severity: domain.ValidationSeverityWarning, reconCritical: true,
			validate: func(d *GSTInvoice) []ValidationResult {
				irn := strings.ToLower(d.Invoice.IRN)
				if irn == "" || d.Seller.GSTIN == "" || d.Invoice.InvoiceNumber == "" || d.Invoice.InvoiceDate == "" {
					return []ValidationResult{{
						Passed: true, FieldPath: "invoice.irn",
						Message: "Cross-field: IRN Hash Verification: required fields missing, skipping",
					}}
				}

				fy, err := DeriveFinancialYear(d.Invoice.InvoiceDate)
				if err != nil {
					return []ValidationResult{{
						Passed: true, FieldPath: "invoice.irn",
						Message: "Cross-field: IRN Hash Verification: cannot parse invoice date, skipping",
					}}
				}

				expected := ComputeIRNHash(d.Seller.GSTIN, d.Invoice.InvoiceNumber, fy)
				passed := irn == expected
				msg := "Cross-field: IRN Hash Verification: IRN matches computed hash"
				if !passed {
					msg = "Cross-field: IRN Hash Verification: IRN does not match SHA-256(GSTIN+InvoiceNumber+FY)"
				}
				return []ValidationResult{{
					Passed: passed, FieldPath: "invoice.irn",
					ExpectedValue: expected,
					ActualValue:   irn,
					Message:       msg,
				}}
			},
		},
	}
}

// IRNLogicalValidators returns logical validators for IRN presence checks.
func IRNLogicalValidators() []*logicalValidator {
	return []*logicalValidator{
		{
			ruleKey: "logic.invoice.irn_expected", ruleName: "Logical: IRN Expected for B2B Invoice",
			severity: domain.ValidationSeverityWarning,
			validate: func(d *GSTInvoice) []ValidationResult {
				// If IRN is present, pass
				if d.Invoice.IRN != "" {
					return []ValidationResult{{
						Passed: true, FieldPath: "invoice.irn",
						Message: "Logical: IRN Expected for B2B Invoice: IRN is present",
					}}
				}
				// If no seller GSTIN, can't determine if B2B — skip
				if d.Seller.GSTIN == "" {
					return []ValidationResult{{
						Passed: true, FieldPath: "invoice.irn",
						Message: "Logical: IRN Expected for B2B Invoice: no seller GSTIN, skipping",
					}}
				}
				// B2B invoice (seller has GSTIN) but no IRN — warn
				return []ValidationResult{{
					Passed:        false,
					FieldPath:     "invoice.irn",
					ExpectedValue: "non-empty IRN for B2B invoice",
					ActualValue:   "",
					Message:       "Logical: IRN Expected for B2B Invoice: IRN is missing on a B2B invoice — e-invoicing may be required",
				}}
			},
		},
	}
}
