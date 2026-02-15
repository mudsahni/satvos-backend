package tallyexport

import (
	"fmt"
	"io"
	"time"

	"satvos/internal/csvexport"
	"satvos/internal/domain"
)

// Writer accumulates Tally vouchers and renders the full XML on Flush.
type Writer struct {
	w        io.Writer
	opts     *TallyExportOptions
	vouchers []Voucher
	skipped  int
}

// NewWriter creates a Writer that will render Tally XML to w.
func NewWriter(w io.Writer, opts *TallyExportOptions) *Writer {
	if opts == nil {
		opts = &TallyExportOptions{}
	}
	opts.ApplyDefaults()
	return &Writer{w: w, opts: opts}
}

// WriteDocuments converts a batch of documents into vouchers and accumulates them.
// Returns the number of documents skipped in this batch.
func (tw *Writer) WriteDocuments(docs []domain.Document) int {
	vouchers, skipped := ConvertDocuments(docs, tw.opts)
	tw.vouchers = append(tw.vouchers, vouchers...)
	tw.skipped += skipped
	return skipped
}

// Flush renders the full Tally XML to the underlying writer.
func (tw *Writer) Flush() error {
	export := TallyExport{
		CompanyName: tw.opts.CompanyName,
		Vouchers:    tw.vouchers,
	}
	return tallyTemplate.Execute(tw.w, export)
}

// Skipped returns the total number of documents skipped across all WriteDocuments calls.
func (tw *Writer) Skipped() int {
	return tw.skipped
}

// BuildFilename returns a sanitized filename for Content-Disposition header.
// Format: {sanitized_collection_name}_{YYYY-MM-DD}.xml
func BuildFilename(collectionName string) string {
	sanitized := csvexport.SanitizeFilename(collectionName)
	date := time.Now().Format("2006-01-02")
	return fmt.Sprintf("%s_%s.xml", sanitized, date)
}
