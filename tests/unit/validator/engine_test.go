package validator_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/validator"
	"satvos/internal/validator/invoice"
	"satvos/mocks"
)

func setupEngine() (*validator.Engine, *mocks.MockDocumentRepo, *mocks.MockDocumentValidationRuleRepo) {
	docRepo := new(mocks.MockDocumentRepo)
	ruleRepo := new(mocks.MockDocumentValidationRuleRepo)
	registry := validator.NewRegistry()
	for _, v := range invoice.AllBuiltinValidators() {
		registry.Register(v)
	}
	engine := validator.NewEngine(registry, ruleRepo, docRepo)
	return engine, docRepo, ruleRepo
}

func validInvoiceJSON() json.RawMessage {
	inv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{
			InvoiceNumber: "INV-001",
			InvoiceDate:   "15/01/2025",
			DueDate:       "15/02/2025",
			Currency:      "INR",
			PlaceOfSupply: "Karnataka",
		},
		Seller: invoice.Party{
			Name:      "Seller Corp",
			GSTIN:     "29ABCDE1234F1Z5",
			PAN:       "ABCDE1234F",
			State:     "Karnataka",
			StateCode: "29",
		},
		Buyer: invoice.Party{
			Name:      "Buyer Corp",
			GSTIN:     "29FGHIJ5678K1Z2",
			PAN:       "FGHIJ5678K",
			State:     "Karnataka",
			StateCode: "29",
		},
		LineItems: []invoice.LineItem{
			{
				Description:   "Widget",
				HSNSACCode:    "8471",
				Quantity:      10,
				UnitPrice:     100,
				TaxableAmount: 1000,
				CGSTRate:      9,
				CGSTAmount:    90,
				SGSTRate:      9,
				SGSTAmount:    90,
				Total:         1180,
			},
		},
		Totals: invoice.Totals{
			Subtotal:      1000,
			TaxableAmount: 1000,
			CGST:          90,
			SGST:          90,
			Total:         1180,
		},
	}
	data, _ := json.Marshal(inv)
	return data
}

func makeRule(id uuid.UUID, key string, severity domain.ValidationSeverity) domain.DocumentValidationRule {
	return domain.DocumentValidationRule{
		ID:             id,
		TenantID:       uuid.New(),
		DocumentType:   "invoice",
		RuleName:       "Test: " + key,
		RuleType:       domain.ValidationRuleRequired,
		RuleConfig:     json.RawMessage("{}"),
		Severity:       severity,
		IsActive:       true,
		IsBuiltin:      true,
		BuiltinRuleKey: &key,
	}
}

// --- ValidateDocument ---

func allBuiltinKeys() []string {
	validators := invoice.AllBuiltinValidators()
	keys := make([]string, 0, len(validators))
	for _, v := range validators {
		keys = append(keys, v.RuleKey())
	}
	return keys
}

func TestEngine_ValidateDocument_Success(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		StructuredData:    validInvoiceJSON(),
		ConfidenceScores:  json.RawMessage("{}"),
		ValidationResults: json.RawMessage("[]"),
		CreatedBy:         uuid.New(),
	}

	ruleKey := "req.invoice.number"
	ruleID := uuid.New()
	rules := []domain.DocumentValidationRule{makeRule(ruleID, ruleKey, domain.ValidationSeverityError)}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return(allBuiltinKeys(), nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return(rules, nil)
	docRepo.On("UpdateValidationResults", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)

	err := engine.ValidateDocument(ctx, tenantID, docID)

	assert.NoError(t, err)
	docRepo.AssertCalled(t, "UpdateValidationResults", mock.Anything, mock.AnythingOfType("*domain.Document"))
}

