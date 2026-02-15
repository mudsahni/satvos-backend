package tallyexport_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"satvos/internal/domain"
	"satvos/internal/tallyexport"
	"satvos/internal/validator/invoice"
)

func makeDoc(inv *invoice.GSTInvoice) domain.Document {
	data, _ := json.Marshal(inv)
	return domain.Document{
		ID:             uuid.New(),
		TenantID:       uuid.New(),
		ParsingStatus:  domain.ParsingStatusCompleted,
		StructuredData: data,
		CreatedAt:      time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
	}
}

func TestConvertDocuments_CGSTSGSTRoundTrip(t *testing.T) {
	inv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{
			InvoiceNumber: "INV-001",
			InvoiceDate:   "31/12/2025",
			PlaceOfSupply: "Delhi",
		},
		Seller: invoice.Party{
			Name:  "Acme Corp",
			GSTIN: "07AABCU9603R1ZM",
		},
		LineItems: []invoice.LineItem{
			{
				Description:   "Widget A",
				HSNSACCode:    "84719000",
				Quantity:      2,
				Unit:          "NOS",
				UnitPrice:     500,
				TaxableAmount: 1000,
				CGSTRate:      9,
				CGSTAmount:    90,
				SGSTRate:      9,
				SGSTAmount:    90,
				Total:         1180,
			},
			{
				Description:   "Widget B",
				HSNSACCode:    "84719001",
				Quantity:      1,
				Unit:          "PCS",
				UnitPrice:     200,
				TaxableAmount: 200,
				CGSTRate:      2.5,
				CGSTAmount:    5,
				SGSTRate:      2.5,
				SGSTAmount:    5,
				Total:         210,
			},
		},
		Totals: invoice.Totals{
			TaxableAmount: 1200,
			CGST:          95,
			SGST:          95,
			Total:         1390,
		},
	}

	docs := []domain.Document{makeDoc(&inv)}
	opts := &tallyexport.TallyExportOptions{CompanyName: "TestCo"}

	vouchers, skipped := tallyexport.ConvertDocuments(docs, opts)
	assert.Equal(t, 0, skipped)
	require.Len(t, vouchers, 1)

	v := vouchers[0]
	assert.Equal(t, "20251231", v.Date)
	assert.Equal(t, "31 Dec 25", v.DateOfInvoice)
	assert.Equal(t, "Acme Corp", v.PartyName)
	assert.Equal(t, "07AABCU9603R1ZM", v.PartyGSTIN)
	assert.Equal(t, "Regular", v.GSTRegType)
	assert.Equal(t, "INV-001", v.Reference)
	assert.Equal(t, "1390.00", v.PartyAmount)

	// 1 party + 4 tax ledgers (CGST@9%, CGST@2.5%, SGST@9%, SGST@2.5%)
	require.Len(t, v.LedgerEntries, 5)
	assert.Equal(t, "Yes", v.LedgerEntries[0].IsPartyLedger)
	assert.Equal(t, "No", v.LedgerEntries[0].IsDeemedPositive)
	assert.Equal(t, "1390.00", v.LedgerEntries[0].Amount)

	// Tax ledgers should be sorted and negative (debit)
	for _, le := range v.LedgerEntries[1:] {
		assert.Equal(t, "Yes", le.IsDeemedPositive)
		assert.Equal(t, "No", le.IsPartyLedger)
		assert.True(t, strings.HasPrefix(le.Amount, "-"))
	}

	// 2 inventory entries
	require.Len(t, v.InventoryEntries, 2)
	assert.Equal(t, "Widget A", v.InventoryEntries[0].StockItemName)
	assert.Equal(t, "-1000.00", v.InventoryEntries[0].Amount)
	assert.Equal(t, "Purchase@18%Gst", v.InventoryEntries[0].PurchaseLedger)
	assert.Equal(t, "Widget B", v.InventoryEntries[1].StockItemName)
	assert.Equal(t, "-200.00", v.InventoryEntries[1].Amount)
	assert.Equal(t, "Purchase@5%Gst", v.InventoryEntries[1].PurchaseLedger)
}

