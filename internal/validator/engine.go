package validator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/port"
	"satvos/internal/validator/invoice"
)

const defaultRuleCacheTTL = 5 * time.Minute

type ruleCacheEntry struct {
	rules     []domain.DocumentValidationRule
	expiresAt time.Time
}

// ValidationResultEntry represents a single validation result stored in the JSONB array.
type ValidationResultEntry struct {
	RuleID                 uuid.UUID       `json:"rule_id"`
	Passed                 bool            `json:"passed"`
	FieldPath              string          `json:"field_path"`
	ExpectedValue          string          `json:"expected_value"`
	ActualValue            string          `json:"actual_value"`
	Message                string          `json:"message"`
	ReconciliationCritical bool            `json:"reconciliation_critical"`
	ValidatedAt            time.Time       `json:"validated_at"`
	Metadata               json.RawMessage `json:"metadata,omitempty"`
}

// Engine orchestrates document validation.
type Engine struct {
	registry  *Registry
	ruleRepo  port.DocumentValidationRuleRepository
	docRepo   port.DocumentRepository
	ruleCache map[string]ruleCacheEntry
	cacheMu   sync.RWMutex
}

// NewEngine creates a new validation engine.
func NewEngine(
	registry *Registry,
	ruleRepo port.DocumentValidationRuleRepository,
	docRepo port.DocumentRepository,
) *Engine {
	return &Engine{
		registry:  registry,
		ruleRepo:  ruleRepo,
		docRepo:   docRepo,
		ruleCache: make(map[string]ruleCacheEntry),
	}
}

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

