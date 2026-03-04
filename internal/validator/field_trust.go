package validator

import (
	"fmt"

	"satvos/internal/domain"
	"satvos/internal/validator/invoice"
)

// FieldTrustEntry holds the computed trust score and reasons for a single field.
type FieldTrustEntry struct {
	Trust   float64  `json:"trust"`
	Reasons []string `json:"reasons"`
}

// ComputeFieldTrust computes trust scores for all fields based on provenance,
// validation results, and format matching.
func ComputeFieldTrust(
	provenance map[string]string,
	results []ValidationResultEntry,
	rules map[string]*domain.DocumentValidationRule,
	inv *invoice.GSTInvoice,
) map[string]*FieldTrustEntry {
	trustMap := make(map[string]*FieldTrustEntry)

	// Collect all known field paths and their string values
	fieldValues := extractFieldValues(inv)

	// Group validation results by field path
	type resultInfo struct {
		passed   bool
		severity domain.ValidationSeverity
	}
	fieldResults := make(map[string][]resultInfo)
	for idx := range results {
		r := &results[idx]
		rule := rules[r.RuleID.String()]
		if rule == nil {
			continue
		}
		fieldResults[r.FieldPath] = append(fieldResults[r.FieldPath], resultInfo{
			passed:   r.Passed,
			severity: rule.Severity,
		})
	}

	// Compute trust for each known field
	for fieldPath, value := range fieldValues {
		entry := &FieldTrustEntry{}

		// 1. Empty field → trust=0.0
		if value == "" {
			entry.Trust = 0.0
			entry.Reasons = []string{"field_empty"}
			trustMap[fieldPath] = entry
			continue
		}

		// 2. Provenance base
		prov, hasProv := provenance[fieldPath]
		switch {
		case prov == "manual_edit":
			entry.Trust = 1.0
			entry.Reasons = append(entry.Reasons, "manual_edit")
		case prov == "agree":
			entry.Trust = 0.85
			entry.Reasons = append(entry.Reasons, "dual_parse_agree")
		case prov == "primary_format" || prov == "secondary_format":
			entry.Trust = 0.7
			entry.Reasons = append(entry.Reasons, "format_resolved")
		case prov == "primary" || prov == "secondary":
			entry.Trust = 0.6
			entry.Reasons = append(entry.Reasons, "single_source")
		case prov == "disagreement":
			entry.Trust = 0.4
			entry.Reasons = append(entry.Reasons, "dual_parse_disagree")
		case !hasProv:
			entry.Trust = 0.5
			entry.Reasons = append(entry.Reasons, "single_parse")
		default:
			entry.Trust = 0.5
			entry.Reasons = append(entry.Reasons, "unknown_provenance")
		}

		// Skip adjustments for manual_edit
		if prov == "manual_edit" {
			trustMap[fieldPath] = entry
			continue
		}

		// 3. Validation bonus/penalty
		if fieldRs, ok := fieldResults[fieldPath]; ok {
			hasError := false
			hasWarning := false
			allPassed := true
			for _, r := range fieldRs {
				if !r.passed {
					allPassed = false
					if r.severity == domain.ValidationSeverityError {
						hasError = true
					} else {
						hasWarning = true
					}
				}
			}
			switch {
			case hasError:
				entry.Trust = 0.1
				entry.Reasons = append(entry.Reasons, "validation_error")
			case hasWarning:
				entry.Trust -= 0.15
				entry.Reasons = append(entry.Reasons, "validation_warning")
			case allPassed:
				entry.Trust += 0.1
				entry.Reasons = append(entry.Reasons, "validation_passed")
			}
		}

		// 4. Format match bonus
		if checker, ok := invoice.FieldFormatCheckers[fieldPath]; ok {
			if checker(value) {
				entry.Trust += 0.05
				entry.Reasons = append(entry.Reasons, "format_match")
			}
		}

		// Clamp to [0.0, 1.0]
		if entry.Trust > 1.0 {
			entry.Trust = 1.0
		}
		if entry.Trust < 0.0 {
			entry.Trust = 0.0
		}

		trustMap[fieldPath] = entry
	}

	return trustMap
}

