package parser

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"regexp"
	"sync"

	"satvos/internal/port"
	"satvos/internal/validator/invoice"
)

// MergeParser wraps two DocumentParsers, runs both in parallel, and merges results.
type MergeParser struct {
	primary   port.DocumentParser
	secondary port.DocumentParser
}

// NewMergeParser creates a MergeParser from primary and secondary parsers.
func NewMergeParser(primary, secondary port.DocumentParser) *MergeParser {
	return &MergeParser{primary: primary, secondary: secondary}
}

func (m *MergeParser) Parse(ctx context.Context, input port.ParseInput) (*port.ParseOutput, error) {
	type result struct {
		output *port.ParseOutput
		err    error
	}

	var wg sync.WaitGroup
	primaryCh := make(chan result, 1)
	secondaryCh := make(chan result, 1)

	wg.Add(2)
	go func() {
		defer wg.Done()
		out, err := m.primary.Parse(ctx, input)
		primaryCh <- result{out, err}
	}()
	go func() {
		defer wg.Done()
		out, err := m.secondary.Parse(ctx, input)
		secondaryCh <- result{out, err}
	}()

	wg.Wait()
	close(primaryCh)
	close(secondaryCh)

	pResult := <-primaryCh
	sResult := <-secondaryCh

	// Both failed
	if pResult.err != nil && sResult.err != nil {
		return nil, fmt.Errorf("both parsers failed: primary: %v; secondary: %v", pResult.err, sResult.err)
	}

	// Only secondary succeeded
	if pResult.err != nil {
		log.Printf("parser.MergeParser: primary parser failed (%v), using secondary only", pResult.err)
		sResult.output.FieldProvenance = map[string]string{"_source": "secondary_only"}
		sResult.output.SecondaryModel = sResult.output.ModelUsed
		sResult.output.ConfidenceScores = json.RawMessage("{}")
		return sResult.output, nil
	}

	// Only primary succeeded
	if sResult.err != nil {
		log.Printf("parser.MergeParser: secondary parser failed (%v), using primary only", sResult.err)
		pResult.output.FieldProvenance = map[string]string{"_source": "primary_only"}
		pResult.output.ConfidenceScores = json.RawMessage("{}")
		return pResult.output, nil
	}

	// Both succeeded — merge
	return mergeOutputs(pResult.output, sResult.output)
}

