package invoice

import (
	"context"
	"fmt"
	"math"

	"satvos/internal/domain"
)

// crossFieldValidator checks relationships between different fields.
type crossFieldValidator struct {
	ruleKey       string
	ruleName      string
	severity      domain.ValidationSeverity
	reconCritical bool
	validate      func(*GSTInvoice) []ValidationResult
}

func (v *crossFieldValidator) RuleKey() string  { return v.ruleKey }
func (v *crossFieldValidator) RuleName() string { return v.ruleName }
func (v *crossFieldValidator) RuleType() domain.ValidationRuleType {
	return domain.ValidationRuleCrossField
}
func (v *crossFieldValidator) Severity() domain.ValidationSeverity { return v.severity }
func (v *crossFieldValidator) ReconciliationCritical() bool        { return v.reconCritical }

func (v *crossFieldValidator) Validate(_ context.Context, data *GSTInvoice) []ValidationResult {
	return v.validate(data)
}

// CrossFieldValidators returns all cross-field validators.
func CrossFieldValidators() []*crossFieldValidator {
	return []*crossFieldValidator{
		{
			ruleKey: "xf.seller.gstin_state", ruleName: "Cross-field: Seller GSTIN-State Match",
			severity: domain.ValidationSeverityError, reconCritical: true,
			validate: func(d *GSTInvoice) []ValidationResult {
				return gstinStateCheck("seller", d.Seller.GSTIN, d.Seller.StateCode)
			},
		},
		{
			ruleKey: "xf.buyer.gstin_state", ruleName: "Cross-field: Buyer GSTIN-State Match",
			severity: domain.ValidationSeverityError, reconCritical: true,
			validate: func(d *GSTInvoice) []ValidationResult {
				return gstinStateCheck("buyer", d.Buyer.GSTIN, d.Buyer.StateCode)
			},
		},
		{
			ruleKey: "xf.seller.gstin_pan", ruleName: "Cross-field: Seller GSTIN-PAN Match",
			severity: domain.ValidationSeverityError,
			validate: func(d *GSTInvoice) []ValidationResult {
				return gstinPANCheck("seller", d.Seller.GSTIN, d.Seller.PAN)
			},
		},
		{
			ruleKey: "xf.buyer.gstin_pan", ruleName: "Cross-field: Buyer GSTIN-PAN Match",
			severity: domain.ValidationSeverityError,
			validate: func(d *GSTInvoice) []ValidationResult {
				return gstinPANCheck("buyer", d.Buyer.GSTIN, d.Buyer.PAN)
			},
		},
		{
			ruleKey: "xf.tax_type.intrastate", ruleName: "Cross-field: Intrastate Tax Type",
			severity: domain.ValidationSeverityError, reconCritical: true,
			validate: func(d *GSTInvoice) []ValidationResult {
				if d.Seller.StateCode == "" || d.Buyer.StateCode == "" {
					return []ValidationResult{{
						Passed: true, FieldPath: "tax_type",
						Message: "Cross-field: Intrastate Tax Type: state codes missing, skipping",
					}}
				}
				if d.Seller.StateCode != d.Buyer.StateCode {
					return nil // not intrastate, skip
				}
				var results []ValidationResult
				for i := range d.LineItems {
					item := &d.LineItems[i]
					// Intrastate: CGST+SGST should be used, IGST should be 0
					cgstSgstUsed := item.CGSTRate > 0 || item.SGSTRate > 0
					igstZero := item.IGSTRate == 0 && item.IGSTAmount == 0
					passed := cgstSgstUsed && igstZero
					fp := fmt.Sprintf("line_items[%d]", i)
					msg := fmt.Sprintf("Cross-field: Intrastate Tax Type: %s uses CGST+SGST correctly", fp)
					if !passed {
						msg = fmt.Sprintf("Cross-field: Intrastate Tax Type: %s should use CGST+SGST (not IGST) for same-state transaction", fp)
					}
					results = append(results, ValidationResult{
						Passed: passed, FieldPath: fp,
						ExpectedValue: "CGST+SGST used, IGST=0",
						ActualValue:   fmt.Sprintf("CGST=%.2f, SGST=%.2f, IGST=%.2f", item.CGSTRate, item.SGSTRate, item.IGSTRate),
						Message:       msg,
					})
				}
				return results
			},
		},
		{
			ruleKey: "xf.tax_type.interstate", ruleName: "Cross-field: Interstate Tax Type",
			severity: domain.ValidationSeverityError, reconCritical: true,
			validate: func(d *GSTInvoice) []ValidationResult {
				if d.Seller.StateCode == "" || d.Buyer.StateCode == "" {
					return []ValidationResult{{
						Passed: true, FieldPath: "tax_type",
						Message: "Cross-field: Interstate Tax Type: state codes missing, skipping",
					}}
				}
				if d.Seller.StateCode == d.Buyer.StateCode {
					return nil // not interstate, skip
				}
				var results []ValidationResult
				for i := range d.LineItems {
					item := &d.LineItems[i]
					igstUsed := item.IGSTRate > 0
					cgstSgstZero := item.CGSTRate == 0 && item.CGSTAmount == 0 && item.SGSTRate == 0 && item.SGSTAmount == 0
					passed := igstUsed && cgstSgstZero
					fp := fmt.Sprintf("line_items[%d]", i)
					msg := fmt.Sprintf("Cross-field: Interstate Tax Type: %s uses IGST correctly", fp)
					if !passed {
						msg = fmt.Sprintf("Cross-field: Interstate Tax Type: %s should use IGST (not CGST+SGST) for different-state transaction", fp)
					}
					results = append(results, ValidationResult{
						Passed: passed, FieldPath: fp,
						ExpectedValue: "IGST used, CGST+SGST=0",
						ActualValue:   fmt.Sprintf("CGST=%.2f, SGST=%.2f, IGST=%.2f", item.CGSTRate, item.SGSTRate, item.IGSTRate),
						Message:       msg,
					})
				}
				return results
			},
		},
		{
			ruleKey: "xf.invoice.due_after_date", ruleName: "Cross-field: Due Date After Invoice Date",
			severity: domain.ValidationSeverityWarning,
			validate: func(d *GSTInvoice) []ValidationResult {
				if d.Invoice.InvoiceDate == "" || d.Invoice.DueDate == "" {
					return []ValidationResult{{
						Passed: true, FieldPath: "invoice.due_date",
						Message: "Cross-field: Due Date After Invoice Date: dates missing, skipping",
					}}
				}
				invDate, err1 := parseDate(d.Invoice.InvoiceDate)
				dueDate, err2 := parseDate(d.Invoice.DueDate)
				if err1 != nil || err2 != nil {
					return []ValidationResult{{
						Passed: true, FieldPath: "invoice.due_date",
						Message: "Cross-field: Due Date After Invoice Date: dates not parseable, skipping",
					}}
				}
				passed := !dueDate.Before(invDate)
				msg := "Cross-field: Due Date After Invoice Date: due date is on or after invoice date"
				if !passed {
					msg = "Cross-field: Due Date After Invoice Date: due date is before invoice date"
				}
				return []ValidationResult{{
					Passed: passed, FieldPath: "invoice.due_date",
					ExpectedValue: fmt.Sprintf(">= %s", d.Invoice.InvoiceDate),
					ActualValue:   d.Invoice.DueDate, Message: msg,
				}}
			},
		},
		{
			ruleKey: "xf.parties.different_gstin", ruleName: "Cross-field: Different Party GSTINs",
			severity: domain.ValidationSeverityWarning,
			validate: func(d *GSTInvoice) []ValidationResult {
				if d.Seller.GSTIN == "" || d.Buyer.GSTIN == "" {
					return []ValidationResult{{
						Passed: true, FieldPath: "seller.gstin",
						Message: "Cross-field: Different Party GSTINs: GSTINs missing, skipping",
					}}
				}
				passed := d.Seller.GSTIN != d.Buyer.GSTIN
				msg := "Cross-field: Different Party GSTINs: seller and buyer have different GSTINs"
				if !passed {
					msg = "Cross-field: Different Party GSTINs: seller and buyer have the same GSTIN"
				}
				return []ValidationResult{{
					Passed: passed, FieldPath: "seller.gstin",
					ExpectedValue: "seller.gstin != buyer.gstin",
					ActualValue:   fmt.Sprintf("seller=%s, buyer=%s", d.Seller.GSTIN, d.Buyer.GSTIN),
					Message:       msg,
				}}
			},
		},
		{
			ruleKey: "xf.seller.state_name_code", ruleName: "Cross-field: Seller State Name-Code Match",
			severity: domain.ValidationSeverityWarning,
			validate: func(d *GSTInvoice) []ValidationResult {
				return stateNameCodeCheck("seller", d.Seller.State, d.Seller.StateCode)
			},
		},
		{
			ruleKey: "xf.buyer.state_name_code", ruleName: "Cross-field: Buyer State Name-Code Match",
			severity: domain.ValidationSeverityWarning,
			validate: func(d *GSTInvoice) []ValidationResult {
				return stateNameCodeCheck("buyer", d.Buyer.State, d.Buyer.StateCode)
			},
		},
		{
			ruleKey: "xf.totals.amount_in_words", ruleName: "Cross-field: Amount in Words Match",
			severity: domain.ValidationSeverityWarning,
			validate: func(d *GSTInvoice) []ValidationResult {
				if d.Totals.AmountInWords == "" || d.Totals.Total == 0 {
					return []ValidationResult{{
						Passed: true, FieldPath: "totals.amount_in_words",
						Message: "Cross-field: Amount in Words Match: fields missing, skipping",
					}}
				}
				parsed, err := ParseIndianAmountWords(d.Totals.AmountInWords)
				if err != nil {
					return []ValidationResult{{
						Passed: true, FieldPath: "totals.amount_in_words",
						Message: "Cross-field: Amount in Words Match: could not parse amount in words, skipping",
					}}
				}
				diff := math.Abs(parsed - d.Totals.Total)
				passed := diff <= 1.0
				msg := "Cross-field: Amount in Words Match: amount in words matches total"
				if !passed {
					msg = fmt.Sprintf("Cross-field: Amount in Words Match: parsed %.2f from words but total is %.2f", parsed, d.Totals.Total)
				}
				return []ValidationResult{{
					Passed:        passed,
					FieldPath:     "totals.amount_in_words",
					ExpectedValue: fmt.Sprintf("%.2f (±1.00)", d.Totals.Total),
					ActualValue:   fmt.Sprintf("%.2f", parsed),
					Message:       msg,
				}}
			},
		},
	}
}

