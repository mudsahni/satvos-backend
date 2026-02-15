package tallyexport

// TallyExportOptions controls how Tally XML is generated.
type TallyExportOptions struct {
	CompanyName   string // SVCURRENTCOMPANY (defaults to collection name)
	DefaultGodown string // defaults "Main Location"
	DefaultUOM    string // defaults "NOS"
	DefaultBatch  string // defaults "Primary Batch"
}

// ApplyDefaults fills in zero-value fields with sensible defaults.
func (o *TallyExportOptions) ApplyDefaults() {
	if o.DefaultGodown == "" {
		o.DefaultGodown = "Main Location"
	}
	if o.DefaultUOM == "" {
		o.DefaultUOM = "NOS"
	}
	if o.DefaultBatch == "" {
		o.DefaultBatch = "Primary Batch"
	}
}

// Voucher holds the data needed to render one Tally purchase voucher.
type Voucher struct {
	Date             string // YYYYMMDD
	DateOfInvoice    string // "DD Mon YY"
	Reference        string
	VoucherNumber    string
	PartyName        string
	PartyGSTIN       string
	GSTRegType       string // "Regular" or "Unregistered/Consumer"
	PlaceOfSupply    string
	Narration        string
	PartyAmount      string // positive — credit to supplier
	RemoteID         string // tenantID-docID for idempotent reimport
	LedgerEntries    []LedgerEntry
	InventoryEntries []InventoryEntry
}

// LedgerEntry represents one LEDGERENTRIES.LIST in the voucher.
type LedgerEntry struct {
	LedgerName       string
	Amount           string
	IsDeemedPositive string // "Yes" or "No"
	IsPartyLedger    string // "Yes" or "No"
	TaxRate          string // for BASICRATEOFINVOICETAX
	HasBillAlloc     bool   // true only for party ledger
	BillRef          string // invoice number for BILLALLOCATIONS
}

// InventoryEntry represents one ALLINVENTORYENTRIES.LIST in the voucher.
type InventoryEntry struct {
	StockItemName  string
	HSNCode        string
	Rate           string // e.g. "100/NOS"
	Amount         string // negative — debit
	ActualQty      string // e.g. "1 NOS"
	BilledQty      string // e.g. "1 NOS"
	Godown         string
	BatchName      string
	PurchaseLedger string // e.g. "Purchase@5%Gst"
}

// TallyExport is the top-level structure rendered by the template.
type TallyExport struct {
	CompanyName string
	Vouchers    []Voucher
}
