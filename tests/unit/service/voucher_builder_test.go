package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"satvos/internal/domain"
	"satvos/internal/service"
	"satvos/internal/validator/invoice"
	"satvos/mocks"
)

// buildTestDocument creates a domain.Document with structured_data from a GSTInvoice.
func buildTestDocument(t *testing.T, inv *invoice.GSTInvoice) *domain.Document {
	t.Helper()
	data, err := json.Marshal(inv)
	require.NoError(t, err)
	return &domain.Document{
		ID:             uuid.New(),
		TenantID:       uuid.New(),
		CollectionID:   uuid.New(),
		StructuredData: data,
	}
}

// sampleInvoice returns a fully populated GSTInvoice for testing.
func sampleInvoice() *invoice.GSTInvoice {
	return &invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{
			InvoiceNumber: "INV-2026-001",
			InvoiceDate:   "15/01/2026",
			InvoiceType:   "Tax Invoice",
			Currency:      "INR",
			PlaceOfSupply: "Karnataka",
		},
		Seller: invoice.Party{
			Name:      "Acme Pvt. Ltd.",
			GSTIN:     "29AABCU9603R1ZM",
			State:     "Karnataka",
			StateCode: "29",
		},
		Buyer: invoice.Party{
			Name:      "Beta Corp",
			GSTIN:     "27AABCB1234R1Z5",
			State:     "Maharashtra",
			StateCode: "27",
		},
		LineItems: []invoice.LineItem{
			{
				Description:   "Widget A",
				HSNSACCode:    "84713010",
				Quantity:      10,
				Unit:          "Nos",
				UnitPrice:     100,
				TaxableAmount: 1000,
				CGSTRate:      9,
				CGSTAmount:    90,
				SGSTRate:      9,
				SGSTAmount:    90,
				Total:         1180,
			},
			{
				Description:   "Widget B",
				HSNSACCode:    "84714100",
				Quantity:      5,
				Unit:          "Pcs",
				UnitPrice:     200,
				TaxableAmount: 1000,
				CGSTRate:      9,
				CGSTAmount:    90,
				SGSTRate:      9,
				SGSTAmount:    90,
				Total:         1180,
			},
		},
		Totals: invoice.Totals{
			Subtotal:      2000,
			TaxableAmount: 2000,
			CGST:          180,
			SGST:          180,
			Total:         2360,
		},
	}
}