func TestEngine_ValidateDocument_NoRules(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		StructuredData:    validInvoiceJSON(),
		ConfidenceScores:  json.RawMessage("{}"),
		ValidationResults: json.RawMessage("[]"),
		CreatedBy:         uuid.New(),
	}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return(allBuiltinKeys(), nil)
	// No active rules
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return([]domain.DocumentValidationRule{}, nil)
	docRepo.On("UpdateValidationResults", mock.Anything, mock.AnythingOfType("*domain.Document")).
		Run(func(args mock.Arguments) {
			d := args.Get(1).(*domain.Document)
			assert.Equal(t, domain.ValidationStatusValid, d.ValidationStatus)
			// Results should be empty array
			var results []validator.ValidationResultEntry
			_ = json.Unmarshal(d.ValidationResults, &results)
			assert.Empty(t, results)
		}).Return(nil)

	err := engine.ValidateDocument(ctx, tenantID, docID)

	assert.NoError(t, err)
}

func TestEngine_ValidateDocument_MixedResults(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	// Invoice missing seller GSTIN → error rule fails
	inv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{
			InvoiceNumber: "INV-001",
			InvoiceDate:   "15/01/2025",
			Currency:      "INR",
			PlaceOfSupply: "Karnataka",
		},
		Seller: invoice.Party{
			Name:      "Seller Corp",
			GSTIN:     "", // missing → error
			StateCode: "29",
		},
		Buyer: invoice.Party{
			Name:      "Buyer Corp",
			GSTIN:     "29FGHIJ5678K1Z2",
			StateCode: "29",
		},
		LineItems: []invoice.LineItem{
			{Description: "Widget", HSNSACCode: "8471", Quantity: 1, UnitPrice: 100, TaxableAmount: 100, Total: 100},
		},
		Totals: invoice.Totals{Subtotal: 100, TaxableAmount: 100, Total: 100},
	}
	data, _ := json.Marshal(inv)

	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		StructuredData:    data,
		ConfidenceScores:  json.RawMessage("{}"),
		ValidationResults: json.RawMessage("[]"),
		CreatedBy:         uuid.New(),
	}

	reqGSTINKey := "req.seller.gstin"
	reqNameKey := "req.seller.name"
	rules := []domain.DocumentValidationRule{
		makeRule(uuid.New(), reqGSTINKey, domain.ValidationSeverityError),
		makeRule(uuid.New(), reqNameKey, domain.ValidationSeverityError),
	}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return(allBuiltinKeys(), nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return(rules, nil)
	docRepo.On("UpdateValidationResults", mock.Anything, mock.AnythingOfType("*domain.Document")).
		Run(func(args mock.Arguments) {
			d := args.Get(1).(*domain.Document)
			// Should be invalid because seller GSTIN is missing
			assert.Equal(t, domain.ValidationStatusInvalid, d.ValidationStatus)
		}).Return(nil)

	err := engine.ValidateDocument(ctx, tenantID, docID)

	assert.NoError(t, err)
}

func TestEngine_ValidateDocument_WarningOnly(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	// Invoice missing currency → warning rule fails
	inv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{
			InvoiceNumber: "INV-001",
			InvoiceDate:   "15/01/2025",
			Currency:      "", // missing → warning
			PlaceOfSupply: "Karnataka",
		},
		Seller: invoice.Party{Name: "S", GSTIN: "29ABCDE1234F1Z5", StateCode: "29"},
		Buyer:  invoice.Party{Name: "B", GSTIN: "29FGHIJ5678K1Z2", StateCode: "29"},
	}
	data, _ := json.Marshal(inv)

	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		StructuredData:    data,
		ConfidenceScores:  json.RawMessage("{}"),
		ValidationResults: json.RawMessage("[]"),
		CreatedBy:         uuid.New(),
	}

	currencyKey := "req.invoice.currency"
	rules := []domain.DocumentValidationRule{
		makeRule(uuid.New(), currencyKey, domain.ValidationSeverityWarning),
	}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return(allBuiltinKeys(), nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return(rules, nil)
	docRepo.On("UpdateValidationResults", mock.Anything, mock.AnythingOfType("*domain.Document")).
		Run(func(args mock.Arguments) {
			d := args.Get(1).(*domain.Document)
			assert.Equal(t, domain.ValidationStatusWarning, d.ValidationStatus)
		}).Return(nil)

	err := engine.ValidateDocument(ctx, tenantID, docID)

	assert.NoError(t, err)
}