func TestConvertDocuments_IGSTLineItems(t *testing.T) {
	inv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{
			InvoiceNumber: "IGST-001",
			InvoiceDate:   "15/01/2026",
		},
		Seller: invoice.Party{
			Name:  "Interstate Supplier",
			GSTIN: "29AABCU9603R1ZM",
		},
		LineItems: []invoice.LineItem{
			{
				Description:   "Product X",
				Quantity:      5,
				Unit:          "NOS",
				UnitPrice:     100,
				TaxableAmount: 500,
				IGSTRate:      18,
				IGSTAmount:    90,
				Total:         590,
			},
		},
		Totals: invoice.Totals{Total: 590},
	}

	docs := []domain.Document{makeDoc(&inv)}
	opts := &tallyexport.TallyExportOptions{}
	vouchers, _ := tallyexport.ConvertDocuments(docs, opts)
	require.Len(t, vouchers, 1)

	v := vouchers[0]
	// 1 party + 1 IGST ledger
	require.Len(t, v.LedgerEntries, 2)
	assert.Equal(t, "Input Igst @18%", v.LedgerEntries[1].LedgerName)
	assert.Equal(t, "-90.00", v.LedgerEntries[1].Amount)

	// Inventory purchase ledger should reference IGST rate
	require.Len(t, v.InventoryEntries, 1)
	assert.Equal(t, "Purchase@18%Gst", v.InventoryEntries[0].PurchaseLedger)
}

func TestConvertDocuments_MixedRates(t *testing.T) {
	inv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{InvoiceNumber: "MIX-001", InvoiceDate: "01/01/2026"},
		Seller:  invoice.Party{Name: "Mixed Seller"},
		LineItems: []invoice.LineItem{
			{Description: "A", Quantity: 1, UnitPrice: 100, TaxableAmount: 100, CGSTRate: 9, CGSTAmount: 9, SGSTRate: 9, SGSTAmount: 9},
			{Description: "B", Quantity: 1, UnitPrice: 200, TaxableAmount: 200, CGSTRate: 2.5, CGSTAmount: 5, SGSTRate: 2.5, SGSTAmount: 5},
			{Description: "C", Quantity: 1, UnitPrice: 50, TaxableAmount: 50, CGSTRate: 9, CGSTAmount: 4.5, SGSTRate: 9, SGSTAmount: 4.5},
		},
		Totals: invoice.Totals{Total: 387},
	}

	docs := []domain.Document{makeDoc(&inv)}
	opts := &tallyexport.TallyExportOptions{}
	vouchers, _ := tallyexport.ConvertDocuments(docs, opts)
	require.Len(t, vouchers, 1)

	v := vouchers[0]
	// 1 party + 4 tax ledgers (CGST@2.5%, CGST@9%, SGST@2.5%, SGST@9%)
	require.Len(t, v.LedgerEntries, 5)

	// Verify grouping: CGST@9% should be 9+4.5=13.5
	cgst9 := findLedger(v.LedgerEntries, "Input Cgst @9%")
	require.NotNil(t, cgst9)
	assert.Equal(t, "-13.50", cgst9.Amount)

	cgst25 := findLedger(v.LedgerEntries, "Input Cgst @2.5%")
	require.NotNil(t, cgst25)
	assert.Equal(t, "-5.00", cgst25.Amount)
}

func TestConvertDocuments_SkipUnparsed(t *testing.T) {
	good := makeDoc(&invoice.GSTInvoice{
		Invoice:   invoice.InvoiceHeader{InvoiceNumber: "OK-001", InvoiceDate: "01/01/2026"},
		Seller:    invoice.Party{Name: "Good Seller"},
		LineItems: []invoice.LineItem{{Description: "Item", Quantity: 1, UnitPrice: 100, TaxableAmount: 100}},
		Totals:    invoice.Totals{Total: 100},
	})

	pending := domain.Document{
		ID:            uuid.New(),
		TenantID:      uuid.New(),
		ParsingStatus: domain.ParsingStatusPending,
	}

	failed := domain.Document{
		ID:            uuid.New(),
		TenantID:      uuid.New(),
		ParsingStatus: domain.ParsingStatusFailed,
	}

	docs := []domain.Document{pending, good, failed}
	opts := &tallyexport.TallyExportOptions{}
	vouchers, skipped := tallyexport.ConvertDocuments(docs, opts)

	assert.Equal(t, 2, skipped)
	assert.Len(t, vouchers, 1)
}

func TestConvertDocuments_SkipMalformedJSON(t *testing.T) {
	doc := domain.Document{
		ID:             uuid.New(),
		TenantID:       uuid.New(),
		ParsingStatus:  domain.ParsingStatusCompleted,
		StructuredData: json.RawMessage(`{invalid json`),
		CreatedAt:      time.Now(),
	}

	docs := []domain.Document{doc}
	opts := &tallyexport.TallyExportOptions{}
	vouchers, skipped := tallyexport.ConvertDocuments(docs, opts)

	assert.Equal(t, 1, skipped)
	assert.Empty(t, vouchers)
}

