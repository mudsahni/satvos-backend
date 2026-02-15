package tallyexport

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"satvos/internal/domain"
	"satvos/internal/validator/invoice"
)

// dateInputFormats lists the date formats we try when parsing invoice dates.
var dateInputFormats = []string{
	"02/01/2006",
	"02-01-2006",
	"2006-01-02",
	"02 Jan 2006",
	"02-Jan-2006",
	"2 Jan 2006",
	"02/01/06",
	"02-01-06",
	"January 2, 2006",
	"Jan 2, 2006",
	"02.01.2006",
}

type taxKey struct {
	name string
	rate float64
}

// ConvertDocuments converts parsed documents into Tally vouchers.
// It returns the vouchers and the count of skipped documents.
func ConvertDocuments(docs []domain.Document, opts *TallyExportOptions) (vouchers []Voucher, skipped int) {
	opts.ApplyDefaults()

	for i := range docs {
		doc := &docs[i]
		if doc.ParsingStatus != domain.ParsingStatusCompleted || len(doc.StructuredData) == 0 {
			skipped++
			continue
		}

		var inv invoice.GSTInvoice
		if err := json.Unmarshal(doc.StructuredData, &inv); err != nil {
			skipped++
			continue
		}

		v, ok := convertOne(doc, &inv, opts)
		if !ok {
			skipped++
			continue
		}
		vouchers = append(vouchers, v)
	}

	return
}

func convertOne(doc *domain.Document, inv *invoice.GSTInvoice, opts *TallyExportOptions) (Voucher, bool) {
	dateFmt, displayDate := parseDatePair(inv.Invoice.InvoiceDate, doc.CreatedAt)

	partyName := inv.Seller.Name
	if partyName == "" {
		partyName = "Unknown Party"
	}

	gstRegType := "Unregistered/Consumer"
	if inv.Seller.GSTIN != "" {
		gstRegType = "Regular"
	}

	ref := inv.Invoice.InvoiceNumber
	narration := buildNarration(inv)

	// Build inventory entries and collect tax amounts grouped by rate.
	taxMap := make(map[taxKey]float64)
	inventoryEntries := make([]InventoryEntry, 0, len(inv.LineItems))

	for idx := range inv.LineItems {
		li := &inv.LineItems[idx]
		qty := li.Quantity
		if qty == 0 {
			qty = 1
		}
		uom := li.Unit
		if uom == "" {
			uom = opts.DefaultUOM
		}

		unitPrice := li.UnitPrice
		if unitPrice == 0 && li.TaxableAmount > 0 {
			unitPrice = li.TaxableAmount / qty
		}

		amount := li.TaxableAmount
		if amount == 0 {
			amount = unitPrice * qty
		}

		var effectiveRate float64
		if li.IGSTRate > 0 {
			taxMap[taxKey{"Input Igst", li.IGSTRate}] += li.IGSTAmount
			effectiveRate = li.IGSTRate
		} else {
			if li.CGSTRate > 0 {
				taxMap[taxKey{"Input Cgst", li.CGSTRate}] += li.CGSTAmount
			}
			if li.SGSTRate > 0 {
				taxMap[taxKey{"Input Sgst", li.SGSTRate}] += li.SGSTAmount
			}
			effectiveRate = li.CGSTRate + li.SGSTRate
		}

		purchaseLedger := buildPurchaseLedger(effectiveRate)

		stockName := li.Description
		if stockName == "" {
			stockName = fmt.Sprintf("Item %d", idx+1)
		}

		inventoryEntries = append(inventoryEntries, InventoryEntry{
			StockItemName:  stockName,
			HSNCode:        li.HSNSACCode,
			Rate:           formatRate(unitPrice, uom),
			Amount:         formatAmount(-amount),
			ActualQty:      formatQty(qty, uom),
			BilledQty:      formatQty(qty, uom),
			Godown:         opts.DefaultGodown,
			BatchName:      opts.DefaultBatch,
			PurchaseLedger: purchaseLedger,
		})
	}

	// Build ledger entries: party first, then taxes sorted.
	partyAmount := inv.Totals.Total
	if partyAmount == 0 {
		// Fallback: sum taxable amounts + tax amounts
		for idx := range inv.LineItems {
			partyAmount += inv.LineItems[idx].TaxableAmount
		}
		for _, amt := range taxMap {
			partyAmount += amt
		}
	}

	// Sort tax entries by name for deterministic output.
	type taxEntry struct {
		key    taxKey
		amount float64
	}
	taxEntries := make([]taxEntry, 0, len(taxMap))
	for k, v := range taxMap {
		if v != 0 {
			taxEntries = append(taxEntries, taxEntry{k, v})
		}
	}
	sort.Slice(taxEntries, func(i, j int) bool {
		ki := taxEntries[i].key.name + "@" + formatCleanRate(taxEntries[i].key.rate)
		kj := taxEntries[j].key.name + "@" + formatCleanRate(taxEntries[j].key.rate)
		return ki < kj
	})

	ledgerEntries := make([]LedgerEntry, 0, 1+len(taxEntries))
	ledgerEntries = append(ledgerEntries, LedgerEntry{
		LedgerName:       partyName,
		Amount:           formatAmount(partyAmount),
		IsDeemedPositive: "No",
		IsPartyLedger:    "Yes",
		HasBillAlloc:     true,
		BillRef:          ref,
	})
	for _, te := range taxEntries {
		ledgerName := fmt.Sprintf("%s @%s%%", te.key.name, formatCleanRate(te.key.rate))
		ledgerEntries = append(ledgerEntries, LedgerEntry{
			LedgerName:       ledgerName,
			Amount:           formatAmount(-te.amount),
			IsDeemedPositive: "Yes",
			IsPartyLedger:    "No",
			TaxRate:          formatTaxRate(te.key.rate),
			HasBillAlloc:     false,
		})
	}

	remoteID := fmt.Sprintf("%s-%s", doc.TenantID, doc.ID)

	return Voucher{
		Date:             dateFmt,
		DateOfInvoice:    displayDate,
		Reference:        ref,
		VoucherNumber:    ref,
		PartyName:        partyName,
		PartyGSTIN:       inv.Seller.GSTIN,
		GSTRegType:       gstRegType,
		PlaceOfSupply:    inv.Invoice.PlaceOfSupply,
		Narration:        narration,
		PartyAmount:      formatAmount(partyAmount),
		RemoteID:         remoteID,
		LedgerEntries:    ledgerEntries,
		InventoryEntries: inventoryEntries,
	}, true
}

