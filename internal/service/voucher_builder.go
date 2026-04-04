package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	if utf8.RuneCountInString(narration) > 500 {
		runes := []rune(narration)
		narration = string(runes[:497]) + "..."
	}

	// 6. Normalize voucher date to YYYY-MM-DD (connector expects this format).
	// LLM parser returns DD-MM-YYYY per prompt instructions.
	now := time.Now()
	voucherDate := clampVoucherDate(normalizeVoucherDate(inv.Invoice.InvoiceDate, now), now)

	// 7. Determine voucher mode.
	// Mode depends on matched inventory items (not source line items), so the
	// same invoice may receive different modes for different tenants — a tenant
	// with matching stock items gets item_invoice while one without gets
	// accounting_invoice.
	var voucherMode string
	switch {
	case len(inventoryItems) > 0:
		voucherMode = domain.VoucherModeItemInvoice
	default:
		voucherMode = domain.VoucherModeAccountingInvoice
	}

	// 8. Build party details from seller — only when at least one
	// meaningful identifier (Name, GSTIN, PAN) is present.
	var partyDetails *domain.VoucherPartyDetail
	if inv.Seller.Name != "" || inv.Seller.GSTIN != "" || inv.Seller.PAN != "" {
		partyDetails = &domain.VoucherPartyDetail{
			Name:      inv.Seller.Name,
			Address:   inv.Seller.Address,
			PAN:       inv.Seller.PAN,
			GSTIN:     inv.Seller.GSTIN,
			State:     inv.Seller.State,
			StateCode: inv.Seller.StateCode,
		}
	}

	remoteID := fmt.Sprintf("%s-%s", tenantID, doc.ID)

	return &domain.VoucherDef{
		DocumentID:          doc.ID,
		VoucherType:         "Purchase",
		VoucherDate:         voucherDate,
		PartyLedger:         partyLedger,
		PurchaseLedger:      purchaseLedger,
		TaxEntries:          taxEntries,
		InventoryItems:      inventoryItems,
		TotalAmount:         inv.Totals.Total,
		Narration:           narration,
		RemoteID:            remoteID,
		MatchConfidence:     confidence,
		VoucherMode:         voucherMode,
		SupplierInvoiceNo:   inv.Invoice.InvoiceNumber,
		SupplierInvoiceDate: voucherDate,
		PartyDetails:        partyDetails,
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
	if overrides.VoucherMode != nil {
		switch *overrides.VoucherMode {
		case domain.VoucherModeAccountingInvoice, domain.VoucherModeItemInvoice, domain.VoucherModeJournal:
			vDef.VoucherMode = *overrides.VoucherMode
		default:
			return nil, fmt.Errorf("invalid voucher mode override: %q", *overrides.VoucherMode)
		}
	}

	return vDef, nil
}