func TestEngine_ValidateDocument_ErrorOverridesWarning(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	// Invoice missing currency (warning) AND seller GSTIN (error)
	inv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{
			InvoiceNumber: "INV-001",
			InvoiceDate:   "15/01/2025",
			Currency:      "", // warning
			PlaceOfSupply: "Karnataka",
		},
		Seller: invoice.Party{Name: "S", GSTIN: "", StateCode: "29"}, // error
		Buyer:  invoice.Party{Name: "B", GSTIN: "29FGHIJ5678K1Z2", StateCode: "29"},
	}
	data, _ := json.Marshal(inv)

	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		StructuredData:    data,
		ConfidenceScores:  json.RawMessage("{}"),
		ValidationResults: json.RawMessage("[]"),
		CreatedBy:         uuid.New(),
	}

	rules := []domain.DocumentValidationRule{
		makeRule(uuid.New(), "req.invoice.currency", domain.ValidationSeverityWarning),
		makeRule(uuid.New(), "req.seller.gstin", domain.ValidationSeverityError),
	}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return(allBuiltinKeys(), nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return(rules, nil)
	docRepo.On("UpdateValidationResults", mock.Anything, mock.AnythingOfType("*domain.Document")).
		Run(func(args mock.Arguments) {
			d := args.Get(1).(*domain.Document)
			assert.Equal(t, domain.ValidationStatusInvalid, d.ValidationStatus)
		}).Return(nil)

	err := engine.ValidateDocument(ctx, tenantID, docID)

	assert.NoError(t, err)
}

func TestEngine_ValidateDocument_DocNotFound(t *testing.T) {
	engine, docRepo, _ := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	docRepo.On("GetByID", ctx, tenantID, docID).Return(nil, domain.ErrDocumentNotFound)

	err := engine.ValidateDocument(ctx, tenantID, docID)

	assert.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrDocumentNotFound)
}

func TestEngine_ValidateDocument_RuleLoadError(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		StructuredData:    validInvoiceJSON(),
		ConfidenceScores:  json.RawMessage("{}"),
		ValidationResults: json.RawMessage("[]"),
		CreatedBy:         uuid.New(),
	}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return(allBuiltinKeys(), nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).
		Return(nil, errors.New("db error"))

	err := engine.ValidateDocument(ctx, tenantID, docID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "loading rules")
}

// --- EnsureBuiltinRules ---

func TestEngine_EnsureBuiltinRules_SeedsNew(t *testing.T) {
	engine, _, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	createdBy := uuid.New()

	// No existing keys → all should be seeded
	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return([]string{}, nil)
	ruleRepo.On("Create", ctx, mock.AnythingOfType("*domain.DocumentValidationRule")).Return(nil)

	err := engine.EnsureBuiltinRules(ctx, tenantID, "invoice", createdBy)

	assert.NoError(t, err)
	// Should have been called for each builtin validator
	numValidators := len(invoice.AllBuiltinValidators())
	assert.Equal(t, numValidators, len(ruleRepo.Calls)-1) // -1 for ListBuiltinKeys
}

func TestEngine_EnsureBuiltinRules_SkipsExisting(t *testing.T) {
	engine, _, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	createdBy := uuid.New()

	// All keys already exist
	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return(allBuiltinKeys(), nil)

	err := engine.EnsureBuiltinRules(ctx, tenantID, "invoice", createdBy)

	assert.NoError(t, err)
	// Create should never be called
	ruleRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// --- GetValidation ---

func TestEngine_GetValidation_Success(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()
	ruleID := uuid.New()

	ruleKey := "req.seller.gstin"
	results := []validator.ValidationResultEntry{
		{RuleID: ruleID, Passed: false, FieldPath: "seller.gstin", ExpectedValue: "non-empty value", ActualValue: "", Message: "GSTIN is missing"},
	}
	resultsJSON, _ := json.Marshal(results)

	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		ValidationStatus:  domain.ValidationStatusInvalid,
		ValidationResults: resultsJSON,
		StructuredData:    json.RawMessage("{}"),
		ConfidenceScores:  json.RawMessage("{}"),
	}

	rules := []domain.DocumentValidationRule{
		makeRule(ruleID, ruleKey, domain.ValidationSeverityError),
	}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return(rules, nil)

	resp, err := engine.GetValidation(ctx, tenantID, docID)

	assert.NoError(t, err)
	assert.Equal(t, docID, resp.DocumentID)
	assert.Equal(t, domain.ValidationStatusInvalid, resp.ValidationStatus)
	assert.Equal(t, 1, resp.Summary.Total)
	assert.Equal(t, 0, resp.Summary.Passed)
	assert.Equal(t, 1, resp.Summary.Errors)
	assert.Equal(t, 0, resp.Summary.Warnings)
	assert.Len(t, resp.Results, 1)
	assert.Equal(t, "seller.gstin", resp.Results[0].FieldPath)
	assert.NotNil(t, resp.FieldStatuses["seller.gstin"])
	assert.Equal(t, domain.FieldStatusInvalid, resp.FieldStatuses["seller.gstin"].Status)
}