func TestVoucherBuilder_FullMatch(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	builder := service.NewVoucherBuilderService(masterRepo)

	inv := sampleInvoice()
	doc := buildTestDocument(t, inv)
	tenantID := doc.TenantID

	// Party match by GSTIN
	masterRepo.On("FindLedgerByGSTIN", mock.Anything, tenantID, "29AABCU9603R1ZM").
		Return(&domain.TallyLedger{Name: "Acme Pvt Ltd"}, nil)

	// Purchase ledger found
	masterRepo.On("FindPurchaseLedger", mock.Anything, tenantID).
		Return(&domain.TallyLedger{Name: "Purchase - Trading"}, nil)

	// Tax ledgers found (CGST rate = 180/2000*100 = 9%, SGST rate = 9%)
	masterRepo.On("FindTaxLedger", mock.Anything, tenantID, "CGST", mock.AnythingOfType("float64")).
		Return(&domain.TallyLedger{Name: "Input CGST @9%"}, nil)
	masterRepo.On("FindTaxLedger", mock.Anything, tenantID, "SGST", mock.AnythingOfType("float64")).
		Return(&domain.TallyLedger{Name: "Input SGST @9%"}, nil)

	// Stock items found by HSN
	masterRepo.On("FindStockItemByHSN", mock.Anything, tenantID, "84713010").
		Return(&domain.TallyStockItem{Name: "Laptop", HSNCode: "84713010", DefaultUOM: "Nos"}, nil)
	masterRepo.On("FindStockItemByHSN", mock.Anything, tenantID, "84714100").
		Return(&domain.TallyStockItem{Name: "Desktop", HSNCode: "84714100", DefaultUOM: "Pcs"}, nil)

	// UOM found
	masterRepo.On("FindUnitBySymbol", mock.Anything, tenantID, "Nos").
		Return(&domain.TallyUnit{Symbol: "Nos", FormalName: "Numbers"}, nil)
	masterRepo.On("FindUnitBySymbol", mock.Anything, tenantID, "Pcs").
		Return(&domain.TallyUnit{Symbol: "Pcs", FormalName: "Pieces"}, nil)

	// Default godown
	masterRepo.On("GetDefaultGodown", mock.Anything, tenantID).
		Return(&domain.TallyGodown{Name: "Main Godown"}, nil)

	vDef, err := builder.Build(context.Background(), tenantID, doc)
	require.NoError(t, err)
	require.NotNil(t, vDef)

	assert.Equal(t, doc.ID, vDef.DocumentID)
	assert.Equal(t, "Purchase", vDef.VoucherType)
	assert.Equal(t, "2026-01-15", vDef.VoucherDate)
	assert.Equal(t, "Acme Pvt Ltd", vDef.PartyLedger)
	assert.Equal(t, "Purchase - Trading", vDef.PurchaseLedger)
	assert.Equal(t, 2360.0, vDef.TotalAmount)

	// Tax entries
	require.Len(t, vDef.TaxEntries, 2)
	assert.Equal(t, "Input CGST @9%", vDef.TaxEntries[0].LedgerName)
	assert.Equal(t, 180.0, vDef.TaxEntries[0].Amount)
	assert.Equal(t, "Input SGST @9%", vDef.TaxEntries[1].LedgerName)
	assert.Equal(t, 180.0, vDef.TaxEntries[1].Amount)

	// Inventory items
	require.Len(t, vDef.InventoryItems, 2)
	assert.Equal(t, "Laptop", vDef.InventoryItems[0].StockItem)
	assert.Equal(t, "84713010", vDef.InventoryItems[0].HSNCode)
	assert.Equal(t, 10.0, vDef.InventoryItems[0].Quantity)
	assert.Equal(t, 100.0, vDef.InventoryItems[0].Rate)
	assert.Equal(t, 1000.0, vDef.InventoryItems[0].Amount)
	assert.Equal(t, "Nos", vDef.InventoryItems[0].UOM)
	assert.Equal(t, "Main Godown", vDef.InventoryItems[0].Godown)

	assert.Equal(t, "Desktop", vDef.InventoryItems[1].StockItem)
	assert.Equal(t, "84714100", vDef.InventoryItems[1].HSNCode)
	assert.Equal(t, 5.0, vDef.InventoryItems[1].Quantity)
	assert.Equal(t, "Pcs", vDef.InventoryItems[1].UOM)

	// Narration
	assert.Equal(t, "Acme Pvt. Ltd. - INV-2026-001", vDef.Narration)

	// RemoteID
	assert.Contains(t, vDef.RemoteID, tenantID.String())
	assert.Contains(t, vDef.RemoteID, doc.ID.String())

	masterRepo.AssertExpectations(t)
}

func TestVoucherBuilder_PartyGSTINNotFound(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	builder := service.NewVoucherBuilderService(masterRepo)

	inv := sampleInvoice()
	doc := buildTestDocument(t, inv)
	tenantID := doc.TenantID

	// GSTIN not found
	masterRepo.On("FindLedgerByGSTIN", mock.Anything, tenantID, "29AABCU9603R1ZM").
		Return(nil, domain.ErrNotFound)

	// Other lookups
	masterRepo.On("FindPurchaseLedger", mock.Anything, tenantID).
		Return(nil, domain.ErrNotFound)
	masterRepo.On("FindTaxLedger", mock.Anything, tenantID, mock.AnythingOfType("string"), mock.AnythingOfType("float64")).
		Return(nil, domain.ErrNotFound)
	masterRepo.On("FindStockItemByHSN", mock.Anything, tenantID, mock.AnythingOfType("string")).
		Return(nil, domain.ErrNotFound)
	masterRepo.On("FindUnitBySymbol", mock.Anything, tenantID, mock.AnythingOfType("string")).
		Return(nil, domain.ErrNotFound)
	masterRepo.On("GetDefaultGodown", mock.Anything, tenantID).
		Return(nil, domain.ErrNotFound)

	vDef, err := builder.Build(context.Background(), tenantID, doc)
	require.NoError(t, err)
	require.NotNil(t, vDef)

	// Party ledger should be the normalized company name
	assert.Equal(t, "ACME PVT LTD", vDef.PartyLedger)
	assert.Equal(t, "convention", vDef.MatchConfidence["party_ledger"])

	masterRepo.AssertExpectations(t)
}