// ValidateDocument runs all applicable validation rules against a document.
func (e *Engine) ValidateDocument(ctx context.Context, tenantID, docID uuid.UUID) error {
	doc, err := e.docRepo.GetByID(ctx, tenantID, docID)
	if err != nil {
		return fmt.Errorf("getting document: %w", err)
	}

	// Ensure built-in rules exist for this tenant/document type
	if err := e.EnsureBuiltinRules(ctx, tenantID, doc.DocumentType, doc.CreatedBy); err != nil {
		return fmt.Errorf("ensuring builtin rules: %w", err)
	}

	// Load all active rules (with cache)
	var collectionID *uuid.UUID
	if doc.CollectionID != (uuid.UUID{}) {
		collectionID = &doc.CollectionID
	}
	cacheKey := ruleCacheKey(tenantID, doc.DocumentType, collectionID)
	rules, cached := e.getCachedRules(cacheKey)
	if !cached {
		rules, err = e.ruleRepo.ListByDocumentType(ctx, tenantID, doc.DocumentType, collectionID)
		if err != nil {
			return fmt.Errorf("loading rules: %w", err)
		}
		e.setCachedRules(cacheKey, rules)
	}

	// Parse structured data into typed struct
	var inv invoice.GSTInvoice
	if err := json.Unmarshal(doc.StructuredData, &inv); err != nil {
		return fmt.Errorf("unmarshaling structured_data: %w", err)
	}

	ctx = invoice.WithValidationContext(ctx, tenantID, docID)

	// Run validators and collect results
	now := time.Now().UTC()
	var allResults []ValidationResultEntry
	hasError := false
	hasWarning := false
	hasReconError := false
	hasReconWarning := false

	for idx := range rules {
		rule := &rules[idx]
		if rule.IsBuiltin && rule.BuiltinRuleKey != nil {
			v := e.registry.Get(*rule.BuiltinRuleKey)
			if v == nil {
				log.Printf("validator.Engine: no validator registered for builtin key %q", *rule.BuiltinRuleKey)
				continue
			}
			vResults, panicked := safeValidate(ctx, v, &inv, *rule.BuiltinRuleKey)
			if panicked {
				allResults = append(allResults, ValidationResultEntry{
					RuleID:                 rule.ID,
					Passed:                 false,
					FieldPath:              "",
					ExpectedValue:          "",
					ActualValue:            "",
					Message:                fmt.Sprintf("validator panicked: %s", *rule.BuiltinRuleKey),
					ReconciliationCritical: rule.ReconciliationCritical,
					ValidatedAt:            now,
				})
				if rule.Severity == domain.ValidationSeverityError {
					hasError = true
				} else {
					hasWarning = true
				}
				if rule.ReconciliationCritical {
					if rule.Severity == domain.ValidationSeverityError {
						hasReconError = true
					} else {
						hasReconWarning = true
					}
				}
				continue
			}
			for _, vr := range vResults {
				allResults = append(allResults, ValidationResultEntry{
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
					if rule.Severity == domain.ValidationSeverityError {
						hasError = true
					} else {
						hasWarning = true
					}
					if rule.ReconciliationCritical {
						if rule.Severity == domain.ValidationSeverityError {
							hasReconError = true
						} else {
							hasReconWarning = true
						}
					}
				}
			}
		}
		// Custom (non-builtin) rules are skipped for now — extensible via CustomRuleExecutor.
	}

	// Marshal results to JSON
	resultsJSON, err := json.Marshal(allResults)
	if err != nil {
		return fmt.Errorf("marshaling validation results: %w", err)
	}

	// Compute validation_status
	var status domain.ValidationStatus
	switch {
	case hasError:
		status = domain.ValidationStatusInvalid
	case hasWarning:
		status = domain.ValidationStatusWarning
	default:
		status = domain.ValidationStatusValid
	}

	// Compute reconciliation_status
	var reconStatus domain.ReconciliationStatus
	switch {
	case hasReconError:
		reconStatus = domain.ReconciliationStatusInvalid
	case hasReconWarning:
		reconStatus = domain.ReconciliationStatusWarning
	default:
		reconStatus = domain.ReconciliationStatusValid
	}

	doc.ValidationStatus = status
	doc.ValidationResults = resultsJSON
	doc.ReconciliationStatus = reconStatus
	if err := e.docRepo.UpdateValidationResults(ctx, doc); err != nil {
		return fmt.Errorf("updating validation results: %w", err)
	}

	log.Printf("validator.Engine: document %s validated — status=%s, reconciliation=%s, results=%d", docID, status, reconStatus, len(allResults))
	return nil
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

// EnsureBuiltinRules lazy-seeds all built-in rules for a tenant+document type combination.
func (e *Engine) EnsureBuiltinRules(ctx context.Context, tenantID uuid.UUID, documentType string, createdBy uuid.UUID) error {
	existing, err := e.ruleRepo.ListBuiltinKeys(ctx, tenantID, documentType)
	if err != nil {
		return fmt.Errorf("listing existing builtin keys: %w", err)
	}

	existingSet := make(map[string]bool, len(existing))
	for _, key := range existing {
		existingSet[key] = true
	}

	seeded := false
	for _, v := range e.registry.All() {
		if existingSet[v.RuleKey()] {
			continue
		}
		key := v.RuleKey()
		rule := &domain.DocumentValidationRule{
			ID:                     uuid.New(),
			TenantID:               tenantID,
			DocumentType:           documentType,
			RuleName:               v.RuleName(),
			RuleType:               v.RuleType(),
			RuleConfig:             json.RawMessage("{}"),
			Severity:               v.Severity(),
			IsActive:               true,
			IsBuiltin:              true,
			BuiltinRuleKey:         &key,
			ReconciliationCritical: v.ReconciliationCritical(),
			CreatedBy:              createdBy,
		}
		if err := e.ruleRepo.Create(ctx, rule); err != nil {
			return fmt.Errorf("seeding builtin rule %s: %w", v.RuleKey(), err)
		}
		seeded = true
	}

	// Invalidate cache if new rules were seeded
	if seeded {
		e.invalidateCacheForTenant(tenantID)
	}

	return nil
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

// GetValidation loads validation results and computes field statuses for a document.
func (e *Engine) GetValidation(ctx context.Context, tenantID, docID uuid.UUID) (*ValidationResponse, error) {
	doc, err := e.docRepo.GetByID(ctx, tenantID, docID)
	if err != nil {
		return nil, err
	}

	// Unmarshal validation results from the document's JSONB column
	var results []ValidationResultEntry
	if len(doc.ValidationResults) > 0 {
		if err := json.Unmarshal(doc.ValidationResults, &results); err != nil {
			return nil, fmt.Errorf("unmarshaling validation results: %w", err)
		}
	}

	// Load rules for looking up severity
	var collectionID *uuid.UUID
	if doc.CollectionID != (uuid.UUID{}) {
		collectionID = &doc.CollectionID
	}
	rulesList, err := e.ruleRepo.ListByDocumentType(ctx, tenantID, doc.DocumentType, collectionID)
	if err != nil {
		return nil, fmt.Errorf("loading rules: %w", err)
	}
	rulesMap := make(map[string]*domain.DocumentValidationRule, len(rulesList))
	for i := range rulesList {
		rulesMap[rulesList[i].ID.String()] = &rulesList[i]
	}

	// Parse confidence scores
	confidenceMap := flattenConfidenceScores(doc.ConfidenceScores)

	// Compute field statuses
	fieldStatuses := ComputeFieldStatuses(results, rulesMap, confidenceMap)

	// Build summary
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

	// Build result items for response
	resultItems := make([]ValidationResultItem, 0, len(results))
	for idx := range results {
		r := &results[idx]
		rule := rulesMap[r.RuleID.String()]
		item := ValidationResultItem{
			RuleName:               "",
			RuleType:               "",
			Severity:               "",
			Passed:                 r.Passed,
			FieldPath:              r.FieldPath,
			ExpectedValue:          r.ExpectedValue,
			ActualValue:            r.ActualValue,
			Message:                r.Message,
			ReconciliationCritical: r.ReconciliationCritical,
			Metadata:               r.Metadata,
		}
		if rule != nil {
			item.RuleName = rule.RuleName
			item.RuleType = string(rule.RuleType)
			item.Severity = string(rule.Severity)
		}
		resultItems = append(resultItems, item)
	}

	return &ValidationResponse{
		DocumentID:       docID,
		ValidationStatus: doc.ValidationStatus,
		Summary: ValidationSummary{
			Total:    len(results),
			Passed:   passed,
			Errors:   errorCount,
			Warnings: warningCount,
		},
		ReconciliationStatus: doc.ReconciliationStatus,
		ReconciliationSummary: ReconciliationSummary{
			Total:    reconPassed + reconErrors + reconWarnings,
			Passed:   reconPassed,
			Errors:   reconErrors,
			Warnings: reconWarnings,
		},
		Results:       resultItems,
		FieldStatuses: fieldStatuses,
	}, nil
}

// ValidationResponse is the API response for GET /documents/:id/validation.
type ValidationResponse struct {
	DocumentID            uuid.UUID                    `json:"document_id"`
	ValidationStatus      domain.ValidationStatus      `json:"validation_status"`
	Summary               ValidationSummary            `json:"summary"`
	ReconciliationStatus  domain.ReconciliationStatus  `json:"reconciliation_status"`
	ReconciliationSummary ReconciliationSummary        `json:"reconciliation_summary"`
	Results               []ValidationResultItem       `json:"results"`
	FieldStatuses         map[string]*FieldStatus      `json:"field_statuses"`
}

// ValidationSummary holds aggregate counts of validation results.
type ValidationSummary struct {
	Total    int `json:"total"`
	Passed   int `json:"passed"`
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

// ReconciliationSummary holds aggregate counts for reconciliation-critical rules only.
type ReconciliationSummary struct {
	Total    int `json:"total"`
	Passed   int `json:"passed"`
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

// ValidationResultItem is a single validation result in the API response.
type ValidationResultItem struct {
	RuleName               string          `json:"rule_name"`
	RuleType               string          `json:"rule_type"`
	Severity               string          `json:"severity"`
	Passed                 bool            `json:"passed"`
	FieldPath              string          `json:"field_path"`
	ExpectedValue          string          `json:"expected_value"`
	ActualValue            string          `json:"actual_value"`
	Message                string          `json:"message"`
	ReconciliationCritical bool            `json:"reconciliation_critical"`
	Metadata               json.RawMessage `json:"metadata,omitempty"`
}

// flattenConfidenceScores converts the nested confidence JSON into a flat map of field_path → confidence.
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
