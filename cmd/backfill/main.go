// Command backfill populates the document_summaries table for existing documents
// that have been successfully parsed but may not yet have summary rows.
// Usage: go run ./cmd/backfill
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"satvos/internal/config"
	"satvos/internal/domain"
	"satvos/internal/repository/postgres"
	"satvos/internal/validator/invoice"
)

const batchSize = 100

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	db, err := postgres.NewDB(&cfg.DB)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer func() { _ = db.Close() }()

	summaryRepo := postgres.NewDocumentSummaryRepo(db)

	ctx := context.Background()
	offset := 0
	total := 0

	for {
		var docs []domain.Document
		err := db.SelectContext(ctx, &docs,
			`SELECT id, tenant_id, collection_id, structured_data,
				parsing_status, review_status, validation_status, reconciliation_status
			 FROM documents
			 WHERE parsing_status = 'completed' AND structured_data IS NOT NULL
			 ORDER BY created_at
			 LIMIT $1 OFFSET $2`, batchSize, offset)
		if err != nil {
			return fmt.Errorf("querying documents at offset %d: %w", offset, err)
		}
		if len(docs) == 0 {
			break
		}

		for i := range docs {
			doc := &docs[i]

			var inv invoice.GSTInvoice
			if err := json.Unmarshal(doc.StructuredData, &inv); err != nil {
				log.Printf("WARN: skipping document %s: unmarshal structured_data: %v", doc.ID, err)
				continue
			}

			summary := &domain.DocumentSummary{
				DocumentID:           doc.ID,
				TenantID:             doc.TenantID,
				CollectionID:         doc.CollectionID,
				InvoiceNumber:        inv.Invoice.InvoiceNumber,
				InvoiceType:          inv.Invoice.InvoiceType,
				Currency:             inv.Invoice.Currency,
				PlaceOfSupply:        inv.Invoice.PlaceOfSupply,
				ReverseCharge:        inv.Invoice.ReverseCharge,
				HasIRN:               inv.Invoice.IRN != "",
				SellerName:           inv.Seller.Name,
				SellerGSTIN:          inv.Seller.GSTIN,
				SellerState:          inv.Seller.State,
				SellerStateCode:      inv.Seller.StateCode,
				BuyerName:            inv.Buyer.Name,
				BuyerGSTIN:           inv.Buyer.GSTIN,
				BuyerState:           inv.Buyer.State,
				BuyerStateCode:       inv.Buyer.StateCode,
				Subtotal:             inv.Totals.Subtotal,
				TotalDiscount:        inv.Totals.TotalDiscount,
				TaxableAmount:        inv.Totals.TaxableAmount,
				CGST:                 inv.Totals.CGST,
				SGST:                 inv.Totals.SGST,
				IGST:                 inv.Totals.IGST,
				Cess:                 inv.Totals.Cess,
				TotalAmount:          inv.Totals.Total,
				LineItemCount:        len(inv.LineItems),
				ParsingStatus:        doc.ParsingStatus,
				ReviewStatus:         doc.ReviewStatus,
				ValidationStatus:     doc.ValidationStatus,
				ReconciliationStatus: doc.ReconciliationStatus,
			}

			// Parse invoice date and due date
			summary.InvoiceDate = parseInvoiceDate(inv.Invoice.InvoiceDate)
			summary.DueDate = parseInvoiceDate(inv.Invoice.DueDate)

			// Collect distinct HSN codes
			hsnSet := make(map[string]struct{})
			for j := range inv.LineItems {
				if inv.LineItems[j].HSNSACCode != "" {
					hsnSet[inv.LineItems[j].HSNSACCode] = struct{}{}
				}
			}
			hsns := make([]string, 0, len(hsnSet))
			for code := range hsnSet {
				hsns = append(hsns, code)
			}
			summary.DistinctHSNCodes = hsns

			if err := summaryRepo.Upsert(ctx, summary); err != nil {
				log.Printf("WARN: failed to upsert summary for document %s: %v", doc.ID, err)
				continue
			}

			// Backfill denormalized line items for HSN queries
			if len(inv.LineItems) > 0 {
				items := make([]domain.DocumentLineItem, len(inv.LineItems))
				for j := range inv.LineItems {
					li := &inv.LineItems[j]
					items[j] = domain.DocumentLineItem{
						DocumentID:    doc.ID,
						TenantID:      doc.TenantID,
						ItemIndex:     j,
						HSNSACCode:    li.HSNSACCode,
						Description:   li.Description,
						Quantity:      li.Quantity,
						UnitPrice:     li.UnitPrice,
						Discount:      li.Discount,
						TaxableAmount: li.TaxableAmount,
						CGSTRate:      li.CGSTRate,
						CGSTAmount:    li.CGSTAmount,
						SGSTRate:      li.SGSTRate,
						SGSTAmount:    li.SGSTAmount,
						IGSTRate:      li.IGSTRate,
						IGSTAmount:    li.IGSTAmount,
						Total:         li.Total,
					}
				}
				if err := summaryRepo.ReplaceLineItems(ctx, doc.ID, doc.TenantID, items); err != nil {
					log.Printf("WARN: failed to backfill line items for document %s: %v", doc.ID, err)
				}
			}

			total++
		}

		if total > 0 && total%batchSize == 0 {
			log.Printf("Progress: %d documents processed", total)
		}

		offset += len(docs)
	}

	log.Printf("Backfill complete: %d document summaries upserted", total)
	return nil
}

// parseInvoiceDate attempts multiple date formats from LLM output.
func parseInvoiceDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	formats := []string{
		"2006-01-02",
		"02/01/2006",
		"02-01-2006",
		"01/02/2006",
		"January 2, 2006",
		"Jan 2, 2006",
		"2 January 2006",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return &t
		}
	}
	return nil
}