func TestVoucherBuilder_NoStockItem(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	builder := service.NewVoucherBuilderService(masterRepo)

	inv := sampleInvoice()
	doc := buildTestDocument(t, inv)
	tenantID := doc.TenantID

	// Party and purchase
	masterRepo.On("FindLedgerByGSTIN", mock.Anything, tenantID, mock.AnythingOfType("string")).
		Return(nil, domain.ErrNotFound)
	masterRepo.On("FindPurchaseLedger", mock.Anything, tenantID).
		Return(nil, domain.ErrNotFound)
	masterRepo.On("FindTaxLedger", mock.Anything, tenantID, mock.AnythingOfType("string"), mock.AnythingOfType("float64")).
		Return(nil, domain.ErrNotFound)

	// Stock items NOT found by HSN
	masterRepo.On("FindStockItemByHSN", mock.Anything, tenantID, mock.AnythingOfType("string")).
		Return(nil, domain.ErrNotFound)
	masterRepo.On("FindUnitBySymbol", mock.Anything, tenantID, mock.AnythingOfType("string")).
		Return(nil, domain.ErrNotFound)
	masterRepo.On("GetDefaultGodown", mock.Anything, tenantID).
		Return(nil, domain.ErrNotFound)

	vDef, err := builder.Build(context.Background(), tenantID, doc)
	require.NoError(t, err)
	require.NotNil(t, vDef)

	// Items should fall back to description
	require.Len(t, vDef.InventoryItems, 2)
	assert.Equal(t, "Widget A", vDef.InventoryItems[0].StockItem)
	assert.Equal(t, "84713010", vDef.InventoryItems[0].HSNCode)
	assert.Equal(t, "description_fallback", vDef.MatchConfidence["item_0"])

	assert.Equal(t, "Widget B", vDef.InventoryItems[1].StockItem)
	assert.Equal(t, "description_fallback", vDef.MatchConfidence["item_1"])

	masterRepo.AssertExpectations(t)
}

func TestVoucherBuilder_NoTaxLedger(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	builder := service.NewVoucherBuilderService(masterRepo)

	inv := sampleInvoice()
	doc := buildTestDocument(t, inv)
	tenantID := doc.TenantID

	// Party and purchase
	masterRepo.On("FindLedgerByGSTIN", mock.Anything, tenantID, mock.AnythingOfType("string")).
		Return(&domain.TallyLedger{Name: "Acme Pvt Ltd"}, nil)
	masterRepo.On("FindPurchaseLedger", mock.Anything, tenantID).
		Return(&domain.TallyLedger{Name: "Purchase Account"}, nil)

	// Tax ledgers NOT found
	masterRepo.On("FindTaxLedger", mock.Anything, tenantID, "CGST", mock.AnythingOfType("float64")).
		Return(nil, domain.ErrNotFound)
	masterRepo.On("FindTaxLedger", mock.Anything, tenantID, "SGST", mock.AnythingOfType("float64")).
		Return(nil, domain.ErrNotFound)

	// Stock items, UOM, godown
	masterRepo.On("FindStockItemByHSN", mock.Anything, tenantID, mock.AnythingOfType("string")).
		Return(nil, domain.ErrNotFound)
	masterRepo.On("FindUnitBySymbol", mock.Anything, tenantID, mock.AnythingOfType("string")).
		Return(nil, domain.ErrNotFound)
	masterRepo.On("GetDefaultGodown", mock.Anything, tenantID).
		Return(nil, domain.ErrNotFound)

	vDef, err := builder.Build(context.Background(), tenantID, doc)
	require.NoError(t, err)
	require.NotNil(t, vDef)

	// Tax entries should use convention names
	require.Len(t, vDef.TaxEntries, 2)
	assert.Equal(t, "Input CGST @9%", vDef.TaxEntries[0].LedgerName)
	assert.Equal(t, 180.0, vDef.TaxEntries[0].Amount)
	assert.Equal(t, "convention", vDef.MatchConfidence["tax_cgst"])

	assert.Equal(t, "Input SGST @9%", vDef.TaxEntries[1].LedgerName)
	assert.Equal(t, 180.0, vDef.TaxEntries[1].Amount)
	assert.Equal(t, "convention", vDef.MatchConfidence["tax_sgst"])

	masterRepo.AssertExpectations(t)
}

func TestVoucherBuilder_EmptyStructuredData(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	builder := service.NewVoucherBuilderService(masterRepo)

	doc := &domain.Document{
		ID:             uuid.New(),
		TenantID:       uuid.New(),
		StructuredData: nil,
	}

	vDef, err := builder.Build(context.Background(), doc.TenantID, doc)
	assert.Error(t, err)
	assert.Nil(t, vDef)
	assert.Contains(t, err.Error(), "no structured data")
}