func TestEngine_GetValidation_EmptyResults(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		ValidationStatus:  domain.ValidationStatusValid,
		ValidationResults: json.RawMessage("[]"),
		StructuredData:    json.RawMessage("{}"),
		ConfidenceScores:  json.RawMessage("{}"),
	}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return([]domain.DocumentValidationRule{}, nil)

	resp, err := engine.GetValidation(ctx, tenantID, docID)

	assert.NoError(t, err)
	assert.Equal(t, 0, resp.Summary.Total)
	assert.Empty(t, resp.Results)
}

// --- Reconciliation Tiering ---

func TestEngine_ValidateDocument_ReconciliationCriticalError(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	// Invoice missing seller GSTIN → reconciliation-critical error
	inv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{
			InvoiceNumber: "INV-001",
			InvoiceDate:   "15/01/2025",
			PlaceOfSupply: "Karnataka",
		},
		Seller: invoice.Party{
			Name:      "Seller Corp",
			GSTIN:     "", // missing → reconciliation-critical error
			StateCode: "29",
		},
		Buyer: invoice.Party{
			Name:      "Buyer Corp",
			GSTIN:     "29FGHIJ5678K1Z2",
			StateCode: "29",
		},
		LineItems: []invoice.LineItem{
			{Description: "Widget", Quantity: 1, UnitPrice: 100, TaxableAmount: 100, Total: 100},
		},
		Totals: invoice.Totals{Subtotal: 100, TaxableAmount: 100, Total: 100},
	}
	data, _ := json.Marshal(inv)

	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		StructuredData:    data,
		ConfidenceScores:  json.RawMessage("{}"),
		ValidationResults: json.RawMessage("[]"),
		CreatedBy:         uuid.New(),
	}

	reqGSTINKey := "req.seller.gstin"
	rule := makeRule(uuid.New(), reqGSTINKey, domain.ValidationSeverityError)
	rule.ReconciliationCritical = true
	rules := []domain.DocumentValidationRule{rule}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return(allBuiltinKeys(), nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return(rules, nil)
	docRepo.On("UpdateValidationResults", mock.Anything, mock.AnythingOfType("*domain.Document")).
		Run(func(args mock.Arguments) {
			d := args.Get(1).(*domain.Document)
			assert.Equal(t, domain.ValidationStatusInvalid, d.ValidationStatus)
			assert.Equal(t, domain.ReconciliationStatusInvalid, d.ReconciliationStatus)
		}).Return(nil)

	err := engine.ValidateDocument(ctx, tenantID, docID)
	assert.NoError(t, err)
}

