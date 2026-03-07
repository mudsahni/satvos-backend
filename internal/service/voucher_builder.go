package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/normalize"
	"satvos/internal/port"
	"satvos/internal/validator/invoice"
)

// NOTE: VoucherBuilderService interface is declared in sync_service.go

type voucherBuilder struct {
	masterRepo port.TallyMasterRepository
}

// NewVoucherBuilderService creates a new VoucherBuilderService implementation.
func NewVoucherBuilderService(masterRepo port.TallyMasterRepository) VoucherBuilderService {
	return &voucherBuilder{masterRepo: masterRepo}
}

// Build matches a parsed invoice against synced Tally masters to produce a VoucherDef.
func (b *voucherBuilder) Build(ctx context.Context, tenantID uuid.UUID, doc *domain.Document) (*domain.VoucherDef, error) {
	if len(doc.StructuredData) == 0 {
		return nil, errors.New("document has no structured data")
	}

	var inv invoice.GSTInvoice
	if err := json.Unmarshal(doc.StructuredData, &inv); err != nil {
		return nil, fmt.Errorf("unmarshal structured data: %w", err)
	}

	confidence := make(map[string]string)

	// 1. Match party ledger: GSTIN exact match -> normalized name fallback
	partyLedger := b.matchPartyLedger(ctx, tenantID, &inv.Seller, confidence)

	// 2. Match purchase ledger
	purchaseLedger := b.matchPurchaseLedger(ctx, tenantID, confidence)

	// 3. Match tax ledgers
	taxEntries := b.matchTaxLedgers(ctx, tenantID, &inv.Totals, confidence)

	// 4. Match inventory items
	inventoryItems := b.matchInventoryItems(ctx, tenantID, &inv, confidence)

	// 5. Build narration
	narration := buildVoucherNarration(&inv)

	// 6. Parse voucher date
	voucherDate := inv.Invoice.InvoiceDate

	remoteID := fmt.Sprintf("%s-%s", tenantID, doc.ID)

	return &domain.VoucherDef{
		DocumentID:      doc.ID,
		VoucherType:     "Purchase",
		VoucherDate:     voucherDate,
		PartyLedger:     partyLedger,
		PurchaseLedger:  purchaseLedger,
		TaxEntries:      taxEntries,
		InventoryItems:  inventoryItems,
		TotalAmount:     inv.Totals.Total,
		Narration:       narration,
		RemoteID:        remoteID,
		MatchConfidence: confidence,
	}, nil
}

// BuildWithOverrides matches a parsed invoice against synced Tally masters, then applies manual overrides.
func (b *voucherBuilder) BuildWithOverrides(ctx context.Context, tenantID uuid.UUID, doc *domain.Document, overrides *domain.VoucherOverrides) (*domain.VoucherDef, error) {
	vDef, err := b.Build(ctx, tenantID, doc)
	if err != nil {
		return nil, err
	}
	if overrides == nil {
		return vDef, nil
	}

	if overrides.PartyLedger != nil {
		vDef.PartyLedger = *overrides.PartyLedger
		vDef.MatchConfidence["party_ledger"] = "manual_override"
	}
	if overrides.PurchaseLedger != nil {
		vDef.PurchaseLedger = *overrides.PurchaseLedger
		vDef.MatchConfidence["purchase_ledger"] = "manual_override"
	}
	for rate, ledgerName := range overrides.TaxOverrides {
		// Find matching tax entry by rate key (e.g. "cgst", "sgst", "igst")
		confKey := "tax_" + rate
		for i := range vDef.TaxEntries {
			if vDef.MatchConfidence[confKey] != "" {
				vDef.TaxEntries[i].LedgerName = ledgerName
				vDef.MatchConfidence[confKey] = "manual_override"
				break
			}
		}
	}
	for itemName, stockItemName := range overrides.ItemOverrides {
		for i := range vDef.InventoryItems {
			if vDef.InventoryItems[i].StockItem == itemName {
				vDef.InventoryItems[i].StockItem = stockItemName
				itemKey := fmt.Sprintf("item_%d", i)
				vDef.MatchConfidence[itemKey] = "manual_override"
				break
			}
		}
	}

	return vDef, nil
}

// matchPartyLedger finds the party ledger by GSTIN, falling back to normalized name.
func (b *voucherBuilder) matchPartyLedger(ctx context.Context, tenantID uuid.UUID, seller *invoice.Party, confidence map[string]string) string {
	if seller.GSTIN != "" {
		ledger, err := b.masterRepo.FindLedgerByGSTIN(ctx, tenantID, seller.GSTIN)
		if err == nil && ledger != nil {
			confidence["party_ledger"] = "exact_gstin"
			return ledger.Name
		}
	}

	// Fallback to normalized company name
	name := seller.Name
	if name != "" {
		confidence["party_ledger"] = "convention"
		return normalize.CompanyName(name)
	}

	confidence["party_ledger"] = "convention"
	return "Unknown Party"
}

// matchPurchaseLedger finds the purchase ledger in masters, falling back to default.
func (b *voucherBuilder) matchPurchaseLedger(ctx context.Context, tenantID uuid.UUID, confidence map[string]string) string {
	ledger, err := b.masterRepo.FindPurchaseLedger(ctx, tenantID)
	if err == nil && ledger != nil {
		confidence["purchase_ledger"] = "exact_group"
		return ledger.Name
	}

	confidence["purchase_ledger"] = "convention"
	return "Purchase Accounts"
}

