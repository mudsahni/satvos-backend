package port

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// DuplicateMatch holds enough information about a matching document for an actionable warning message.
type DuplicateMatch struct {
	DocumentID   uuid.UUID `db:"id"`
	DocumentName string    `db:"name"`
	MatchType    string    `db:"match_type"`
	CreatedAt    time.Time `db:"created_at"`
}

// DuplicateInvoiceFinder checks for other documents with the same seller GSTIN + invoice number,
// optionally matching by IRN (exact) or financial year (strong).
type DuplicateInvoiceFinder interface {
	FindDuplicates(ctx context.Context, tenantID, excludeDocID uuid.UUID,
		sellerGSTIN, invoiceNumber, financialYear, irn string) ([]DuplicateMatch, error)
}