func TestEngine_ValidateDocument_NonReconCriticalError_ReconValid(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	// Invoice missing due date → non-reconciliation-critical warning
	inv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{
			InvoiceNumber: "INV-001",
			InvoiceDate:   "15/01/2025",
			Currency:      "", // missing → warning, NOT recon-critical
			PlaceOfSupply: "Karnataka",
		},
		Seller: invoice.Party{Name: "S", GSTIN: "29ABCDE1234F1Z5", StateCode: "29"},
		Buyer:  invoice.Party{Name: "B", GSTIN: "29FGHIJ5678K1Z2", StateCode: "29"},
	}
	data, _ := json.Marshal(inv)

	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		StructuredData:    data,
		ConfidenceScores:  json.RawMessage("{}"),
		ValidationResults: json.RawMessage("[]"),
		CreatedBy:         uuid.New(),
	}

	currencyKey := "req.invoice.currency"
	rule := makeRule(uuid.New(), currencyKey, domain.ValidationSeverityWarning)
	rule.ReconciliationCritical = false
	rules := []domain.DocumentValidationRule{rule}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return(allBuiltinKeys(), nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return(rules, nil)
	docRepo.On("UpdateValidationResults", mock.Anything, mock.AnythingOfType("*domain.Document")).
		Run(func(args mock.Arguments) {
			d := args.Get(1).(*domain.Document)
			assert.Equal(t, domain.ValidationStatusWarning, d.ValidationStatus)
			// Reconciliation should be valid since the failing rule is not recon-critical
			assert.Equal(t, domain.ReconciliationStatusValid, d.ReconciliationStatus)
		}).Return(nil)

	err := engine.ValidateDocument(ctx, tenantID, docID)
	assert.NoError(t, err)
}

func TestEngine_ValidateDocument_MixedReconAndNonRecon(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	// Invoice missing seller GSTIN (recon-critical error) AND currency (non-recon warning)
	inv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{
			InvoiceNumber: "INV-001",
			InvoiceDate:   "15/01/2025",
			Currency:      "", // warning, not recon-critical
			PlaceOfSupply: "Karnataka",
		},
		Seller: invoice.Party{Name: "S", GSTIN: "", StateCode: "29"}, // error, recon-critical
		Buyer:  invoice.Party{Name: "B", GSTIN: "29FGHIJ5678K1Z2", StateCode: "29"},
	}
	data, _ := json.Marshal(inv)

	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		StructuredData:    data,
		ConfidenceScores:  json.RawMessage("{}"),
		ValidationResults: json.RawMessage("[]"),
		CreatedBy:         uuid.New(),
	}

	reconRule := makeRule(uuid.New(), "req.seller.gstin", domain.ValidationSeverityError)
	reconRule.ReconciliationCritical = true

	nonReconRule := makeRule(uuid.New(), "req.invoice.currency", domain.ValidationSeverityWarning)
	nonReconRule.ReconciliationCritical = false

	rules := []domain.DocumentValidationRule{reconRule, nonReconRule}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return(allBuiltinKeys(), nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return(rules, nil)
	docRepo.On("UpdateValidationResults", mock.Anything, mock.AnythingOfType("*domain.Document")).
		Run(func(args mock.Arguments) {
			d := args.Get(1).(*domain.Document)
			assert.Equal(t, domain.ValidationStatusInvalid, d.ValidationStatus)
			assert.Equal(t, domain.ReconciliationStatusInvalid, d.ReconciliationStatus)
		}).Return(nil)

	err := engine.ValidateDocument(ctx, tenantID, docID)
	assert.NoError(t, err)
}

func TestEngine_ValidateDocument_AllPass_ReconValid(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		StructuredData:    validInvoiceJSON(),
		ConfidenceScores:  json.RawMessage("{}"),
		ValidationResults: json.RawMessage("[]"),
		CreatedBy:         uuid.New(),
	}

	ruleKey := "req.invoice.number"
	rule := makeRule(uuid.New(), ruleKey, domain.ValidationSeverityError)
	rule.ReconciliationCritical = true
	rules := []domain.DocumentValidationRule{rule}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return(allBuiltinKeys(), nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return(rules, nil)
	docRepo.On("UpdateValidationResults", mock.Anything, mock.AnythingOfType("*domain.Document")).
		Run(func(args mock.Arguments) {
			d := args.Get(1).(*domain.Document)
			assert.Equal(t, domain.ValidationStatusValid, d.ValidationStatus)
			assert.Equal(t, domain.ReconciliationStatusValid, d.ReconciliationStatus)
		}).Return(nil)

	err := engine.ValidateDocument(ctx, tenantID, docID)
	assert.NoError(t, err)
}