func mergeOutputs(primary, secondary *port.ParseOutput) (*port.ParseOutput, error) {
	var pData, sData invoice.GSTInvoice
	if err := json.Unmarshal(primary.StructuredData, &pData); err != nil {
		log.Printf("parser.MergeParser: WARNING: failed to unmarshal primary structured data (%s), falling back to secondary", err)
		secondary.FieldProvenance = map[string]string{"_source": "secondary_only", "_reason": "primary_unmarshal_failed"}
		secondary.SecondaryModel = secondary.ModelUsed
		secondary.ConfidenceScores = json.RawMessage("{}")
		return secondary, nil
	}
	if err := json.Unmarshal(secondary.StructuredData, &sData); err != nil {
		log.Printf("parser.MergeParser: WARNING: failed to unmarshal secondary structured data (%s), falling back to primary", err)
		primary.FieldProvenance = map[string]string{"_source": "primary_only", "_reason": "secondary_unmarshal_failed"}
		primary.ConfidenceScores = json.RawMessage("{}")
		return primary, nil
	}

	provenance := make(map[string]string)
	merged := pData // start with primary

	// Merge scalar invoice fields
	mergeString(&merged.Invoice.InvoiceNumber, sData.Invoice.InvoiceNumber, "invoice.invoice_number", provenance, nil, nil)
	mergeString(&merged.Invoice.InvoiceDate, sData.Invoice.InvoiceDate, "invoice.invoice_date", provenance, nil, invoice.IsParseableDate)
	mergeString(&merged.Invoice.DueDate, sData.Invoice.DueDate, "invoice.due_date", provenance, nil, invoice.IsParseableDate)
	mergeString(&merged.Invoice.InvoiceType, sData.Invoice.InvoiceType, "invoice.invoice_type", provenance, nil, nil)
	mergeString(&merged.Invoice.Currency, sData.Invoice.Currency, "invoice.currency", provenance, nil, invoice.IsValidCurrency)
	mergeString(&merged.Invoice.PlaceOfSupply, sData.Invoice.PlaceOfSupply, "invoice.place_of_supply", provenance, nil, nil)
	mergeString(&merged.Invoice.IRN, sData.Invoice.IRN, "invoice.irn", provenance, invoice.IRNPattern, nil)
	mergeString(&merged.Invoice.AcknowledgementNumber, sData.Invoice.AcknowledgementNumber, "invoice.acknowledgement_number", provenance, nil, nil)
	mergeString(&merged.Invoice.AcknowledgementDate, sData.Invoice.AcknowledgementDate, "invoice.acknowledgement_date", provenance, nil, invoice.IsParseableDate)

	// Merge seller fields
	mergeString(&merged.Seller.Name, sData.Seller.Name, "seller.name", provenance, nil, nil)
	mergeString(&merged.Seller.Address, sData.Seller.Address, "seller.address", provenance, nil, nil)
	mergeString(&merged.Seller.GSTIN, sData.Seller.GSTIN, "seller.gstin", provenance, invoice.GSTINPattern, nil)
	mergeString(&merged.Seller.PAN, sData.Seller.PAN, "seller.pan", provenance, invoice.PANPattern, nil)
	mergeString(&merged.Seller.State, sData.Seller.State, "seller.state", provenance, nil, nil)
	mergeString(&merged.Seller.StateCode, sData.Seller.StateCode, "seller.state_code", provenance, nil, invoice.IsValidStateCode)

	// Merge buyer fields
	mergeString(&merged.Buyer.Name, sData.Buyer.Name, "buyer.name", provenance, nil, nil)
	mergeString(&merged.Buyer.Address, sData.Buyer.Address, "buyer.address", provenance, nil, nil)
	mergeString(&merged.Buyer.GSTIN, sData.Buyer.GSTIN, "buyer.gstin", provenance, invoice.GSTINPattern, nil)
	mergeString(&merged.Buyer.PAN, sData.Buyer.PAN, "buyer.pan", provenance, invoice.PANPattern, nil)
	mergeString(&merged.Buyer.State, sData.Buyer.State, "buyer.state", provenance, nil, nil)
	mergeString(&merged.Buyer.StateCode, sData.Buyer.StateCode, "buyer.state_code", provenance, nil, invoice.IsValidStateCode)

	// Merge line items: pick the array with more items
	if len(sData.LineItems) > len(pData.LineItems) {
		merged.LineItems = sData.LineItems
		provenance["line_items"] = "secondary"
	} else {
		provenance["line_items"] = "primary"
	}

	// Merge totals with math consistency checks
	mergeFloat(&merged.Totals.Subtotal, sData.Totals.Subtotal, "totals.subtotal", provenance)
	mergeFloat(&merged.Totals.TotalDiscount, sData.Totals.TotalDiscount, "totals.total_discount", provenance)
	mergeFloat(&merged.Totals.TaxableAmount, sData.Totals.TaxableAmount, "totals.taxable_amount", provenance)

	// For tax totals, prefer the value consistent with line item sums
	mergeFloatWithConsistency(&merged.Totals.CGST, sData.Totals.CGST, "totals.cgst", provenance, merged.LineItems, func(li *invoice.LineItem) float64 { return li.CGSTAmount })
	mergeFloatWithConsistency(&merged.Totals.SGST, sData.Totals.SGST, "totals.sgst", provenance, merged.LineItems, func(li *invoice.LineItem) float64 { return li.SGSTAmount })
	mergeFloatWithConsistency(&merged.Totals.IGST, sData.Totals.IGST, "totals.igst", provenance, merged.LineItems, func(li *invoice.LineItem) float64 { return li.IGSTAmount })
	mergeFloat(&merged.Totals.Cess, sData.Totals.Cess, "totals.cess", provenance)
	mergeFloat(&merged.Totals.RoundOff, sData.Totals.RoundOff, "totals.round_off", provenance)
	mergeFloat(&merged.Totals.Total, sData.Totals.Total, "totals.total", provenance)

	// Merge payment
	mergeString(&merged.Payment.BankName, sData.Payment.BankName, "payment.bank_name", provenance, nil, nil)
	mergeString(&merged.Payment.AccountNumber, sData.Payment.AccountNumber, "payment.account_number", provenance, invoice.AcctPattern, nil)
	mergeString(&merged.Payment.IFSCCode, sData.Payment.IFSCCode, "payment.ifsc_code", provenance, invoice.IFSCPattern, nil)
	mergeString(&merged.Payment.PaymentTerms, sData.Payment.PaymentTerms, "payment.payment_terms", provenance, nil, nil)

	mergedData, _ := json.Marshal(merged)

	return &port.ParseOutput{
		StructuredData:   mergedData,
		ConfidenceScores: json.RawMessage("{}"),
		ModelUsed:        primary.ModelUsed,
		PromptUsed:       primary.PromptUsed,
		FieldProvenance:  provenance,
		SecondaryModel:   secondary.ModelUsed,
	}, nil
}

