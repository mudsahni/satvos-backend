package invoice

import (
	"context"
	"fmt"
	"strings"

	"satvos/internal/domain"
	"satvos/internal/port"
)

// DuplicateInvoiceValidator returns a validator that checks whether another document in
// the same tenant already has the same (seller GSTIN + invoice number) combination.
func DuplicateInvoiceValidator(finder port.DuplicateInvoiceFinder) *BuiltinValidator {
	return &BuiltinValidator{
		key:      "logic.invoice.duplicate",
		name:     "Logical: Duplicate Invoice Detection",
		ruleType: domain.ValidationRuleCustom,
		sev:      domain.ValidationSeverityWarning,
		fn:       duplicateInvoiceValidator(finder),
	}
}

func duplicateInvoiceValidator(finder port.DuplicateInvoiceFinder) func(context.Context, *GSTInvoice) []ValidationResult {
	return func(ctx context.Context, inv *GSTInvoice) []ValidationResult {
		gstin := inv.Seller.GSTIN
		invoiceNum := inv.Invoice.InvoiceNumber

		if gstin == "" || invoiceNum == "" {
			return []ValidationResult{{
				Passed:    true,
				FieldPath: "invoice",
				Message:   "Logical: Duplicate Invoice Detection: seller GSTIN or invoice number is empty, skipping duplicate check",
			}}
		}

		tenantID, ok := TenantIDFromContext(ctx)
		if !ok {
			return []ValidationResult{{
				Passed:    true,
				FieldPath: "invoice",
				Message:   "Logical: Duplicate Invoice Detection: validation context missing, skipping duplicate check",
			}}
		}
		docID, ok := DocumentIDFromContext(ctx)
		if !ok {
			return []ValidationResult{{
				Passed:    true,
				FieldPath: "invoice",
				Message:   "Logical: Duplicate Invoice Detection: validation context missing, skipping duplicate check",
			}}
		}

		// Extract IRN (lowercased) for tier 1 matching.
		irn := strings.ToLower(inv.Invoice.IRN)

		// Derive financial year for tier 2 matching.
		// If invoice date is unparseable, fy stays empty and tier 2 is skipped.
		fy := ""
		if inv.Invoice.InvoiceDate != "" {
			derivedFY, err := DeriveFinancialYear(inv.Invoice.InvoiceDate)
			if err == nil {
				fy = derivedFY
			}
		}

		matches, err := finder.FindDuplicates(ctx, tenantID, docID, gstin, invoiceNum, fy, irn)
		if err != nil {
			return []ValidationResult{{
				Passed:    true,
				FieldPath: "invoice",
				Message:   "Logical: Duplicate Invoice Detection: duplicate check unavailable",
			}}
		}

		if len(matches) == 0 {
			return []ValidationResult{{
				Passed:        true,
				FieldPath:     "invoice",
				ExpectedValue: "no duplicate invoices",
				ActualValue:   "none found",
				Message:       "Logical: Duplicate Invoice Detection: no duplicate invoices found",
			}}
		}

		// Determine severity from match types.
		hasStrong := false
		names := make([]string, 0, len(matches))
		for idx := range matches {
			m := &matches[idx]
			if m.MatchType == "exact_irn" || m.MatchType == "strong" {
				hasStrong = true
			}
			names = append(names, fmt.Sprintf("%q [%s] (uploaded %s)", m.DocumentName, m.MatchType, m.CreatedAt.Format("2006-01-02")))
		}

		severity := "warning"
		if hasStrong {
			severity = "error"
		}

		return []ValidationResult{{
			Passed:        false,
			FieldPath:     "invoice",
			ExpectedValue: "no duplicate invoices",
			ActualValue:   fmt.Sprintf("%d duplicate(s) found [%s]", len(matches), severity),
			Message: fmt.Sprintf(
				"Logical: Duplicate Invoice Detection: invoice %s from seller %s already exists in: %s",
				invoiceNum, gstin, strings.Join(names, ", "),
			),
		}}
	}
}
