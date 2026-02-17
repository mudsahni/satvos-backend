package csvexport

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"satvos/internal/domain"
	"satvos/internal/validator/invoice"
)

// UTF-8 BOM bytes for Excel compatibility on Windows.
var BOM = []byte{0xEF, 0xBB, 0xBF}

// columns defines the CSV header row.
var columns = []string{
	"Document Name",
	"Parsing Status",
	"Review Status",
	"Validation Status",
	"Reconciliation Status",
	"Invoice Number",
	"IRN",
	"Acknowledgement Number",
	"Acknowledgement Date",
	"Invoice Date",
	"Invoice Type",
	"Place of Supply",
	"Reverse Charge",
	"Seller Name",
	"Seller GSTIN",
	"Seller State",
	"Seller State Code",
	"Buyer Name",
	"Buyer GSTIN",
	"Buyer State",
	"Buyer State Code",
	"Taxable Amount",
	"CGST",
	"SGST",
	"IGST",
	"Cess",
	"Total",
	"Currency",
	"Due Date",
	"Line Item Count",
	"Reviewer Notes",
	"Parsed At",
	"Created At",
	"Line Item Index",
	"Line Description",
	"HSN/SAC Code",
	"Quantity",
	"Unit",
	"Unit Price",
	"Line Discount",
	"Line Taxable Amount",
	"Line CGST Rate",
	"Line CGST Amount",
	"Line SGST Rate",
	"Line SGST Amount",
	"Line IGST Rate",
	"Line IGST Amount",
	"Line Total",
}

// Writer wraps csv.Writer for exporting documents as CSV.
type Writer struct {
	csv *csv.Writer
}

// NewWriter creates a Writer that writes CSV to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{csv: csv.NewWriter(w)}
}

// WriteHeader writes the header row.
func (w *Writer) WriteHeader() error {
	return w.csv.Write(columns)
}

// WriteDocuments converts a batch of documents to CSV rows and writes them.
// Export format is row-per-line-item with document/invoice columns repeated.
func (w *Writer) WriteDocuments(docs []domain.Document) error {
	for i := range docs {
		rows := documentToRows(&docs[i])
		for j := range rows {
			if err := w.csv.Write(rows[j]); err != nil {
				return err
			}
		}
	}
	return nil
}

// Flush flushes the underlying csv.Writer buffer.
func (w *Writer) Flush() {
	w.csv.Flush()
}

// Error returns any error from the underlying csv.Writer.
func (w *Writer) Error() error {
	return w.csv.Error()
}

// documentToRows converts a document to CSV rows.
// For parsed documents with line items: one row per line item.
// Otherwise: one row with metadata/invoice fields and empty line-item fields.
func documentToRows(doc *domain.Document) [][]string {
	row := make([]string, len(columns))

	// Metadata columns (always filled)
	row[0] = doc.Name
	row[1] = string(doc.ParsingStatus)
	row[2] = string(doc.ReviewStatus)
	row[3] = string(doc.ValidationStatus)
	row[4] = string(doc.ReconciliationStatus)
	row[30] = doc.ReviewerNotes
	row[31] = formatTime(doc.ParsedAt)
	row[32] = doc.CreatedAt.Format(time.RFC3339)

	// Invoice columns: only if parsing completed and JSON is valid
	if doc.ParsingStatus != domain.ParsingStatusCompleted || len(doc.StructuredData) == 0 {
		return [][]string{row}
	}

	var inv invoice.GSTInvoice
	if err := json.Unmarshal(doc.StructuredData, &inv); err != nil {
		return [][]string{row}
	}

	row[5] = inv.Invoice.InvoiceNumber
	row[6] = inv.Invoice.IRN
	row[7] = inv.Invoice.AcknowledgementNumber
	row[8] = inv.Invoice.AcknowledgementDate
	row[9] = inv.Invoice.InvoiceDate
	row[10] = inv.Invoice.InvoiceType
	row[11] = inv.Invoice.PlaceOfSupply
	row[12] = formatBool(inv.Invoice.ReverseCharge)
	row[13] = inv.Seller.Name
	row[14] = inv.Seller.GSTIN
	row[15] = inv.Seller.State
	row[16] = inv.Seller.StateCode
	row[17] = inv.Buyer.Name
	row[18] = inv.Buyer.GSTIN
	row[19] = inv.Buyer.State
	row[20] = inv.Buyer.StateCode
	row[21] = formatMoney(inv.Totals.TaxableAmount)
	row[22] = formatMoney(inv.Totals.CGST)
	row[23] = formatMoney(inv.Totals.SGST)
	row[24] = formatMoney(inv.Totals.IGST)
	row[25] = formatMoney(inv.Totals.Cess)
	row[26] = formatMoney(inv.Totals.Total)
	row[27] = inv.Invoice.Currency
	row[28] = inv.Invoice.DueDate
	row[29] = strconv.Itoa(len(inv.LineItems))

	if len(inv.LineItems) == 0 {
		return [][]string{row}
	}

	rows := make([][]string, 0, len(inv.LineItems))
	for i := range inv.LineItems {
		li := inv.LineItems[i]
		r := append([]string(nil), row...)
		r[33] = strconv.Itoa(i + 1)
		r[34] = li.Description
		r[35] = li.HSNSACCode
		r[36] = formatMoney(li.Quantity)
		r[37] = li.Unit
		r[38] = formatMoney(li.UnitPrice)
		r[39] = formatMoney(li.Discount)
		r[40] = formatMoney(li.TaxableAmount)
		r[41] = formatMoney(li.CGSTRate)
		r[42] = formatMoney(li.CGSTAmount)
		r[43] = formatMoney(li.SGSTRate)
		r[44] = formatMoney(li.SGSTAmount)
		r[45] = formatMoney(li.IGSTRate)
		r[46] = formatMoney(li.IGSTAmount)
		r[47] = formatMoney(li.Total)
		rows = append(rows, r)
	}

	return rows
}

func formatMoney(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func formatBool(v bool) string {
	if v {
		return "Yes"
	}
	return "No"
}

func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// nonAlphanumeric matches characters that are not alphanumeric, hyphen, or underscore.
var nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// multiUnderscore matches consecutive underscores.
var multiUnderscore = regexp.MustCompile(`_{2,}`)

// SanitizeFilename cleans a collection name for use in Content-Disposition.
// Replaces non-alphanumeric chars (except - _) with _, collapses consecutive
// underscores, and truncates to 100 chars.
func SanitizeFilename(name string) string {
	s := nonAlphanumeric.ReplaceAllString(name, "_")
	s = multiUnderscore.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if len(s) > 100 {
		s = s[:100]
	}
	return s
}

// BuildFilename returns a sanitized filename for Content-Disposition header.
// Format: {sanitized_collection_name}_{YYYY-MM-DD}.csv
func BuildFilename(collectionName string) string {
	sanitized := SanitizeFilename(collectionName)
	date := time.Now().Format("2006-01-02")
	return fmt.Sprintf("%s_%s.csv", sanitized, date)
}