func TestWriter_XMLWellFormed(t *testing.T) {
	inv := invoice.GSTInvoice{
		Invoice:   invoice.InvoiceHeader{InvoiceNumber: "WF-001", InvoiceDate: "2025-06-15"},
		Seller:    invoice.Party{Name: "Well Formed Ltd", GSTIN: "07AABCU9603R1ZM"},
		LineItems: []invoice.LineItem{{Description: "Item", Quantity: 1, UnitPrice: 100, TaxableAmount: 100, CGSTRate: 9, CGSTAmount: 9, SGSTRate: 9, SGSTAmount: 9}},
		Totals:    invoice.Totals{Total: 118},
	}

	docs := []domain.Document{makeDoc(&inv)}
	var buf bytes.Buffer
	tw := tallyexport.NewWriter(&buf, &tallyexport.TallyExportOptions{CompanyName: "TestCo"})
	tw.WriteDocuments(docs)
	require.NoError(t, tw.Flush())

	// Verify the output is valid XML
	decoder := xml.NewDecoder(strings.NewReader(buf.String()))
	for {
		_, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("XML parse error: %v\nOutput:\n%s", err, buf.String())
		}
	}
}

func TestWriter_XMLEscaping(t *testing.T) {
	inv := invoice.GSTInvoice{
		Invoice:   invoice.InvoiceHeader{InvoiceNumber: "ESC&001", InvoiceDate: "01/01/2026"},
		Seller:    invoice.Party{Name: `Smith & Sons <"Pvt"> Ltd`},
		LineItems: []invoice.LineItem{{Description: "Item <special> & 'quoted'", Quantity: 1, UnitPrice: 100, TaxableAmount: 100}},
		Totals:    invoice.Totals{Total: 100},
	}

	docs := []domain.Document{makeDoc(&inv)}
	var buf bytes.Buffer
	tw := tallyexport.NewWriter(&buf, &tallyexport.TallyExportOptions{CompanyName: "Test & <Co>"})
	tw.WriteDocuments(docs)
	require.NoError(t, tw.Flush())

	output := buf.String()
	assert.Contains(t, output, "Smith &amp; Sons &lt;&quot;Pvt&quot;&gt; Ltd")
	assert.Contains(t, output, "Test &amp; &lt;Co&gt;")
	assert.Contains(t, output, "Item &lt;special&gt; &amp; &apos;quoted&apos;")

	// Must still be valid XML
	decoder := xml.NewDecoder(strings.NewReader(output))
	for {
		_, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("XML parse error: %v", err)
		}
	}
}

func TestDateParsing_MultipleFormats(t *testing.T) {
	tests := []struct {
		input       string
		wantDate    string
		wantDisplay string
	}{
		{"31/12/2025", "20251231", "31 Dec 25"},
		{"31-12-2025", "20251231", "31 Dec 25"},
		{"2025-12-31", "20251231", "31 Dec 25"},
		{"31 Dec 2025", "20251231", "31 Dec 25"},
		{"15/01/2026", "20260115", "15 Jan 26"},
		{"15.01.2026", "20260115", "15 Jan 26"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			inv := invoice.GSTInvoice{
				Invoice:   invoice.InvoiceHeader{InvoiceNumber: "DATE-001", InvoiceDate: tc.input},
				Seller:    invoice.Party{Name: "Date Seller"},
				LineItems: []invoice.LineItem{{Description: "Item", Quantity: 1, UnitPrice: 100, TaxableAmount: 100}},
				Totals:    invoice.Totals{Total: 100},
			}
			docs := []domain.Document{makeDoc(&inv)}
			opts := &tallyexport.TallyExportOptions{}
			vouchers, _ := tallyexport.ConvertDocuments(docs, opts)
			require.Len(t, vouchers, 1)
			assert.Equal(t, tc.wantDate, vouchers[0].Date)
			assert.Equal(t, tc.wantDisplay, vouchers[0].DateOfInvoice)
		})
	}
}

func TestConvertDocuments_EmptyLineItems(t *testing.T) {
	inv := invoice.GSTInvoice{
		Invoice:   invoice.InvoiceHeader{InvoiceNumber: "EMPTY-001", InvoiceDate: "01/01/2026"},
		Seller:    invoice.Party{Name: "Empty Seller"},
		LineItems: nil,
		Totals:    invoice.Totals{Total: 500},
	}

	docs := []domain.Document{makeDoc(&inv)}
	opts := &tallyexport.TallyExportOptions{}
	vouchers, _ := tallyexport.ConvertDocuments(docs, opts)
	require.Len(t, vouchers, 1)

	v := vouchers[0]
	// Only party ledger, no tax ledgers
	assert.Len(t, v.LedgerEntries, 1)
	assert.Empty(t, v.InventoryEntries)
	assert.Equal(t, "500.00", v.PartyAmount)
}

func TestBuildFilename(t *testing.T) {
	name := tallyexport.BuildFilename("My Collection (2025)")
	assert.True(t, strings.HasSuffix(name, ".xml"))
	assert.Contains(t, name, "My_Collection_2025")
	assert.NotContains(t, name, "(")
}

