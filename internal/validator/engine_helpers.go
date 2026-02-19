package validator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/validator/invoice"
)

const defaultRuleCacheTTL = 5 * time.Minute

// ruleCacheKey builds a cache key for rules.
func ruleCacheKey(tenantID uuid.UUID, docType string, collectionID *uuid.UUID) string {
	cid := "none"
	if collectionID != nil {
		cid = collectionID.String()
	}
	return tenantID.String() + ":" + docType + ":" + cid
}

// getCachedRules returns cached rules if available and not expired.
func (e *Engine) getCachedRules(key string) ([]domain.DocumentValidationRule, bool) {
	e.cacheMu.RLock()
	defer e.cacheMu.RUnlock()
	entry, ok := e.ruleCache[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.rules, true
}

// setCachedRules stores rules in the cache with a TTL.
func (e *Engine) setCachedRules(key string, rules []domain.DocumentValidationRule) {
	e.cacheMu.Lock()
	defer e.cacheMu.Unlock()
	e.ruleCache[key] = ruleCacheEntry{
		rules:     rules,
		expiresAt: time.Now().Add(defaultRuleCacheTTL),
	}
}

// invalidateCacheForTenant removes all cached entries for a given tenant.
func (e *Engine) invalidateCacheForTenant(tenantID uuid.UUID) {
	prefix := tenantID.String() + ":"
	e.cacheMu.Lock()
	defer e.cacheMu.Unlock()
	for key := range e.ruleCache {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(e.ruleCache, key)
		}
	}
}

// loadDocAndRules fetches a document, ensures builtin rules exist, and loads applicable rules.
func (e *Engine) loadDocAndRules(ctx context.Context, tenantID, docID uuid.UUID) (*domain.Document, []domain.DocumentValidationRule, error) {
	doc, err := e.docRepo.GetByID(ctx, tenantID, docID)
	if err != nil {
		return nil, nil, fmt.Errorf("getting document: %w", err)
	}

	if err := e.EnsureBuiltinRules(ctx, tenantID, doc.DocumentType, doc.CreatedBy); err != nil {
		return nil, nil, fmt.Errorf("ensuring builtin rules: %w", err)
	}

	var collectionID *uuid.UUID
	if doc.CollectionID != (uuid.UUID{}) {
		collectionID = &doc.CollectionID
	}
	cacheKey := ruleCacheKey(tenantID, doc.DocumentType, collectionID)
	rules, cached := e.getCachedRules(cacheKey)
	if !cached {
		rules, err = e.ruleRepo.ListByDocumentType(ctx, tenantID, doc.DocumentType, collectionID)
		if err != nil {
			return nil, nil, fmt.Errorf("loading rules: %w", err)
		}
		e.setCachedRules(cacheKey, rules)
	}

	return doc, rules, nil
}

// validationRunResult holds the aggregated output of running all validators.
type validationRunResult struct {
	entries         []ValidationResultEntry
	hasError        bool
	hasWarning      bool
	hasReconError   bool
	hasReconWarning bool
}

// runValidators executes all applicable validators against an invoice and collects results.
func (e *Engine) runValidators(ctx context.Context, inv *invoice.GSTInvoice, rules []domain.DocumentValidationRule) validationRunResult {
	now := time.Now().UTC()
	var result validationRunResult

	for idx := range rules {
		rule := &rules[idx]
		if !rule.IsBuiltin || rule.BuiltinRuleKey == nil {
			// Custom (non-builtin) rules are skipped for now — extensible via CustomRuleExecutor.
			continue
		}

		v := e.registry.Get(*rule.BuiltinRuleKey)
		if v == nil {
			log.Printf("validator.Engine: no validator registered for builtin key %q", *rule.BuiltinRuleKey)
			continue
		}

		vResults, panicked := safeValidate(ctx, v, inv, *rule.BuiltinRuleKey)
		if panicked {
			result.entries = append(result.entries, ValidationResultEntry{
				RuleID:                 rule.ID,
				Passed:                 false,
				Message:                fmt.Sprintf("validator panicked: %s", *rule.BuiltinRuleKey),
				ReconciliationCritical: rule.ReconciliationCritical,
				ValidatedAt:            now,
			})
			trackFailure(rule, &result)
			continue
		}
		for _, vr := range vResults {
			result.entries = append(result.entries, ValidationResultEntry{
				RuleID:                 rule.ID,
				Passed:                 vr.Passed,
				FieldPath:              vr.FieldPath,
				ExpectedValue:          vr.ExpectedValue,
				ActualValue:            vr.ActualValue,
				Message:                vr.Message,
				ReconciliationCritical: rule.ReconciliationCritical,
				ValidatedAt:            now,
				Metadata:               vr.Metadata,
			})
			if !vr.Passed {
				trackFailure(rule, &result)
			}
		}
	}

	return result
}