// matchPartyLedger finds the party ledger by GSTIN, then normalized name, falling back to convention.
func (b *voucherBuilder) matchPartyLedger(ctx context.Context, tenantID uuid.UUID, seller *invoice.Party, confidence map[string]string) string {
	// 1. Try exact GSTIN match
	if seller.GSTIN != "" {
		ledger, err := b.masterRepo.FindLedgerByGSTIN(ctx, tenantID, seller.GSTIN)
		if err == nil && ledger != nil {
			confidence["party_ledger"] = "exact_gstin"
			return ledger.Name
		}
	}

	// 2. Try normalized name match against existing Tally ledgers
	if seller.Name != "" {
		normalizedName := normalize.CompanyName(seller.Name)
		ledger, err := b.masterRepo.FindLedgerByNormalizedName(ctx, tenantID, normalizedName)
		if err == nil && ledger != nil {
			confidence["party_ledger"] = "normalized_name"
			return ledger.Name // Use Tally's exact name, not our normalized version
		}
		// 3. Convention fallback — use normalized name
		confidence["party_ledger"] = "convention"
		return normalizedName
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
// Only includes items that match an actual stock item in Tally.
// Service invoices (ads, subscriptions, etc.) typically have no matching
// stock items, so they become accounting-only vouchers with no inventory.
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

	var items []domain.VoucherDefItem
	for idx := range inv.LineItems {
		li := &inv.LineItems[idx]
		itemKey := fmt.Sprintf("item_%d", idx)

		stockName, hsnCode, matched := b.matchStockItemStrict(ctx, tenantID, li, itemKey, confidence)
		if !matched {
			// No matching stock item in Tally — skip this line item from inventory.
			// The amount is still captured in the purchase ledger entry.
			confidence[itemKey] = "no_stock_item"
			continue
		}

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

// matchStockItemStrict finds a stock item only if it exists in Tally masters.
// Returns matched=false if no Tally stock item was found (service invoices, unknown items).
func (b *voucherBuilder) matchStockItemStrict(ctx context.Context, tenantID uuid.UUID, li *invoice.LineItem, itemKey string, confidence map[string]string) (stockName, hsnCode string, matched bool) {
	hsnCode = li.HSNSACCode

	if hsnCode != "" {
		stockItem, err := b.masterRepo.FindStockItemByHSN(ctx, tenantID, hsnCode)
		if err == nil && stockItem != nil {
			confidence[itemKey] = "exact_hsn"
			return stockItem.Name, hsnCode, true
		}
	}

	// No match in Tally masters — don't fabricate a stock item name
	return "", hsnCode, false
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
// Rounds to 2 decimal places to avoid floating point noise (e.g., 18.00003 → 18.00).
// Then snaps to standard GST rates if within 0.1% tolerance.
func calculateTaxRate(taxAmount, taxableAmount float64) float64 {
	if math.Abs(taxableAmount) < 0.01 {
		return 0
	}
	raw := (taxAmount / taxableAmount) * 100

	// Snap to standard Indian GST rates if close enough (within 0.1%)
	standardRates := []float64{0, 0.25, 1.5, 2.5, 3, 5, 6, 7.5, 9, 12, 14, 18, 28}
	for _, std := range standardRates {
		if diff := raw - std; diff < 0.1 && diff > -0.1 {
			return std
		}
	}

	// Round to 2 decimal places for non-standard rates
	return math.Round(raw*100) / 100
}

// formatRate formats a rate without trailing zeros.
func formatRate(rate float64) string {
	s := strconv.FormatFloat(rate, 'f', 2, 64)
	// Trim trailing zeros: "18.00" → "18", "2.50" → "2.5"
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

// normalizeVoucherDate converts a date string to YYYY-MM-DD format.
// Handles DD-MM-YYYY, DD/MM/YYYY, and passes through YYYY-MM-DD as-is.
// Returns now formatted as YYYY-MM-DD if the input is empty or unparseable.
func normalizeVoucherDate(dateStr string, now time.Time) string {
	dateStr = strings.TrimSpace(dateStr)
	today := now.Format("2006-01-02")
	if dateStr == "" {
		return today
	}

	// Replace slashes with dashes for uniform parsing
	dateStr = strings.ReplaceAll(dateStr, "/", "-")

	parts := strings.Split(dateStr, "-")
	if len(parts) != 3 {
		return today
	}

	// If first part is 4 digits, it's already YYYY-MM-DD
	if len(parts[0]) == 4 {
		if isValidDate(parts[0], parts[1], parts[2]) {
			return dateStr
		}
		return today
	}

	// Otherwise assume DD-MM-YYYY → YYYY-MM-DD
	if len(parts[2]) == 4 {
		result := parts[2] + "-" + parts[1] + "-" + parts[0]
		if isValidDate(parts[2], parts[1], parts[0]) {
			return result
		}
		return today
	}

	return today
}

func isValidDate(year, month, day string) bool {
	_, err := time.Parse("2006-01-02", year+"-"+month+"-"+day)
	return err == nil
}

// clampVoucherDate returns now's date if the parsed date is more than 18 months
// in the past or in the future (beyond tomorrow). This prevents out-of-range Tally errors.
func clampVoucherDate(dateStr string, now time.Time) string {
	parsed, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return now.Format("2006-01-02")
	}
	if parsed.Before(now.AddDate(0, -18, 0)) {
		return now.Format("2006-01-02")
	}
	if parsed.After(now.AddDate(0, 0, 1)) {
		return now.Format("2006-01-02")
	}
	return dateStr
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
	if inv.Invoice.InvoiceDate != "" {
		parts = append(parts, "dt. "+formatDisplayDate(inv.Invoice.InvoiceDate))
	}
	if len(inv.LineItems) > 0 && inv.LineItems[0].Description != "" {
		parts = append(parts, inv.LineItems[0].Description)
	}
	if len(parts) == 0 {
		return "Purchase"
	}
	return strings.Join(parts, " - ")
}

// formatDisplayDate normalizes an invoice date to DD/MM/YYYY display format.
// Handles DD-MM-YYYY, DD/MM/YYYY, and YYYY-MM-DD inputs.
func formatDisplayDate(dateStr string) string {
	dateStr = strings.TrimSpace(dateStr)
	if dateStr == "" {
		return ""
	}

	// Replace dashes with slashes for uniform handling
	dateStr = strings.ReplaceAll(dateStr, "-", "/")

	parts := strings.Split(dateStr, "/")
	if len(parts) != 3 {
		return dateStr
	}

	// If first part is 4 digits (YYYY/MM/DD), rearrange to DD/MM/YYYY
	if len(parts[0]) == 4 {
		return parts[2] + "/" + parts[1] + "/" + parts[0]
	}

	// Already DD/MM/YYYY
	return parts[0] + "/" + parts[1] + "/" + parts[2]
}
