package parser

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sync"

	"satvos/internal/port"
	"satvos/internal/validator/invoice"
)

var gstinRe = regexp.MustCompile(`^\d{2}[A-Z]{5}\d{4}[A-Z][1-9A-Z]Z[0-9A-Z]$`)
var irnRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

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
		return sResult.output, nil
	}

	// Only primary succeeded
	if sResult.err != nil {
		log.Printf("parser.MergeParser: secondary parser failed (%v), using primary only", sResult.err)
		pResult.output.FieldProvenance = map[string]string{"_source": "primary_only"}
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
		return secondary, nil
	}
	if err := json.Unmarshal(secondary.StructuredData, &sData); err != nil {
		log.Printf("parser.MergeParser: WARNING: failed to unmarshal secondary structured data (%s), falling back to primary", err)
		primary.FieldProvenance = map[string]string{"_source": "primary_only", "_reason": "secondary_unmarshal_failed"}
		return primary, nil
	}

	var pConf, sConf invoice.ConfidenceScores
	_ = json.Unmarshal(primary.ConfidenceScores, &pConf)
	_ = json.Unmarshal(secondary.ConfidenceScores, &sConf)

	provenance := make(map[string]string)
	merged := pData // start with primary

	// Merge scalar invoice fields
	mergeString(&merged.Invoice.InvoiceNumber, sData.Invoice.InvoiceNumber, &pConf.Invoice.InvoiceNumber, sConf.Invoice.InvoiceNumber, "invoice.invoice_number", provenance, nil)
	mergeString(&merged.Invoice.InvoiceDate, sData.Invoice.InvoiceDate, &pConf.Invoice.InvoiceDate, sConf.Invoice.InvoiceDate, "invoice.invoice_date", provenance, nil)
	mergeString(&merged.Invoice.DueDate, sData.Invoice.DueDate, &pConf.Invoice.DueDate, sConf.Invoice.DueDate, "invoice.due_date", provenance, nil)
	mergeString(&merged.Invoice.InvoiceType, sData.Invoice.InvoiceType, &pConf.Invoice.InvoiceType, sConf.Invoice.InvoiceType, "invoice.invoice_type", provenance, nil)
	mergeString(&merged.Invoice.Currency, sData.Invoice.Currency, &pConf.Invoice.Currency, sConf.Invoice.Currency, "invoice.currency", provenance, nil)
	mergeString(&merged.Invoice.PlaceOfSupply, sData.Invoice.PlaceOfSupply, &pConf.Invoice.PlaceOfSupply, sConf.Invoice.PlaceOfSupply, "invoice.place_of_supply", provenance, nil)
	mergeString(&merged.Invoice.IRN, sData.Invoice.IRN, &pConf.Invoice.IRN, sConf.Invoice.IRN, "invoice.irn", provenance, irnRe)
	mergeString(&merged.Invoice.AcknowledgementNumber, sData.Invoice.AcknowledgementNumber, &pConf.Invoice.AcknowledgementNumber, sConf.Invoice.AcknowledgementNumber, "invoice.acknowledgement_number", provenance, nil)
	mergeString(&merged.Invoice.AcknowledgementDate, sData.Invoice.AcknowledgementDate, &pConf.Invoice.AcknowledgementDate, sConf.Invoice.AcknowledgementDate, "invoice.acknowledgement_date", provenance, nil)

	// Merge seller fields
	mergeString(&merged.Seller.Name, sData.Seller.Name, &pConf.Seller.Name, sConf.Seller.Name, "seller.name", provenance, nil)
	mergeString(&merged.Seller.Address, sData.Seller.Address, &pConf.Seller.Address, sConf.Seller.Address, "seller.address", provenance, nil)
	mergeString(&merged.Seller.GSTIN, sData.Seller.GSTIN, &pConf.Seller.GSTIN, sConf.Seller.GSTIN, "seller.gstin", provenance, gstinRe)
	mergeString(&merged.Seller.PAN, sData.Seller.PAN, &pConf.Seller.PAN, sConf.Seller.PAN, "seller.pan", provenance, nil)
	mergeString(&merged.Seller.State, sData.Seller.State, &pConf.Seller.State, sConf.Seller.State, "seller.state", provenance, nil)
	mergeString(&merged.Seller.StateCode, sData.Seller.StateCode, &pConf.Seller.StateCode, sConf.Seller.StateCode, "seller.state_code", provenance, nil)

	// Merge buyer fields
	mergeString(&merged.Buyer.Name, sData.Buyer.Name, &pConf.Buyer.Name, sConf.Buyer.Name, "buyer.name", provenance, nil)
	mergeString(&merged.Buyer.Address, sData.Buyer.Address, &pConf.Buyer.Address, sConf.Buyer.Address, "buyer.address", provenance, nil)
	mergeString(&merged.Buyer.GSTIN, sData.Buyer.GSTIN, &pConf.Buyer.GSTIN, sConf.Buyer.GSTIN, "buyer.gstin", provenance, gstinRe)
	mergeString(&merged.Buyer.PAN, sData.Buyer.PAN, &pConf.Buyer.PAN, sConf.Buyer.PAN, "buyer.pan", provenance, nil)
	mergeString(&merged.Buyer.State, sData.Buyer.State, &pConf.Buyer.State, sConf.Buyer.State, "buyer.state", provenance, nil)
	mergeString(&merged.Buyer.StateCode, sData.Buyer.StateCode, &pConf.Buyer.StateCode, sConf.Buyer.StateCode, "buyer.state_code", provenance, nil)

	// Merge line items: pick the array with more items or higher avg confidence
	if len(sData.LineItems) > len(pData.LineItems) {
		merged.LineItems = sData.LineItems
		provenance["line_items"] = "secondary"
		if len(sConf.LineItems) > 0 {
			pConf.LineItems = sConf.LineItems
		}
	} else {
		provenance["line_items"] = "primary"
	}

	// Merge totals
	mergeFloat(&merged.Totals.Subtotal, sData.Totals.Subtotal, &pConf.Totals.Subtotal, sConf.Totals.Subtotal, "totals.subtotal", provenance)
	mergeFloat(&merged.Totals.TotalDiscount, sData.Totals.TotalDiscount, &pConf.Totals.TotalDiscount, sConf.Totals.TotalDiscount, "totals.total_discount", provenance)
	mergeFloat(&merged.Totals.TaxableAmount, sData.Totals.TaxableAmount, &pConf.Totals.TaxableAmount, sConf.Totals.TaxableAmount, "totals.taxable_amount", provenance)
	mergeFloat(&merged.Totals.CGST, sData.Totals.CGST, &pConf.Totals.CGST, sConf.Totals.CGST, "totals.cgst", provenance)
	mergeFloat(&merged.Totals.SGST, sData.Totals.SGST, &pConf.Totals.SGST, sConf.Totals.SGST, "totals.sgst", provenance)
	mergeFloat(&merged.Totals.IGST, sData.Totals.IGST, &pConf.Totals.IGST, sConf.Totals.IGST, "totals.igst", provenance)
	mergeFloat(&merged.Totals.Cess, sData.Totals.Cess, &pConf.Totals.Cess, sConf.Totals.Cess, "totals.cess", provenance)
	mergeFloat(&merged.Totals.RoundOff, sData.Totals.RoundOff, &pConf.Totals.RoundOff, sConf.Totals.RoundOff, "totals.round_off", provenance)
	mergeFloat(&merged.Totals.Total, sData.Totals.Total, &pConf.Totals.Total, sConf.Totals.Total, "totals.total", provenance)

	// Merge payment
	mergeString(&merged.Payment.BankName, sData.Payment.BankName, &pConf.Payment.BankName, sConf.Payment.BankName, "payment.bank_name", provenance, nil)
	mergeString(&merged.Payment.AccountNumber, sData.Payment.AccountNumber, &pConf.Payment.AccountNumber, sConf.Payment.AccountNumber, "payment.account_number", provenance, nil)
	mergeString(&merged.Payment.IFSCCode, sData.Payment.IFSCCode, &pConf.Payment.IFSCCode, sConf.Payment.IFSCCode, "payment.ifsc_code", provenance, nil)
	mergeString(&merged.Payment.PaymentTerms, sData.Payment.PaymentTerms, &pConf.Payment.PaymentTerms, sConf.Payment.PaymentTerms, "payment.payment_terms", provenance, nil)

	mergedData, _ := json.Marshal(merged)
	mergedConf, _ := json.Marshal(pConf)

	return &port.ParseOutput{
		StructuredData:   mergedData,
		ConfidenceScores: mergedConf,
		ModelUsed:        primary.ModelUsed,
		PromptUsed:       primary.PromptUsed,
		FieldProvenance:  provenance,
		SecondaryModel:   secondary.ModelUsed,
	}, nil
}