func TestVoucherBuilder_InterstateTax(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	builder := service.NewVoucherBuilderService(masterRepo)

	inv := &invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{
			InvoiceNumber: "INV-IGST-001",
			InvoiceDate:   "20/02/2026",
			PlaceOfSupply: "Maharashtra",
		},
		Seller: invoice.Party{
			Name:      "Interstate Supplier",
			GSTIN:     "27AABCI1234R1ZM",
			State:     "Maharashtra",
			StateCode: "27",
		},
		Buyer: invoice.Party{
			Name:      "Local Buyer",
			GSTIN:     "29AABCL5678R1Z5",
			State:     "Karnataka",
			StateCode: "29",
		},
		LineItems: []invoice.LineItem{
			{
				Description:   "Server Rack",
				HSNSACCode:    "73089090",
				Quantity:      2,
				Unit:          "Nos",
				UnitPrice:     5000,
				TaxableAmount: 10000,
				IGSTRate:      18,
				IGSTAmount:    1800,
				Total:         11800,
			},
		},
		Totals: invoice.Totals{
			Subtotal:      10000,
			TaxableAmount: 10000,
			IGST:          1800,
			Total:         11800,
		},
	}

	doc := buildTestDocument(t, inv)
	tenantID := doc.TenantID

	// Party
	masterRepo.On("FindLedgerByGSTIN", mock.Anything, tenantID, "27AABCI1234R1ZM").
		Return(nil, domain.ErrNotFound)

	// Purchase
	masterRepo.On("FindPurchaseLedger", mock.Anything, tenantID).
		Return(nil, domain.ErrNotFound)

	// IGST tax ledger found
	masterRepo.On("FindTaxLedger", mock.Anything, tenantID, "IGST", mock.AnythingOfType("float64")).
		Return(&domain.TallyLedger{Name: "Input IGST @18%"}, nil)

	// Stock item
	masterRepo.On("FindStockItemByHSN", mock.Anything, tenantID, "73089090").
		Return(nil, domain.ErrNotFound)

	// UOM, godown
	masterRepo.On("FindUnitBySymbol", mock.Anything, tenantID, "Nos").
		Return(nil, domain.ErrNotFound)
	masterRepo.On("GetDefaultGodown", mock.Anything, tenantID).
		Return(nil, domain.ErrNotFound)

	vDef, err := builder.Build(context.Background(), tenantID, doc)
	require.NoError(t, err)
	require.NotNil(t, vDef)

	// Should have only IGST, no CGST/SGST
	require.Len(t, vDef.TaxEntries, 1)
	assert.Equal(t, "Input IGST @18%", vDef.TaxEntries[0].LedgerName)
	assert.Equal(t, 1800.0, vDef.TaxEntries[0].Amount)
	assert.Equal(t, "exact_rate", vDef.MatchConfidence["tax_igst"])

	// No CGST/SGST in confidence
	_, hasCGST := vDef.MatchConfidence["tax_cgst"]
	_, hasSGST := vDef.MatchConfidence["tax_sgst"]
	assert.False(t, hasCGST)
	assert.False(t, hasSGST)

	assert.Equal(t, 11800.0, vDef.TotalAmount)

	masterRepo.AssertExpectations(t)
}

func TestVoucherBuilder_NoLineItems(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	builder := service.NewVoucherBuilderService(masterRepo)

	inv := &invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{
			InvoiceNumber: "INV-NOLI-001",
			InvoiceDate:   "10/01/2026",
		},
		Seller: invoice.Party{
			Name:  "Simple Vendor",
			GSTIN: "29AABCS0001R1Z1",
		},
		LineItems: nil,
		Totals: invoice.Totals{
			TaxableAmount: 5000,
			CGST:          450,
			SGST:          450,
			Total:         5900,
		},
	}

	doc := buildTestDocument(t, inv)
	tenantID := doc.TenantID

	masterRepo.On("FindLedgerByGSTIN", mock.Anything, tenantID, "29AABCS0001R1Z1").
		Return(nil, domain.ErrNotFound)
	masterRepo.On("FindPurchaseLedger", mock.Anything, tenantID).
		Return(nil, domain.ErrNotFound)
	masterRepo.On("FindTaxLedger", mock.Anything, tenantID, mock.AnythingOfType("string"), mock.AnythingOfType("float64")).
		Return(nil, domain.ErrNotFound)

	vDef, err := builder.Build(context.Background(), tenantID, doc)
	require.NoError(t, err)
	require.NotNil(t, vDef)

	// Inventory items should be empty/nil
	assert.Empty(t, vDef.InventoryItems)

	// Tax entries should still be present
	require.Len(t, vDef.TaxEntries, 2)

	assert.Equal(t, 5900.0, vDef.TotalAmount)

	masterRepo.AssertExpectations(t)
}