func TestEngine_EnsureBuiltinRules_SetsReconciliationCritical(t *testing.T) {
	engine, _, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	createdBy := uuid.New()

	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return([]string{}, nil)

	var reconCriticalKeys []string
	ruleRepo.On("Create", ctx, mock.AnythingOfType("*domain.DocumentValidationRule")).
		Run(func(args mock.Arguments) {
			rule := args.Get(1).(*domain.DocumentValidationRule)
			if rule.ReconciliationCritical && rule.BuiltinRuleKey != nil {
				reconCriticalKeys = append(reconCriticalKeys, *rule.BuiltinRuleKey)
			}
		}).Return(nil)

	err := engine.EnsureBuiltinRules(ctx, tenantID, "invoice", createdBy)

	assert.NoError(t, err)

	// Verify the expected 21 reconciliation-critical keys
	expectedReconKeys := map[string]bool{
		"req.invoice.number":          true,
		"req.invoice.date":            true,
		"req.invoice.place_of_supply": true,
		"req.seller.name":             true,
		"req.seller.gstin":            true,
		"req.buyer.gstin":             true,
		"fmt.seller.gstin":            true,
		"fmt.buyer.gstin":             true,
		"fmt.seller.state_code":       true,
		"fmt.buyer.state_code":        true,
		"math.totals.taxable_amount":  true,
		"math.totals.cgst":            true,
		"math.totals.sgst":            true,
		"math.totals.igst":            true,
		"math.totals.grand_total":     true,
		"xf.seller.gstin_state":       true,
		"xf.buyer.gstin_state":        true,
		"xf.tax_type.intrastate":      true,
		"xf.tax_type.interstate":      true,
		"logic.line_items.at_least_one":  true,
		"logic.line_item.exclusive_tax": true,
		"xf.invoice.irn_hash":           true,
	}

	assert.Equal(t, len(expectedReconKeys), len(reconCriticalKeys), "expected 22 reconciliation-critical rules")
	for _, key := range reconCriticalKeys {
		assert.True(t, expectedReconKeys[key], "unexpected reconciliation-critical key: %s", key)
	}
}

func TestEngine_GetValidation_ReconciliationSummary(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()
	reconRuleID := uuid.New()
	nonReconRuleID := uuid.New()

	reconKey := "req.seller.gstin"
	nonReconKey := "req.invoice.currency"

	results := []validator.ValidationResultEntry{
		{RuleID: reconRuleID, Passed: false, FieldPath: "seller.gstin", Message: "missing GSTIN", ReconciliationCritical: true},
		{RuleID: nonReconRuleID, Passed: false, FieldPath: "invoice.currency", Message: "missing currency", ReconciliationCritical: false},
	}
	resultsJSON, _ := json.Marshal(results)

	doc := &domain.Document{
		ID:                   docID,
		TenantID:             tenantID,
		DocumentType:         "invoice",
		ValidationStatus:     domain.ValidationStatusInvalid,
		ReconciliationStatus: domain.ReconciliationStatusInvalid,
		ValidationResults:    resultsJSON,
		StructuredData:       json.RawMessage("{}"),
		ConfidenceScores:     json.RawMessage("{}"),
	}

	reconRule := makeRule(reconRuleID, reconKey, domain.ValidationSeverityError)
	reconRule.ReconciliationCritical = true
	nonReconRule := makeRule(nonReconRuleID, nonReconKey, domain.ValidationSeverityWarning)
	nonReconRule.ReconciliationCritical = false

	rules := []domain.DocumentValidationRule{reconRule, nonReconRule}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return(rules, nil)

	resp, err := engine.GetValidation(ctx, tenantID, docID)

	assert.NoError(t, err)
	assert.Equal(t, domain.ReconciliationStatusInvalid, resp.ReconciliationStatus)

	// Reconciliation summary should only count the recon-critical rule
	assert.Equal(t, 1, resp.ReconciliationSummary.Total)
	assert.Equal(t, 0, resp.ReconciliationSummary.Passed)
	assert.Equal(t, 1, resp.ReconciliationSummary.Errors)
	assert.Equal(t, 0, resp.ReconciliationSummary.Warnings)

	// Overall summary should count both
	assert.Equal(t, 2, resp.Summary.Total)
	assert.Equal(t, 0, resp.Summary.Passed)
	assert.Equal(t, 1, resp.Summary.Errors)
	assert.Equal(t, 1, resp.Summary.Warnings)

	// Verify reconciliation_critical flag in results
	assert.True(t, resp.Results[0].ReconciliationCritical)
	assert.False(t, resp.Results[1].ReconciliationCritical)
}