// mergeString implements the merge strategy for scalar string fields.
// formatRe is a regex for format matching; formatFn is an alternative function-based check.
func mergeString(pVal *string, sVal, fieldPath string, provenance map[string]string, formatRe *regexp.Regexp, formatFn func(string) bool) {
	if *pVal == sVal {
		provenance[fieldPath] = "agree"
		return
	}

	if *pVal == "" && sVal != "" {
		*pVal = sVal
		provenance[fieldPath] = "secondary"
		return
	}

	if sVal == "" {
		provenance[fieldPath] = "primary"
		return
	}

	// Disagreement: prefer value matching expected format
	if formatRe != nil || formatFn != nil {
		pMatch := matchesFormat(*pVal, formatRe, formatFn)
		sMatch := matchesFormat(sVal, formatRe, formatFn)
		if sMatch && !pMatch {
			*pVal = sVal
			provenance[fieldPath] = "secondary_format"
			return
		}
		if pMatch && !sMatch {
			provenance[fieldPath] = "primary_format"
			return
		}
	}

	// Both disagree, keep primary
	provenance[fieldPath] = "disagreement"
}

// matchesFormat checks a value against a regex and/or function-based format check.
func matchesFormat(val string, re *regexp.Regexp, fn func(string) bool) bool {
	if re != nil && re.MatchString(val) {
		return true
	}
	if fn != nil && fn(val) {
		return true
	}
	return false
}

// mergeFloat implements the merge strategy for scalar float64 fields.
func mergeFloat(pVal *float64, sVal float64, fieldPath string, provenance map[string]string) {
	if *pVal == sVal {
		provenance[fieldPath] = "agree"
		return
	}

	if *pVal == 0 && sVal != 0 {
		*pVal = sVal
		provenance[fieldPath] = "secondary"
		return
	}

	if sVal == 0 {
		provenance[fieldPath] = "primary"
		return
	}

	// Disagreement: keep primary
	provenance[fieldPath] = "disagreement"
}

// mergeFloatWithConsistency prefers the value that matches the sum of line item fields.
func mergeFloatWithConsistency(pVal *float64, sVal float64, fieldPath string, provenance map[string]string, lineItems []invoice.LineItem, extract func(*invoice.LineItem) float64) {
	if *pVal == sVal {
		provenance[fieldPath] = "agree"
		return
	}

	if *pVal == 0 && sVal != 0 {
		*pVal = sVal
		provenance[fieldPath] = "secondary"
		return
	}

	if sVal == 0 {
		provenance[fieldPath] = "primary"
		return
	}

	// Disagreement: check math consistency
	if len(lineItems) > 0 {
		var sum float64
		for i := range lineItems {
			sum += extract(&lineItems[i])
		}

		pDiff := math.Abs(*pVal - sum)
		sDiff := math.Abs(sVal - sum)

		if sDiff < pDiff && sDiff <= 0.01 {
			*pVal = sVal
			provenance[fieldPath] = "secondary_format"
			return
		}
		if pDiff < sDiff && pDiff <= 0.01 {
			provenance[fieldPath] = "primary_format"
			return
		}
	}

	// Disagreement: keep primary
	provenance[fieldPath] = "disagreement"
}