func TestVoucherBuilder_MatchConfidence(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	builder := service.NewVoucherBuilderService(masterRepo)

	inv := sampleInvoice()
	// Add a line item with no HSN
	inv.LineItems = append(inv.LineItems, invoice.LineItem{
		Description:   "Service Fee",
		HSNSACCode:    "",
		Quantity:      1,
		Unit:          "Nos",
		UnitPrice:     500,
		TaxableAmount: 500,
	})

	doc := buildTestDocument(t, inv)
	tenantID := doc.TenantID

	// Party: GSTIN found
	masterRepo.On("FindLedgerByGSTIN", mock.Anything, tenantID, "29AABCU9603R1ZM").
		Return(&domain.TallyLedger{Name: "Acme Pvt Ltd"}, nil)

	// Purchase: found in masters
	masterRepo.On("FindPurchaseLedger", mock.Anything, tenantID).
		Return(&domain.TallyLedger{Name: "Purchase Account"}, nil)

	// Tax: CGST found, SGST not found
	masterRepo.On("FindTaxLedger", mock.Anything, tenantID, "CGST", mock.AnythingOfType("float64")).
		Return(&domain.TallyLedger{Name: "Input CGST @9%"}, nil)
	masterRepo.On("FindTaxLedger", mock.Anything, tenantID, "SGST", mock.AnythingOfType("float64")).
		Return(nil, domain.ErrNotFound)

	// Stock item 0: HSN found
	masterRepo.On("FindStockItemByHSN", mock.Anything, tenantID, "84713010").
		Return(&domain.TallyStockItem{Name: "Laptop"}, nil)
	// Stock item 1: HSN not found
	masterRepo.On("FindStockItemByHSN", mock.Anything, tenantID, "84714100").
		Return(nil, domain.ErrNotFound)
	// Stock item 2: no HSN (no FindStockItemByHSN call)

	masterRepo.On("FindUnitBySymbol", mock.Anything, tenantID, "Nos").
		Return(&domain.TallyUnit{Symbol: "Nos"}, nil)
	masterRepo.On("FindUnitBySymbol", mock.Anything, tenantID, "Pcs").
		Return(nil, domain.ErrNotFound)
	masterRepo.On("GetDefaultGodown", mock.Anything, tenantID).
		Return(&domain.TallyGodown{Name: "Warehouse A"}, nil)

	vDef, err := builder.Build(context.Background(), tenantID, doc)
	require.NoError(t, err)
	require.NotNil(t, vDef)

	// Verify all confidence entries
	assert.Equal(t, "exact_gstin", vDef.MatchConfidence["party_ledger"])
	assert.Equal(t, "exact_group", vDef.MatchConfidence["purchase_ledger"])
	assert.Equal(t, "exact_rate", vDef.MatchConfidence["tax_cgst"])
	assert.Equal(t, "convention", vDef.MatchConfidence["tax_sgst"])
	assert.Equal(t, "exact_hsn", vDef.MatchConfidence["item_0"])
	assert.Equal(t, "description_fallback", vDef.MatchConfidence["item_1"])
	assert.Equal(t, "no_hsn", vDef.MatchConfidence["item_2"])

	// Verify the no-HSN item uses description
	require.Len(t, vDef.InventoryItems, 3)
	assert.Equal(t, "Service Fee", vDef.InventoryItems[2].StockItem)
	assert.Equal(t, "", vDef.InventoryItems[2].HSNCode)

	masterRepo.AssertExpectations(t)
}

func TestVoucherBuilder_InvalidJSON(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	builder := service.NewVoucherBuilderService(masterRepo)

	doc := &domain.Document{
		ID:             uuid.New(),
		TenantID:       uuid.New(),
		StructuredData: json.RawMessage(`{invalid json`),
	}

	vDef, err := builder.Build(context.Background(), doc.TenantID, doc)
	assert.Error(t, err)
	assert.Nil(t, vDef)
	assert.Contains(t, err.Error(), "unmarshal structured data")
}