// --- ReconciliationCritical on validators ---

func TestBuiltinValidators_ReconciliationCritical(t *testing.T) {
	validators := invoice.AllBuiltinValidators()

	reconCriticalKeys := map[string]bool{
		"req.invoice.number": true, "req.invoice.date": true,
		"req.invoice.place_of_supply": true, "req.seller.name": true,
		"req.seller.gstin": true, "req.buyer.gstin": true,
		"fmt.seller.gstin": true, "fmt.buyer.gstin": true,
		"fmt.seller.state_code": true, "fmt.buyer.state_code": true,
		"math.totals.taxable_amount": true, "math.totals.cgst": true,
		"math.totals.sgst": true, "math.totals.igst": true,
		"math.totals.grand_total": true,
		"xf.seller.gstin_state": true, "xf.buyer.gstin_state": true,
		"xf.tax_type.intrastate": true, "xf.tax_type.interstate": true,
		"logic.line_items.at_least_one": true, "logic.line_item.exclusive_tax": true,
		"xf.invoice.irn_hash": true,
	}

	for _, v := range validators {
		expected := reconCriticalKeys[v.RuleKey()]
		assert.Equal(t, expected, v.ReconciliationCritical(),
			"rule %s: expected ReconciliationCritical=%v, got %v", v.RuleKey(), expected, v.ReconciliationCritical())
	}
}

func TestEngine_GetValidation_WithConfidenceScores(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	confidenceScores := invoice.ConfidenceScores{
		Seller: invoice.PartyConfidence{
			GSTIN: 0.3, // low confidence → unsure
			Name:  0.9, // high confidence → valid
		},
	}
	confJSON, _ := json.Marshal(confidenceScores)

	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		ValidationStatus:  domain.ValidationStatusValid,
		ValidationResults: json.RawMessage("[]"),
		StructuredData:    json.RawMessage("{}"),
		ConfidenceScores:  confJSON,
	}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return([]domain.DocumentValidationRule{}, nil)

	resp, err := engine.GetValidation(ctx, tenantID, docID)

	assert.NoError(t, err)
	assert.Equal(t, domain.FieldStatusUnsure, resp.FieldStatuses["seller.gstin"].Status)
	assert.Equal(t, domain.FieldStatusValid, resp.FieldStatuses["seller.name"].Status)
}

// --- Edge cases: empty/malformed structured data ---

func TestEngine_ValidateDocument_EmptyStructuredData(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	// Empty JSON object — all fields zero-value, required validators will fail
	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		StructuredData:    json.RawMessage("{}"),
		ConfidenceScores:  json.RawMessage("{}"),
		ValidationResults: json.RawMessage("[]"),
		CreatedBy:         uuid.New(),
	}

	ruleKey := "req.invoice.number"
	ruleID := uuid.New()
	rules := []domain.DocumentValidationRule{makeRule(ruleID, ruleKey, domain.ValidationSeverityError)}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return(allBuiltinKeys(), nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return(rules, nil)
	docRepo.On("UpdateValidationResults", mock.Anything, mock.AnythingOfType("*domain.Document")).
		Run(func(args mock.Arguments) {
			d := args.Get(1).(*domain.Document)
			assert.Equal(t, domain.ValidationStatusInvalid, d.ValidationStatus)
			var results []validator.ValidationResultEntry
			_ = json.Unmarshal(d.ValidationResults, &results)
			assert.NotEmpty(t, results)
		}).Return(nil)

	err := engine.ValidateDocument(ctx, tenantID, docID)

	assert.NoError(t, err)
	docRepo.AssertCalled(t, "UpdateValidationResults", mock.Anything, mock.AnythingOfType("*domain.Document"))
}