func TestWriter_EmptyCollection(t *testing.T) {
	var buf bytes.Buffer
	tw := tallyexport.NewWriter(&buf, &tallyexport.TallyExportOptions{CompanyName: "EmptyCo"})
	tw.WriteDocuments(nil)
	require.NoError(t, tw.Flush())

	output := buf.String()
	assert.Contains(t, output, "<SVCURRENTCOMPANY>EmptyCo</SVCURRENTCOMPANY>")
	assert.NotContains(t, output, "<VOUCHER")

	// Must still be valid XML
	decoder := xml.NewDecoder(strings.NewReader(output))
	for {
		_, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("XML parse error: %v", err)
		}
	}
}

func TestWriter_SkippedCount(t *testing.T) {
	good := makeDoc(&invoice.GSTInvoice{
		Invoice:   invoice.InvoiceHeader{InvoiceNumber: "OK-001", InvoiceDate: "01/01/2026"},
		Seller:    invoice.Party{Name: "Good"},
		LineItems: []invoice.LineItem{{Description: "X", Quantity: 1, UnitPrice: 10, TaxableAmount: 10}},
		Totals:    invoice.Totals{Total: 10},
	})
	bad := domain.Document{ID: uuid.New(), TenantID: uuid.New(), ParsingStatus: domain.ParsingStatusFailed}

	var buf bytes.Buffer
	tw := tallyexport.NewWriter(&buf, &tallyexport.TallyExportOptions{CompanyName: "Co"})

	// First batch
	tw.WriteDocuments([]domain.Document{good, bad})
	assert.Equal(t, 1, tw.Skipped())

	// Second batch
	tw.WriteDocuments([]domain.Document{bad, bad})
	assert.Equal(t, 3, tw.Skipped())
}

func TestConvertDocuments_UnregisteredParty(t *testing.T) {
	inv := invoice.GSTInvoice{
		Invoice:   invoice.InvoiceHeader{InvoiceNumber: "UNREG-001", InvoiceDate: "01/01/2026"},
		Seller:    invoice.Party{Name: "No GSTIN Seller"},
		LineItems: []invoice.LineItem{{Description: "Item", Quantity: 1, UnitPrice: 100, TaxableAmount: 100}},
		Totals:    invoice.Totals{Total: 100},
	}

	docs := []domain.Document{makeDoc(&inv)}
	opts := &tallyexport.TallyExportOptions{}
	vouchers, _ := tallyexport.ConvertDocuments(docs, opts)
	require.Len(t, vouchers, 1)
	assert.Equal(t, "Unregistered/Consumer", vouchers[0].GSTRegType)
}

func TestConvertDocuments_FallbackPartyAmount(t *testing.T) {
	inv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{InvoiceNumber: "FB-001", InvoiceDate: "01/01/2026"},
		Seller:  invoice.Party{Name: "Fallback Seller"},
		LineItems: []invoice.LineItem{
			{Description: "A", Quantity: 1, UnitPrice: 100, TaxableAmount: 100, CGSTRate: 9, CGSTAmount: 9, SGSTRate: 9, SGSTAmount: 9},
		},
		Totals: invoice.Totals{Total: 0}, // zero total triggers fallback
	}

	docs := []domain.Document{makeDoc(&inv)}
	opts := &tallyexport.TallyExportOptions{}
	vouchers, _ := tallyexport.ConvertDocuments(docs, opts)
	require.Len(t, vouchers, 1)
	assert.Equal(t, "118.00", vouchers[0].PartyAmount) // 100 taxable + 9 cgst + 9 sgst
}

func TestConvertDocuments_RemoteID(t *testing.T) {
	inv := invoice.GSTInvoice{
		Invoice:   invoice.InvoiceHeader{InvoiceNumber: "RID-001", InvoiceDate: "01/01/2026"},
		Seller:    invoice.Party{Name: "Remote Seller"},
		LineItems: []invoice.LineItem{{Description: "Item", Quantity: 1, UnitPrice: 100, TaxableAmount: 100}},
		Totals:    invoice.Totals{Total: 100},
	}

	doc := makeDoc(&inv)
	docs := []domain.Document{doc}
	opts := &tallyexport.TallyExportOptions{}
	vouchers, _ := tallyexport.ConvertDocuments(docs, opts)
	require.Len(t, vouchers, 1)

	expected := doc.TenantID.String() + "-" + doc.ID.String()
	assert.Equal(t, expected, vouchers[0].RemoteID)
}

func findLedger(entries []tallyexport.LedgerEntry, name string) *tallyexport.LedgerEntry {
	for i := range entries {
		if entries[i].LedgerName == name {
			return &entries[i]
		}
	}
	return nil
}