func parseDatePair(dateStr string, fallback time.Time) (dateFmt, displayDate string) {
	for _, layout := range dateInputFormats {
		if t, err := time.Parse(layout, dateStr); err == nil {
			return t.Format("20060102"), t.Format("2 Jan 06")
		}
	}
	return fallback.Format("20060102"), fallback.Format("2 Jan 06")
}

func buildNarration(inv *invoice.GSTInvoice) string {
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

func buildPurchaseLedger(effectiveRate float64) string {
	if effectiveRate == 0 {
		return "Purchase@0%Gst"
	}
	return fmt.Sprintf("Purchase@%s%%Gst", formatCleanRate(effectiveRate))
}

// formatCleanRate formats a rate without trailing zeros: 2.5 not 2.50, 18 not 18.00.
func formatCleanRate(rate float64) string {
	return strconv.FormatFloat(rate, 'f', -1, 64)
}

// formatTaxRate formats for BASICRATEOFINVOICETAX (2 decimal places matching reference XML).
func formatTaxRate(rate float64) string {
	return strconv.FormatFloat(rate, 'f', 2, 64)
}

func formatAmount(v float64) string {
	return strconv.FormatFloat(roundToTwo(v), 'f', 2, 64)
}

func formatRate(unitPrice float64, uom string) string {
	return fmt.Sprintf("%s/%s", formatAmount(unitPrice), uom)
}

func formatQty(qty float64, uom string) string {
	qtyStr := strconv.FormatFloat(qty, 'f', -1, 64)
	return fmt.Sprintf("%s %s", qtyStr, uom)
}

func roundToTwo(v float64) float64 {
	return math.Round(v*100) / 100
}