func TestEngine_ValidateDocument_NilStructuredData(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		StructuredData:    nil,
		ConfidenceScores:  json.RawMessage("{}"),
		ValidationResults: json.RawMessage("[]"),
		CreatedBy:         uuid.New(),
	}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return(allBuiltinKeys(), nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return([]domain.DocumentValidationRule{}, nil)

	err := engine.ValidateDocument(ctx, tenantID, docID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshaling structured_data")
}

func TestEngine_ValidateDocument_ValidatorPanicRecovery(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	ruleRepo := new(mocks.MockDocumentValidationRuleRepo)
	registry := validator.NewRegistry()
	// Register a panicking validator
	registry.Register(panickingValidator{})
	engine := validator.NewEngine(registry, ruleRepo, docRepo)

	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		StructuredData:    json.RawMessage("{}"),
		ConfidenceScores:  json.RawMessage("{}"),
		ValidationResults: json.RawMessage("[]"),
		CreatedBy:         uuid.New(),
	}

	ruleKey := "test.panicking"
	ruleID := uuid.New()
	rules := []domain.DocumentValidationRule{{
		ID:              ruleID,
		TenantID:        tenantID,
		DocumentType:    "invoice",
		RuleName:        "Panicking Validator",
		RuleType:        domain.ValidationRuleRequired,
		Severity:        domain.ValidationSeverityError,
		IsActive:        true,
		IsBuiltin:       true,
		BuiltinRuleKey:  &ruleKey,
	}}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return([]string{ruleKey}, nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return(rules, nil)
	docRepo.On("UpdateValidationResults", mock.Anything, mock.AnythingOfType("*domain.Document")).
		Run(func(args mock.Arguments) {
			d := args.Get(1).(*domain.Document)
			// Panic should result in invalid status
			assert.Equal(t, domain.ValidationStatusInvalid, d.ValidationStatus)
			// Should have a result entry for the panicked validator
			var results []validator.ValidationResultEntry
			_ = json.Unmarshal(d.ValidationResults, &results)
			assert.Len(t, results, 1)
			assert.False(t, results[0].Passed)
			assert.Contains(t, results[0].Message, "validator panicked")
		}).Return(nil)

	// Should not panic — engine should recover gracefully
	err := engine.ValidateDocument(ctx, tenantID, docID)

	assert.NoError(t, err)
	docRepo.AssertCalled(t, "UpdateValidationResults", mock.Anything, mock.AnythingOfType("*domain.Document"))
}

func TestEngine_ValidateDocument_MalformedJSON(t *testing.T) {
	engine, docRepo, ruleRepo := setupEngine()
	ctx := context.Background()
	tenantID := uuid.New()
	docID := uuid.New()

	doc := &domain.Document{
		ID:                docID,
		TenantID:          tenantID,
		DocumentType:      "invoice",
		StructuredData:    json.RawMessage("{invalid"),
		ConfidenceScores:  json.RawMessage("{}"),
		ValidationResults: json.RawMessage("[]"),
		CreatedBy:         uuid.New(),
	}

	docRepo.On("GetByID", ctx, tenantID, docID).Return(doc, nil)
	ruleRepo.On("ListBuiltinKeys", ctx, tenantID, "invoice").Return(allBuiltinKeys(), nil)
	ruleRepo.On("ListByDocumentType", ctx, tenantID, "invoice", (*uuid.UUID)(nil)).Return([]domain.DocumentValidationRule{}, nil)

	err := engine.ValidateDocument(ctx, tenantID, docID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshaling structured_data")
}

// panickingValidator is a test validator that always panics.
type panickingValidator struct{}

func (panickingValidator) Validate(_ context.Context, _ *invoice.GSTInvoice) []invoice.ValidationResult {
	panic("intentional test panic")
}
func (panickingValidator) RuleKey() string                     { return "test.panicking" }
func (panickingValidator) RuleName() string                    { return "Panicking Validator" }
func (panickingValidator) RuleType() domain.ValidationRuleType { return domain.ValidationRuleRequired }
func (panickingValidator) Severity() domain.ValidationSeverity { return domain.ValidationSeverityError }
func (panickingValidator) ReconciliationCritical() bool        { return false }
