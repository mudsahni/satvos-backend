package invoice

import (
	"context"
	"fmt"
	"strings"
	"time"

	"satvos/internal/domain"
)

// Valid GST tax rates.
var validTaxRates = map[float64]bool{
	0: true, 0.125: true, 0.25: true, 1.5: true, 2.5: true,
	3: true, 5: true, 6: true, 9: true, 12: true, 14: true,
	18: true, 28: true,
}

// logicalValidator checks logical constraints on the invoice data.
type logicalValidator struct {
	ruleKey       string
	ruleName      string
	severity      domain.ValidationSeverity
	reconCritical bool
	validate      func(*GSTInvoice) []ValidationResult
}

func (v *logicalValidator) RuleKey() string                     { return v.ruleKey }
func (v *logicalValidator) RuleName() string                    { return v.ruleName }
func (v *logicalValidator) RuleType() domain.ValidationRuleType { return domain.ValidationRuleCustom }
func (v *logicalValidator) Severity() domain.ValidationSeverity { return v.severity }
func (v *logicalValidator) ReconciliationCritical() bool        { return v.reconCritical }

func (v *logicalValidator) Validate(_ context.Context, data *GSTInvoice) []ValidationResult {
	return v.validate(data)
}

// LogicalValidators returns all logical validators.
func LogicalValidators() []*logicalValidator {
	return []*logicalValidator{
		{
			ruleKey: "logic.line_item.non_negative", ruleName: "Logical: Line Item Non-Negative Amounts",
			severity: domain.ValidationSeverityError,
			validate: func(d *GSTInvoice) []ValidationResult {
				results := make([]ValidationResult, 0, len(d.LineItems)*7)
				for i := range d.LineItems {
					item := &d.LineItems[i]
					amounts := map[string]float64{
						"quantity":       item.Quantity,
						"unit_price":     item.UnitPrice,
						"taxable_amount": item.TaxableAmount,
						"cgst_amount":    item.CGSTAmount,
						"sgst_amount":    item.SGSTAmount,
						"igst_amount":    item.IGSTAmount,
						"total":          item.Total,
					}
					for field, val := range amounts {
						fp := fmt.Sprintf("line_items[%d].%s", i, field)
						passed := val >= 0
						msg := fmt.Sprintf("Logical: Line Item Non-Negative Amounts: %s is non-negative", fp)
						if !passed {
							msg = fmt.Sprintf("Logical: Line Item Non-Negative Amounts: %s is negative (%.2f)", fp, val)
						}
						results = append(results, ValidationResult{
							Passed: passed, FieldPath: fp,
							ExpectedValue: ">= 0", ActualValue: fmtf(val), Message: msg,
						})
					}
				}
				return results
			},
		},
		{
			ruleKey: "logic.line_item.valid_tax_rate", ruleName: "Logical: Valid Tax Rate",
			severity: domain.ValidationSeverityWarning,
			validate: func(d *GSTInvoice) []ValidationResult {
				results := make([]ValidationResult, 0, len(d.LineItems)*3)
				for i := range d.LineItems {
					item := &d.LineItems[i]
					rates := map[string]float64{
						"cgst_rate": item.CGSTRate,
						"sgst_rate": item.SGSTRate,
						"igst_rate": item.IGSTRate,
					}
					for field, rate := range rates {
						fp := fmt.Sprintf("line_items[%d].%s", i, field)
						passed := validTaxRates[rate]
						msg := fmt.Sprintf("Logical: Valid Tax Rate: %s has a standard GST rate", fp)
						if !passed {
							msg = fmt.Sprintf("Logical: Valid Tax Rate: %s has non-standard rate %.2f", fp, rate)
						}
						results = append(results, ValidationResult{
							Passed: passed, FieldPath: fp,
							ExpectedValue: "one of {0, 0.125, 0.25, 1.5, 2.5, 3, 5, 6, 9, 12, 14, 18, 28}",
							ActualValue:   fmtf(rate), Message: msg,
						})
					}
				}
				return results
			},
		},
		{
			ruleKey: "logic.line_item.cgst_eq_sgst", ruleName: "Logical: CGST Equals SGST Rate",
			severity: domain.ValidationSeverityError,
			validate: func(d *GSTInvoice) []ValidationResult {
				results := make([]ValidationResult, 0, len(d.LineItems))
				for i := range d.LineItems {
					fp := fmt.Sprintf("line_items[%d]", i)
					item := &d.LineItems[i]
					passed := item.CGSTRate == item.SGSTRate
					msg := fmt.Sprintf("Logical: CGST Equals SGST Rate: %s CGST and SGST rates match", fp)
					if !passed {
						msg = fmt.Sprintf("Logical: CGST Equals SGST Rate: %s CGST rate (%.2f) != SGST rate (%.2f)", fp, item.CGSTRate, item.SGSTRate)
					}
					results = append(results, ValidationResult{
						Passed: passed, FieldPath: fp,
						ExpectedValue: "cgst_rate == sgst_rate",
						ActualValue:   fmt.Sprintf("cgst=%.2f, sgst=%.2f", item.CGSTRate, item.SGSTRate),
						Message:       msg,
					})
				}
				return results
			},
		},
		{
			ruleKey: "logic.line_item.exclusive_tax", ruleName: "Logical: Exclusive Tax Types",
			severity: domain.ValidationSeverityError, reconCritical: true,
			validate: func(d *GSTInvoice) []ValidationResult {
				results := make([]ValidationResult, 0, len(d.LineItems))
				for i := range d.LineItems {
					item := &d.LineItems[i]
					fp := fmt.Sprintf("line_items[%d]", i)
					hasCgstSgst := item.CGSTRate > 0 || item.SGSTRate > 0 || item.CGSTAmount > 0 || item.SGSTAmount > 0
					hasIgst := item.IGSTRate > 0 || item.IGSTAmount > 0
					passed := !hasCgstSgst || !hasIgst
					msg := fmt.Sprintf("Logical: Exclusive Tax Types: %s uses either CGST+SGST or IGST, not both", fp)
					if !passed {
						msg = fmt.Sprintf("Logical: Exclusive Tax Types: %s has both CGST/SGST and IGST applied", fp)
					}
					results = append(results, ValidationResult{
						Passed: passed, FieldPath: fp,
						ExpectedValue: "either CGST+SGST or IGST, not both",
						ActualValue:   fmt.Sprintf("CGST=%.2f, SGST=%.2f, IGST=%.2f", item.CGSTRate, item.SGSTRate, item.IGSTRate),
						Message:       msg,
					})
				}
				return results
			},
		},
		{
			ruleKey: "logic.line_items.at_least_one", ruleName: "Logical: At Least One Line Item",
			severity: domain.ValidationSeverityError, reconCritical: true,
			validate: func(d *GSTInvoice) []ValidationResult {
				passed := len(d.LineItems) >= 1
				msg := "Logical: At Least One Line Item: invoice has line items"
				if !passed {
					msg = "Logical: At Least One Line Item: invoice has no line items"
				}
				return []ValidationResult{{
					Passed: passed, FieldPath: "line_items",
					ExpectedValue: ">= 1 line item",
					ActualValue:   fmt.Sprintf("%d", len(d.LineItems)),
					Message:       msg,
				}}
			},
		},
		{
			ruleKey: "logic.invoice.date_not_future", ruleName: "Logical: Invoice Date Not in Future",
			severity: domain.ValidationSeverityWarning,
			validate: func(d *GSTInvoice) []ValidationResult {
				if d.Invoice.InvoiceDate == "" {
					return []ValidationResult{{
						Passed: true, FieldPath: "invoice.invoice_date",
						Message: "Logical: Invoice Date Not in Future: date missing, skipping",
					}}
				}
				invDate, err := parseDate(d.Invoice.InvoiceDate)
				if err != nil {
					return []ValidationResult{{
						Passed: true, FieldPath: "invoice.invoice_date",
						Message: "Logical: Invoice Date Not in Future: date not parseable, skipping",
					}}
				}
				today := time.Now().Truncate(24 * time.Hour)
				passed := !invDate.After(today)
				msg := "Logical: Invoice Date Not in Future: invoice date is not in the future"
				if !passed {
					msg = "Logical: Invoice Date Not in Future: invoice date is in the future"
				}
				return []ValidationResult{{
					Passed: passed, FieldPath: "invoice.invoice_date",
					ExpectedValue: fmt.Sprintf("<= %s", today.Format("2006-01-02")),
					ActualValue:   d.Invoice.InvoiceDate, Message: msg,
				}}
			},
		},
		{
			ruleKey: "logic.totals.non_negative", ruleName: "Logical: Non-Negative Totals",
			severity: domain.ValidationSeverityError,
			validate: func(d *GSTInvoice) []ValidationResult {
				amounts := map[string]float64{
					"totals.subtotal":       d.Totals.Subtotal,
					"totals.taxable_amount": d.Totals.TaxableAmount,
					"totals.cgst":           d.Totals.CGST,
					"totals.sgst":           d.Totals.SGST,
					"totals.igst":           d.Totals.IGST,
					"totals.total":          d.Totals.Total,
				}
				results := make([]ValidationResult, 0, len(amounts))
				for fp, val := range amounts {
					passed := val >= 0
					msg := fmt.Sprintf("Logical: Non-Negative Totals: %s is non-negative", fp)
					if !passed {
						msg = fmt.Sprintf("Logical: Non-Negative Totals: %s is negative (%.2f)", fp, val)
					}
					results = append(results, ValidationResult{
						Passed: passed, FieldPath: fp,
						ExpectedValue: ">= 0", ActualValue: fmtf(val), Message: msg,
					})
				}
				return results
			},
		},
		{
			ruleKey: "logic.invoice.type_valid", ruleName: "Logical: Valid Invoice Type",
			severity: domain.ValidationSeverityWarning,
			validate: func(d *GSTInvoice) []ValidationResult {
				if d.Invoice.InvoiceType == "" {
					return []ValidationResult{{
						Passed: true, FieldPath: "invoice.invoice_type",
						Message: "Logical: Valid Invoice Type: field is empty, skipping",
					}}
				}
				normalized := strings.ToLower(strings.TrimSpace(d.Invoice.InvoiceType))
				normalized = strings.ReplaceAll(normalized, "_", " ")
				validTypes := map[string]bool{
					"regular":         true,
					"sez":             true,
					"export":          true,
					"deemed export":   true,
					"bill of supply":  true,
					"tax invoice":     true,
					"credit note":     true,
					"debit note":      true,
				}
				passed := validTypes[normalized]
				msg := "Logical: Valid Invoice Type: invoice type is recognized"
				if !passed {
					msg = fmt.Sprintf("Logical: Valid Invoice Type: '%s' is not a recognized invoice type", d.Invoice.InvoiceType)
				}
				return []ValidationResult{{
					Passed:        passed,
					FieldPath:     "invoice.invoice_type",
					ExpectedValue: "one of {Regular, SEZ, Export, Deemed Export, Bill of Supply, Tax Invoice, Credit Note, Debit Note}",
					ActualValue:   d.Invoice.InvoiceType,
					Message:       msg,
				}}
			},
		},
	}
}