// matchTaxLedgers builds tax entries for each tax type with amount > 0.
func (b *voucherBuilder) matchTaxLedgers(ctx context.Context, tenantID uuid.UUID, totals *invoice.Totals, confidence map[string]string) []domain.VoucherDefTaxEntry {
	var entries []domain.VoucherDefTaxEntry

	type taxInfo struct {
		taxType string
		amount  float64
	}

	var taxes []taxInfo
	if totals.CGST > 0 {
		taxes = append(taxes, taxInfo{taxType: "CGST", amount: totals.CGST})
	}
	if totals.SGST > 0 {
		taxes = append(taxes, taxInfo{taxType: "SGST", amount: totals.SGST})
	}
	if totals.IGST > 0 {
		taxes = append(taxes, taxInfo{taxType: "IGST", amount: totals.IGST})
	}

	for _, t := range taxes {
		rate := calculateTaxRate(t.amount, totals.TaxableAmount)

		ledger, err := b.masterRepo.FindTaxLedger(ctx, tenantID, t.taxType, rate)
		if err == nil && ledger != nil {
			confidence["tax_"+strings.ToLower(t.taxType)] = "exact_rate"
			entries = append(entries, domain.VoucherDefTaxEntry{
				LedgerName: ledger.Name,
				Amount:     t.amount,
			})
		} else {
			// Convention-based fallback: "Input CGST @9%"
			conventionName := fmt.Sprintf("Input %s @%s%%", t.taxType, formatRate(rate))
			confidence["tax_"+strings.ToLower(t.taxType)] = "convention"
			entries = append(entries, domain.VoucherDefTaxEntry{
				LedgerName: conventionName,
				Amount:     t.amount,
			})
		}
	}

	return entries
}

// matchInventoryItems builds inventory items from line items.
func (b *voucherBuilder) matchInventoryItems(ctx context.Context, tenantID uuid.UUID, inv *invoice.GSTInvoice, confidence map[string]string) []domain.VoucherDefItem {
	if len(inv.LineItems) == 0 {
		return nil
	}

	// Get default godown
	godownName := "Main Location"
	godown, err := b.masterRepo.GetDefaultGodown(ctx, tenantID)
	if err == nil && godown != nil {
		godownName = godown.Name
	}

	items := make([]domain.VoucherDefItem, 0, len(inv.LineItems))
	for idx := range inv.LineItems {
		li := &inv.LineItems[idx]
		itemKey := fmt.Sprintf("item_%d", idx)

		stockName, hsnCode := b.matchStockItem(ctx, tenantID, li, itemKey, confidence)
		uom := b.matchUOM(ctx, tenantID, li.Unit)

		qty := li.Quantity
		if qty == 0 {
			qty = 1
		}

		rate := li.UnitPrice
		if rate == 0 && li.TaxableAmount > 0 {
			rate = li.TaxableAmount / qty
		}

		amount := li.TaxableAmount
		if amount == 0 {
			amount = rate * qty
		}

		items = append(items, domain.VoucherDefItem{
			StockItem: stockName,
			Quantity:  qty,
			Rate:      rate,
			Amount:    amount,
			UOM:       uom,
			Godown:    godownName,
			HSNCode:   hsnCode,
		})
	}

	return items
}

// matchStockItem finds a stock item by HSN, falling back to description.
func (b *voucherBuilder) matchStockItem(ctx context.Context, tenantID uuid.UUID, li *invoice.LineItem, itemKey string, confidence map[string]string) (stockName, hsnCode string) {
	hsnCode = li.HSNSACCode

	if hsnCode != "" {
		stockItem, err := b.masterRepo.FindStockItemByHSN(ctx, tenantID, hsnCode)
		if err == nil && stockItem != nil {
			confidence[itemKey] = "exact_hsn"
			return stockItem.Name, hsnCode
		}

		// HSN present but not found in masters
		if li.Description != "" {
			confidence[itemKey] = "description_fallback"
			return li.Description, hsnCode
		}
		confidence[itemKey] = "description_fallback"
		return fmt.Sprintf("Item (HSN: %s)", hsnCode), hsnCode
	}

	// No HSN code at all
	confidence[itemKey] = "no_hsn"
	if li.Description != "" {
		return li.Description, ""
	}
	return "Unknown Item", ""
}

// matchUOM finds the unit of measure by symbol, falling back to the raw unit string.
func (b *voucherBuilder) matchUOM(ctx context.Context, tenantID uuid.UUID, unit string) string {
	if unit == "" {
		return "Nos"
	}

	tallyUnit, err := b.masterRepo.FindUnitBySymbol(ctx, tenantID, unit)
	if err == nil && tallyUnit != nil {
		return tallyUnit.Symbol
	}

	return unit
}

// calculateTaxRate derives the tax rate from the tax amount and taxable amount.
func calculateTaxRate(taxAmount, taxableAmount float64) float64 {
	if taxableAmount == 0 {
		return 0
	}
	return (taxAmount / taxableAmount) * 100
}

// formatRate formats a rate without trailing zeros.
func formatRate(rate float64) string {
	return strconv.FormatFloat(rate, 'f', -1, 64)
}

// buildVoucherNarration builds a narration string for the voucher.
func buildVoucherNarration(inv *invoice.GSTInvoice) string {
	var parts []string
	if inv.Seller.Name != "" {
		parts = append(parts, inv.Seller.Name)
	}
	if inv.Invoice.InvoiceNumber != "" {
		parts = append(parts, inv.Invoice.InvoiceNumber)
	}
	if len(parts) == 0 {
		return "Purchase"
	}
	return strings.Join(parts, " - ")
}