func gstinStateCheck(party, gstin, stateCode string) []ValidationResult {
	fieldPath := fmt.Sprintf("%s.gstin", party)
	if gstin == "" || stateCode == "" {
		return []ValidationResult{{
			Passed: true, FieldPath: fieldPath,
			Message: fmt.Sprintf("Cross-field: %s GSTIN-State Match: fields missing, skipping", party),
		}}
	}
	// Normalize state_code to 2-digit for comparison
	if len(stateCode) == 1 {
		stateCode = "0" + stateCode
	}
	if len(gstin) < 2 {
		return []ValidationResult{{
			Passed: false, FieldPath: fieldPath,
			ExpectedValue: fmt.Sprintf("GSTIN[0:2] == %s", stateCode),
			ActualValue:   gstin,
			Message:       fmt.Sprintf("Cross-field: %s GSTIN-State Match: GSTIN too short", party),
		}}
	}
	gstinState := gstin[:2]
	passed := gstinState == stateCode
	msg := fmt.Sprintf("Cross-field: %s GSTIN-State Match: GSTIN state code matches", party)
	if !passed {
		msg = fmt.Sprintf("Cross-field: %s GSTIN-State Match: GSTIN prefix %s does not match state_code %s", party, gstinState, stateCode)
	}
	return []ValidationResult{{
		Passed: passed, FieldPath: fieldPath,
		ExpectedValue: fmt.Sprintf("GSTIN[0:2] == %s", stateCode),
		ActualValue:   gstinState, Message: msg,
	}}
}

