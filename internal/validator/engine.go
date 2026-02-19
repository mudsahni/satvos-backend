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

// ValidateDocument runs all applicable validation rules against a document.
func (e *Engine) ValidateDocument(ctx context.Context, tenantID, docID uuid.UUID) error {
	doc, rules, err := e.loadDocAndRules(ctx, tenantID, docID)
	if err != nil {
		return err
	}

	var inv invoice.GSTInvoice
	if err := json.Unmarshal(doc.StructuredData, &inv); err != nil {
		return fmt.Errorf("unmarshaling structured_data: %w", err)
	}

	ctx = invoice.WithValidationContext(ctx, tenantID, docID)
	result := e.runValidators(ctx, &inv, rules)

	resultsJSON, err := json.Marshal(result.entries)
	if err != nil {
		return fmt.Errorf("marshaling validation results: %w", err)
	}

	doc.ValidationStatus = computeValidationStatus(result.hasError, result.hasWarning)
	doc.ValidationResults = resultsJSON
	doc.ReconciliationStatus = computeReconciliationStatus(result.hasReconError, result.hasReconWarning)
	if err := e.docRepo.UpdateValidationResults(ctx, doc); err != nil {
		return fmt.Errorf("updating validation results: %w", err)
	}

	log.Printf("validator.Engine: document %s validated — status=%s, reconciliation=%s, results=%d", docID, doc.ValidationStatus, doc.ReconciliationStatus, len(result.entries))
	return nil
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

// GetValidation loads validation results and computes field statuses for a document.
func (e *Engine) GetValidation(ctx context.Context, tenantID, docID uuid.UUID) (*ValidationResponse, error) {
	doc, err := e.docRepo.GetByID(ctx, tenantID, docID)
	if err != nil {
		return nil, err
	}

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

	confidenceMap := flattenConfidenceScores(doc.ConfidenceScores)
	fieldStatuses := ComputeFieldStatuses(results, rulesMap, confidenceMap)
	valSummary, reconSummary := buildSummaries(results, rulesMap)
	resultItems := buildResultItems(results, rulesMap)

	return &ValidationResponse{
		DocumentID:            docID,
		ValidationStatus:      doc.ValidationStatus,
		Summary:               valSummary,
		ReconciliationStatus:  doc.ReconciliationStatus,
		ReconciliationSummary: reconSummary,
		Results:               resultItems,
		FieldStatuses:         fieldStatuses,
	}, nil
}

// ValidationResponse is the API response for GET /documents/:id/validation.
type ValidationResponse struct {
	DocumentID            uuid.UUID                   `json:"document_id"`
	ValidationStatus      domain.ValidationStatus     `json:"validation_status"`
	Summary               ValidationSummary           `json:"summary"`
	ReconciliationStatus  domain.ReconciliationStatus `json:"reconciliation_status"`
	ReconciliationSummary ReconciliationSummary       `json:"reconciliation_summary"`
	Results               []ValidationResultItem      `json:"results"`
	FieldStatuses         map[string]*FieldStatus     `json:"field_statuses"`
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
