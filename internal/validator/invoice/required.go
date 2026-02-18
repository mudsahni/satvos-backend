package invoice

import (
	"context"
	"encoding/json"
	"fmt"

	"satvos/internal/domain"
)

// requiredFieldValidator checks that a required field is not empty.
type requiredFieldValidator struct {
	ruleKey       string
	ruleName      string
	fieldPath     string
	severity      domain.ValidationSeverity
	reconCritical bool
	extract       func(*GSTInvoice) string
	perItem       bool // true for line-item level checks
	extractItem   func(*LineItem) string
}

func (v *requiredFieldValidator) RuleKey() string  { return v.ruleKey }
func (v *requiredFieldValidator) RuleName() string { return v.ruleName }
func (v *requiredFieldValidator) RuleType() domain.ValidationRuleType {
	return domain.ValidationRuleRequired
}
func (v *requiredFieldValidator) Severity() domain.ValidationSeverity { return v.severity }
func (v *requiredFieldValidator) ReconciliationCritical() bool        { return v.reconCritical }

// ValidationResult is a local alias to avoid import cycles.
type ValidationResult struct {
	Passed        bool
	FieldPath     string
	ExpectedValue string
	ActualValue   string
	Message       string
	Metadata      json.RawMessage // optional structured data (e.g. duplicate match details)
}

func (v *requiredFieldValidator) Validate(_ context.Context, data *GSTInvoice) []ValidationResult {
	if v.perItem {
		results := make([]ValidationResult, 0, len(data.LineItems))
		for i := range data.LineItems {
			item := &data.LineItems[i]
			val := v.extractItem(item)
			fieldPath := fmt.Sprintf("line_items[%d].%s", i, stripPrefix(v.fieldPath))
			results = append(results, ValidationResult{
				Passed:        val != "",
				FieldPath:     fieldPath,
				ExpectedValue: "non-empty value",
				ActualValue:   val,
				Message:       fieldMessage(val != "", v.ruleName, fieldPath),
			})
		}
		return results
	}

	val := v.extract(data)
	return []ValidationResult{{
		Passed:        val != "",
		FieldPath:     v.fieldPath,
		ExpectedValue: "non-empty value",
		ActualValue:   val,
		Message:       fieldMessage(val != "", v.ruleName, v.fieldPath),
	}}
}

func fieldMessage(passed bool, ruleName, fieldPath string) string {
	if passed {
		return fmt.Sprintf("%s: %s is present", ruleName, fieldPath)
	}
	return fmt.Sprintf("%s: %s is missing or empty", ruleName, fieldPath)
}

func stripPrefix(fieldPath string) string {
	// "line_items[i].description" → "description"
	for i := len(fieldPath) - 1; i >= 0; i-- {
		if fieldPath[i] == '.' {
			return fieldPath[i+1:]
		}
	}
	return fieldPath
}

// RequiredFieldValidators returns all required field validators.
func RequiredFieldValidators() []*requiredFieldValidator {
	return []*requiredFieldValidator{
		{
			ruleKey: "req.invoice.number", ruleName: "Required: Invoice Number",
			fieldPath: "invoice.invoice_number", severity: domain.ValidationSeverityError, reconCritical: true,
			extract: func(d *GSTInvoice) string { return d.Invoice.InvoiceNumber },
		},
		{
			ruleKey: "req.invoice.date", ruleName: "Required: Invoice Date",
			fieldPath: "invoice.invoice_date", severity: domain.ValidationSeverityError, reconCritical: true,
			extract: func(d *GSTInvoice) string { return d.Invoice.InvoiceDate },
		},
		{
			ruleKey: "req.invoice.place_of_supply", ruleName: "Required: Place of Supply",
			fieldPath: "invoice.place_of_supply", severity: domain.ValidationSeverityError, reconCritical: true,
			extract: func(d *GSTInvoice) string { return d.Invoice.PlaceOfSupply },
		},
		{
			ruleKey: "req.invoice.currency", ruleName: "Required: Currency",
			fieldPath: "invoice.currency", severity: domain.ValidationSeverityWarning,
			extract: func(d *GSTInvoice) string { return d.Invoice.Currency },
		},
		{
			ruleKey: "req.seller.name", ruleName: "Required: Seller Name",
			fieldPath: "seller.name", severity: domain.ValidationSeverityError, reconCritical: true,
			extract: func(d *GSTInvoice) string { return d.Seller.Name },
		},
		{
			ruleKey: "req.seller.gstin", ruleName: "Required: Seller GSTIN",
			fieldPath: "seller.gstin", severity: domain.ValidationSeverityError, reconCritical: true,
			extract: func(d *GSTInvoice) string { return d.Seller.GSTIN },
		},
		{
			ruleKey: "req.seller.state_code", ruleName: "Required: Seller State Code",
			fieldPath: "seller.state_code", severity: domain.ValidationSeverityError,
			extract: func(d *GSTInvoice) string { return d.Seller.StateCode },
		},
		{
			ruleKey: "req.buyer.name", ruleName: "Required: Buyer Name",
			fieldPath: "buyer.name", severity: domain.ValidationSeverityError,
			extract: func(d *GSTInvoice) string { return d.Buyer.Name },
		},
		{
			ruleKey: "req.buyer.gstin", ruleName: "Required: Buyer GSTIN",
			fieldPath: "buyer.gstin", severity: domain.ValidationSeverityError, reconCritical: true,
			extract: func(d *GSTInvoice) string { return d.Buyer.GSTIN },
		},
		{
			ruleKey: "req.buyer.state_code", ruleName: "Required: Buyer State Code",
			fieldPath: "buyer.state_code", severity: domain.ValidationSeverityError,
			extract: func(d *GSTInvoice) string { return d.Buyer.StateCode },
		},
		{
			ruleKey: "req.line_item.description", ruleName: "Required: Line Item Description",
			fieldPath: "line_items[i].description", severity: domain.ValidationSeverityWarning,
			perItem: true, extractItem: func(li *LineItem) string { return li.Description },
		},
		{
			ruleKey: "req.line_item.hsn_sac", ruleName: "Required: Line Item HSN/SAC Code",
			fieldPath: "line_items[i].hsn_sac_code", severity: domain.ValidationSeverityWarning,
			perItem: true, extractItem: func(li *LineItem) string { return li.HSNSACCode },
		},
	}
}