// extractFieldValues returns a map of field paths to their string representations
// for all scalar fields in the invoice.
func extractFieldValues(inv *invoice.GSTInvoice) map[string]string {
	fields := make(map[string]string)
	if inv == nil {
		return fields
	}

	// Invoice fields
	fields["invoice.invoice_number"] = inv.Invoice.InvoiceNumber
	fields["invoice.invoice_date"] = inv.Invoice.InvoiceDate
	fields["invoice.due_date"] = inv.Invoice.DueDate
	fields["invoice.invoice_type"] = inv.Invoice.InvoiceType
	fields["invoice.currency"] = inv.Invoice.Currency
	fields["invoice.place_of_supply"] = inv.Invoice.PlaceOfSupply
	fields["invoice.irn"] = inv.Invoice.IRN
	fields["invoice.acknowledgement_number"] = inv.Invoice.AcknowledgementNumber
	fields["invoice.acknowledgement_date"] = inv.Invoice.AcknowledgementDate
	fields["invoice.qr_code_data"] = inv.Invoice.QRCodeData

	// Seller fields
	fields["seller.name"] = inv.Seller.Name
	fields["seller.address"] = inv.Seller.Address
	fields["seller.gstin"] = inv.Seller.GSTIN
	fields["seller.pan"] = inv.Seller.PAN
	fields["seller.state"] = inv.Seller.State
	fields["seller.state_code"] = inv.Seller.StateCode

	// Buyer fields
	fields["buyer.name"] = inv.Buyer.Name
	fields["buyer.address"] = inv.Buyer.Address
	fields["buyer.gstin"] = inv.Buyer.GSTIN
	fields["buyer.pan"] = inv.Buyer.PAN
	fields["buyer.state"] = inv.Buyer.State
	fields["buyer.state_code"] = inv.Buyer.StateCode

	// Totals (float → string for emptiness check)
	addFloat(fields, "totals.subtotal", inv.Totals.Subtotal)
	addFloat(fields, "totals.total_discount", inv.Totals.TotalDiscount)
	addFloat(fields, "totals.taxable_amount", inv.Totals.TaxableAmount)
	addFloat(fields, "totals.cgst", inv.Totals.CGST)
	addFloat(fields, "totals.sgst", inv.Totals.SGST)
	addFloat(fields, "totals.igst", inv.Totals.IGST)
	addFloat(fields, "totals.cess", inv.Totals.Cess)
	addFloat(fields, "totals.round_off", inv.Totals.RoundOff)
	addFloat(fields, "totals.total", inv.Totals.Total)
	fields["totals.amount_in_words"] = inv.Totals.AmountInWords

	// Payment
	fields["payment.bank_name"] = inv.Payment.BankName
	fields["payment.account_number"] = inv.Payment.AccountNumber
	fields["payment.ifsc_code"] = inv.Payment.IFSCCode
	fields["payment.payment_terms"] = inv.Payment.PaymentTerms

	// Notes
	fields["notes"] = inv.Notes

	// Line items
	for i := range inv.LineItems {
		li := &inv.LineItems[i]
		prefix := fmt.Sprintf("line_items[%d]", i)
		fields[prefix+".description"] = li.Description
		fields[prefix+".hsn_sac_code"] = li.HSNSACCode
		addFloat(fields, prefix+".quantity", li.Quantity)
		fields[prefix+".unit"] = li.Unit
		addFloat(fields, prefix+".unit_price", li.UnitPrice)
		addFloat(fields, prefix+".discount", li.Discount)
		addFloat(fields, prefix+".taxable_amount", li.TaxableAmount)
		addFloat(fields, prefix+".cgst_rate", li.CGSTRate)
		addFloat(fields, prefix+".cgst_amount", li.CGSTAmount)
		addFloat(fields, prefix+".sgst_rate", li.SGSTRate)
		addFloat(fields, prefix+".sgst_amount", li.SGSTAmount)
		addFloat(fields, prefix+".igst_rate", li.IGSTRate)
		addFloat(fields, prefix+".igst_amount", li.IGSTAmount)
		addFloat(fields, prefix+".total", li.Total)
	}

	return fields
}

// addFloat adds a float field to the map. Zero values are represented as empty strings
// for the purpose of emptiness checks.
func addFloat(m map[string]string, key string, val float64) {
	if val == 0 {
		m[key] = ""
	} else {
		m[key] = fmt.Sprintf("%.2f", val)
	}
}