func TestVoucherBuilder_ZeroQuantityDefaultsToOne(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	builder := service.NewVoucherBuilderService(masterRepo)

	inv := &invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{
			InvoiceNumber: "INV-QTY-001",
			InvoiceDate:   "01/01/2026",
		},
		Seller: invoice.Party{
			Name: "Vendor X",
		},
		LineItems: []invoice.LineItem{
			{
				Description:   "Subscription",
				HSNSACCode:    "998314",
				Quantity:      0, // zero quantity
				UnitPrice:     0,
				TaxableAmount: 3000,
			},
		},
		Totals: invoice.Totals{
			TaxableAmount: 3000,
			Total:         3000,
		},
	}

	doc := buildTestDocument(t, inv)
	tenantID := doc.TenantID

	masterRepo.On("FindLedgerByGSTIN", mock.Anything, tenantID, mock.AnythingOfType("string")).
		Return(nil, domain.ErrNotFound).Maybe()
	masterRepo.On("FindPurchaseLedger", mock.Anything, tenantID).
		Return(nil, domain.ErrNotFound)
	masterRepo.On("FindStockItemByHSN", mock.Anything, tenantID, "998314").
		Return(nil, domain.ErrNotFound)
	masterRepo.On("FindUnitBySymbol", mock.Anything, tenantID, mock.AnythingOfType("string")).
		Return(nil, domain.ErrNotFound).Maybe()
	masterRepo.On("GetDefaultGodown", mock.Anything, tenantID).
		Return(nil, domain.ErrNotFound)

	vDef, err := builder.Build(context.Background(), tenantID, doc)
	require.NoError(t, err)
	require.NotNil(t, vDef)

	require.Len(t, vDef.InventoryItems, 1)
	assert.Equal(t, 1.0, vDef.InventoryItems[0].Quantity)
	assert.Equal(t, 3000.0, vDef.InventoryItems[0].Rate) // TaxableAmount / 1
	assert.Equal(t, 3000.0, vDef.InventoryItems[0].Amount)
}

func TestVoucherBuilder_EmptyUnitDefaultsToNos(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	builder := service.NewVoucherBuilderService(masterRepo)

	inv := &invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{
			InvoiceNumber: "INV-UOM-001",
			InvoiceDate:   "01/01/2026",
		},
		Seller: invoice.Party{
			Name: "Vendor Y",
		},
		LineItems: []invoice.LineItem{
			{
				Description:   "Misc Item",
				Quantity:      5,
				Unit:          "", // empty unit
				UnitPrice:     100,
				TaxableAmount: 500,
			},
		},
		Totals: invoice.Totals{
			TaxableAmount: 500,
			Total:         500,
		},
	}

	doc := buildTestDocument(t, inv)
	tenantID := doc.TenantID

	masterRepo.On("FindLedgerByGSTIN", mock.Anything, tenantID, mock.AnythingOfType("string")).
		Return(nil, domain.ErrNotFound).Maybe()
	masterRepo.On("FindPurchaseLedger", mock.Anything, tenantID).
		Return(nil, domain.ErrNotFound)
	masterRepo.On("FindStockItemByHSN", mock.Anything, tenantID, mock.AnythingOfType("string")).
		Return(nil, domain.ErrNotFound).Maybe()
	masterRepo.On("GetDefaultGodown", mock.Anything, tenantID).
		Return(nil, domain.ErrNotFound)

	vDef, err := builder.Build(context.Background(), tenantID, doc)
	require.NoError(t, err)
	require.NotNil(t, vDef)

	require.Len(t, vDef.InventoryItems, 1)
	assert.Equal(t, "Nos", vDef.InventoryItems[0].UOM)
}

func TestVoucherBuilder_PartyNoGSTINNoName(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	builder := service.NewVoucherBuilderService(masterRepo)

	inv := &invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{
			InvoiceNumber: "INV-NOPARTY-001",
			InvoiceDate:   "01/01/2026",
		},
		Seller:    invoice.Party{}, // empty seller
		LineItems: nil,
		Totals: invoice.Totals{
			Total: 1000,
		},
	}

	doc := buildTestDocument(t, inv)
	tenantID := doc.TenantID

	masterRepo.On("FindPurchaseLedger", mock.Anything, tenantID).
		Return(nil, domain.ErrNotFound)

	vDef, err := builder.Build(context.Background(), tenantID, doc)
	require.NoError(t, err)
	require.NotNil(t, vDef)

	assert.Equal(t, "Unknown Party", vDef.PartyLedger)
	assert.Equal(t, "convention", vDef.MatchConfidence["party_ledger"])
}