// mergeString implements the merge strategy for scalar string fields.
func mergeString(pVal *string, sVal string, pConf *float64, sConf float64, fieldPath string, provenance map[string]string, formatRe *regexp.Regexp) {
	if *pVal == sVal {
		// Agreement: boost confidence
		if *pConf < 1.0 {
			boosted := *pConf + (1.0-*pConf)*0.2
			if boosted > 1.0 {
				boosted = 1.0
			}
			*pConf = boosted
		}
		provenance[fieldPath] = "agree"
		return
	}

	if *pVal == "" && sVal != "" {
		*pVal = sVal
		*pConf = sConf
		provenance[fieldPath] = "secondary"
		return
	}

	if sVal == "" {
		provenance[fieldPath] = "primary"
		return
	}

	// Disagreement: prefer value matching expected format
	if formatRe != nil {
		pMatch := formatRe.MatchString(*pVal)
		sMatch := formatRe.MatchString(sVal)
		if sMatch && !pMatch {
			*pVal = sVal
			*pConf = sConf * 0.8
			provenance[fieldPath] = "secondary_format"
			return
		}
		if pMatch && !sMatch {
			*pConf *= 0.8
			provenance[fieldPath] = "primary_format"
			return
		}
	}

	// Both disagree, keep primary but reduce confidence
	*pConf *= 0.6
	provenance[fieldPath] = "disagreement"
}

// mergeFloat implements the merge strategy for scalar float64 fields.
func mergeFloat(pVal *float64, sVal float64, pConf *float64, sConf float64, fieldPath string, provenance map[string]string) {
	if *pVal == sVal {
		if *pConf < 1.0 {
			boosted := *pConf + (1.0-*pConf)*0.2
			if boosted > 1.0 {
				boosted = 1.0
			}
			*pConf = boosted
		}
		provenance[fieldPath] = "agree"
		return
	}

	if *pVal == 0 && sVal != 0 {
		*pVal = sVal
		*pConf = sConf
		provenance[fieldPath] = "secondary"
		return
	}

	if sVal == 0 {
		provenance[fieldPath] = "primary"
		return
	}

	// Disagreement: keep primary, reduce confidence
	*pConf *= 0.6
	provenance[fieldPath] = "disagreement"
}