func gstinPANCheck(party, gstin, pan string) []ValidationResult {
	fieldPath := fmt.Sprintf("%s.gstin", party)
	if gstin == "" || pan == "" {
		return []ValidationResult{{
			Passed: true, FieldPath: fieldPath,
			Message: fmt.Sprintf("Cross-field: %s GSTIN-PAN Match: fields missing, skipping", party),
		}}
	}
	if len(gstin) < 12 {
		return []ValidationResult{{
			Passed: false, FieldPath: fieldPath,
			ExpectedValue: fmt.Sprintf("GSTIN[2:12] == %s", pan),
			ActualValue:   gstin,
			Message:       fmt.Sprintf("Cross-field: %s GSTIN-PAN Match: GSTIN too short", party),
		}}
	}
	gstinPAN := gstin[2:12]
	passed := gstinPAN == pan
	msg := fmt.Sprintf("Cross-field: %s GSTIN-PAN Match: GSTIN contains matching PAN", party)
	if !passed {
		msg = fmt.Sprintf("Cross-field: %s GSTIN-PAN Match: GSTIN[2:12] %s does not match PAN %s", party, gstinPAN, pan)
	}
	return []ValidationResult{{
		Passed: passed, FieldPath: fieldPath,
		ExpectedValue: fmt.Sprintf("GSTIN[2:12] == %s", pan),
		ActualValue:   gstinPAN, Message: msg,
	}}
}