// trackFailure updates the run result flags based on rule severity and reconciliation criticality.
func trackFailure(rule *domain.DocumentValidationRule, result *validationRunResult) {
	if rule.Severity == domain.ValidationSeverityError {
		result.hasError = true
	} else {
		result.hasWarning = true
	}
	if rule.ReconciliationCritical {
		if rule.Severity == domain.ValidationSeverityError {
			result.hasReconError = true
		} else {
			result.hasReconWarning = true
		}
	}
}

// safeValidate runs a validator with panic recovery. If the validator panics,
// it returns nil results and panicked=true.
func safeValidate(ctx context.Context, v Validator, inv *invoice.GSTInvoice, ruleKey string) (results []invoice.ValidationResult, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("validator.Engine: PANIC in validator %q: %v", ruleKey, r)
			panicked = true
		}
	}()
	return v.Validate(ctx, inv), false
}

// computeValidationStatus determines the overall validation status from error/warning flags.
func computeValidationStatus(hasError, hasWarning bool) domain.ValidationStatus {
	switch {
	case hasError:
		return domain.ValidationStatusInvalid
	case hasWarning:
		return domain.ValidationStatusWarning
	default:
		return domain.ValidationStatusValid
	}
}

// computeReconciliationStatus determines the reconciliation status from error/warning flags.
func computeReconciliationStatus(hasReconError, hasReconWarning bool) domain.ReconciliationStatus {
	switch {
	case hasReconError:
		return domain.ReconciliationStatusInvalid
	case hasReconWarning:
		return domain.ReconciliationStatusWarning
	default:
		return domain.ReconciliationStatusValid
	}
}

// buildResultItems assembles validation result items with rule metadata for the API response.
func buildResultItems(results []ValidationResultEntry, rulesMap map[string]*domain.DocumentValidationRule) []ValidationResultItem {
	items := make([]ValidationResultItem, 0, len(results))
	for idx := range results {
		r := &results[idx]
		item := ValidationResultItem{
			Passed:                 r.Passed,
			FieldPath:              r.FieldPath,
			ExpectedValue:          r.ExpectedValue,
			ActualValue:            r.ActualValue,
			Message:                r.Message,
			ReconciliationCritical: r.ReconciliationCritical,
			Metadata:               r.Metadata,
		}
		if rule := rulesMap[r.RuleID.String()]; rule != nil {
			item.RuleName = rule.RuleName
			item.RuleType = string(rule.RuleType)
			item.Severity = string(rule.Severity)
		}
		items = append(items, item)
	}
	return items
}

// buildSummaries computes aggregate counts for validation and reconciliation results.
func buildSummaries(results []ValidationResultEntry, rulesMap map[string]*domain.DocumentValidationRule) (ValidationSummary, ReconciliationSummary) {
	var passed, errorCount, warningCount int
	var reconPassed, reconErrors, reconWarnings int

	for idx := range results {
		r := &results[idx]
		if r.Passed {
			passed++
			if r.ReconciliationCritical {
				reconPassed++
			}
		} else {
			rule := rulesMap[r.RuleID.String()]
			if rule != nil && rule.Severity == domain.ValidationSeverityError {
				errorCount++
			} else {
				warningCount++
			}
			if r.ReconciliationCritical {
				if rule != nil && rule.Severity == domain.ValidationSeverityError {
					reconErrors++
				} else {
					reconWarnings++
				}
			}
		}
	}

	return ValidationSummary{
			Total:    len(results),
			Passed:   passed,
			Errors:   errorCount,
			Warnings: warningCount,
		}, ReconciliationSummary{
			Total:    reconPassed + reconErrors + reconWarnings,
			Passed:   reconPassed,
			Errors:   reconErrors,
			Warnings: reconWarnings,
		}
}

// flattenConfidenceScores converts the nested confidence JSON into a flat map of field_path to confidence.
func flattenConfidenceScores(raw json.RawMessage) map[string]float64 {
	result := make(map[string]float64)
	if len(raw) == 0 {
		return result
	}

	var scores invoice.ConfidenceScores
	if err := json.Unmarshal(raw, &scores); err != nil {
		return result
	}

	// Invoice fields
	result["invoice.invoice_number"] = scores.Invoice.InvoiceNumber
	result["invoice.invoice_date"] = scores.Invoice.InvoiceDate
	result["invoice.due_date"] = scores.Invoice.DueDate
	result["invoice.invoice_type"] = scores.Invoice.InvoiceType
	result["invoice.currency"] = scores.Invoice.Currency
	result["invoice.place_of_supply"] = scores.Invoice.PlaceOfSupply
	result["invoice.irn"] = scores.Invoice.IRN
	result["invoice.acknowledgement_number"] = scores.Invoice.AcknowledgementNumber
	result["invoice.acknowledgement_date"] = scores.Invoice.AcknowledgementDate

	// Seller fields
	result["seller.name"] = scores.Seller.Name
	result["seller.address"] = scores.Seller.Address
	result["seller.gstin"] = scores.Seller.GSTIN
	result["seller.pan"] = scores.Seller.PAN
	result["seller.state"] = scores.Seller.State
	result["seller.state_code"] = scores.Seller.StateCode

	// Buyer fields
	result["buyer.name"] = scores.Buyer.Name
	result["buyer.address"] = scores.Buyer.Address
	result["buyer.gstin"] = scores.Buyer.GSTIN
	result["buyer.pan"] = scores.Buyer.PAN
	result["buyer.state"] = scores.Buyer.State
	result["buyer.state_code"] = scores.Buyer.StateCode

	// Line items
	for i, li := range scores.LineItems {
		prefix := fmt.Sprintf("line_items[%d]", i)
		result[prefix+".description"] = li.Description
		result[prefix+".hsn_sac_code"] = li.HSNSACCode
		result[prefix+".quantity"] = li.Quantity
		result[prefix+".unit_price"] = li.UnitPrice
		result[prefix+".discount"] = li.Discount
		result[prefix+".taxable_amount"] = li.TaxableAmount
		result[prefix+".cgst_rate"] = li.CGSTRate
		result[prefix+".cgst_amount"] = li.CGSTAmount
		result[prefix+".sgst_rate"] = li.SGSTRate
		result[prefix+".sgst_amount"] = li.SGSTAmount
		result[prefix+".igst_rate"] = li.IGSTRate
		result[prefix+".igst_amount"] = li.IGSTAmount
		result[prefix+".total"] = li.Total
	}

	// Totals
	result["totals.subtotal"] = scores.Totals.Subtotal
	result["totals.total_discount"] = scores.Totals.TotalDiscount
	result["totals.taxable_amount"] = scores.Totals.TaxableAmount
	result["totals.cgst"] = scores.Totals.CGST
	result["totals.sgst"] = scores.Totals.SGST
	result["totals.igst"] = scores.Totals.IGST
	result["totals.cess"] = scores.Totals.Cess
	result["totals.round_off"] = scores.Totals.RoundOff
	result["totals.total"] = scores.Totals.Total

	// Payment
	result["payment.bank_name"] = scores.Payment.BankName
	result["payment.account_number"] = scores.Payment.AccountNumber
	result["payment.ifsc_code"] = scores.Payment.IFSCCode
	result["payment.payment_terms"] = scores.Payment.PaymentTerms

	return result
}
