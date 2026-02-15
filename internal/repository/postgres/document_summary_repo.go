package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"satvos/internal/domain"
)

type documentSummaryRepo struct {
	db *sqlx.DB
}

// NewDocumentSummaryRepo creates a new PostgreSQL-backed DocumentSummaryRepository.
func NewDocumentSummaryRepo(db *sqlx.DB) *documentSummaryRepo {
	return &documentSummaryRepo{db: db}
}

func (r *documentSummaryRepo) Upsert(ctx context.Context, summary *domain.DocumentSummary) error {
	query := `
		INSERT INTO document_summaries (
			document_id, tenant_id, collection_id,
			invoice_number, invoice_date, due_date, invoice_type, currency,
			place_of_supply, reverse_charge, has_irn,
			seller_name, seller_gstin, seller_state, seller_state_code,
			buyer_name, buyer_gstin, buyer_state, buyer_state_code,
			subtotal, total_discount, taxable_amount, cgst, sgst, igst, cess, total_amount,
			line_item_count, distinct_hsn_codes,
			parsing_status, review_status, validation_status, reconciliation_status,
			created_at, updated_at
		) VALUES (
			:document_id, :tenant_id, :collection_id,
			:invoice_number, :invoice_date, :due_date, :invoice_type, :currency,
			:place_of_supply, :reverse_charge, :has_irn,
			:seller_name, :seller_gstin, :seller_state, :seller_state_code,
			:buyer_name, :buyer_gstin, :buyer_state, :buyer_state_code,
			:subtotal, :total_discount, :taxable_amount, :cgst, :sgst, :igst, :cess, :total_amount,
			:line_item_count, :distinct_hsn_codes,
			:parsing_status, :review_status, :validation_status, :reconciliation_status,
			NOW(), NOW()
		)
		ON CONFLICT (document_id) DO UPDATE SET
			invoice_number = EXCLUDED.invoice_number,
			invoice_date = EXCLUDED.invoice_date,
			due_date = EXCLUDED.due_date,
			invoice_type = EXCLUDED.invoice_type,
			currency = EXCLUDED.currency,
			place_of_supply = EXCLUDED.place_of_supply,
			reverse_charge = EXCLUDED.reverse_charge,
			has_irn = EXCLUDED.has_irn,
			seller_name = EXCLUDED.seller_name,
			seller_gstin = EXCLUDED.seller_gstin,
			seller_state = EXCLUDED.seller_state,
			seller_state_code = EXCLUDED.seller_state_code,
			buyer_name = EXCLUDED.buyer_name,
			buyer_gstin = EXCLUDED.buyer_gstin,
			buyer_state = EXCLUDED.buyer_state,
			buyer_state_code = EXCLUDED.buyer_state_code,
			subtotal = EXCLUDED.subtotal,
			total_discount = EXCLUDED.total_discount,
			taxable_amount = EXCLUDED.taxable_amount,
			cgst = EXCLUDED.cgst,
			sgst = EXCLUDED.sgst,
			igst = EXCLUDED.igst,
			cess = EXCLUDED.cess,
			total_amount = EXCLUDED.total_amount,
			line_item_count = EXCLUDED.line_item_count,
			distinct_hsn_codes = EXCLUDED.distinct_hsn_codes,
			parsing_status = EXCLUDED.parsing_status,
			review_status = EXCLUDED.review_status,
			validation_status = EXCLUDED.validation_status,
			reconciliation_status = EXCLUDED.reconciliation_status,
			updated_at = NOW()`

	_, err := r.db.NamedExecContext(ctx, query, summary)
	if err != nil {
		return fmt.Errorf("upserting document summary: %w", err)
	}
	return nil
}

func (r *documentSummaryRepo) ReplaceLineItems(ctx context.Context, documentID, tenantID uuid.UUID, items []domain.DocumentLineItem) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning line items transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "DELETE FROM document_line_items WHERE document_id = $1", documentID); err != nil {
		return fmt.Errorf("deleting old line items: %w", err)
	}

	if len(items) > 0 {
		query := `INSERT INTO document_line_items (
			document_id, tenant_id, item_index, hsn_sac_code, description,
			quantity, unit_price, discount, taxable_amount,
			cgst_rate, cgst_amount, sgst_rate, sgst_amount,
			igst_rate, igst_amount, total
		) VALUES (
			:document_id, :tenant_id, :item_index, :hsn_sac_code, :description,
			:quantity, :unit_price, :discount, :taxable_amount,
			:cgst_rate, :cgst_amount, :sgst_rate, :sgst_amount,
			:igst_rate, :igst_amount, :total
		)`
		if _, err := tx.NamedExecContext(ctx, query, items); err != nil {
			return fmt.Errorf("inserting line items: %w", err)
		}
	}

	return tx.Commit()
}

func (r *documentSummaryRepo) UpdateStatuses(ctx context.Context, documentID uuid.UUID, statuses domain.SummaryStatusUpdate) error {
	query := `
		UPDATE document_summaries SET
			parsing_status = $2,
			review_status = $3,
			validation_status = $4,
			reconciliation_status = $5,
			updated_at = NOW()
		WHERE document_id = $1`

	_, err := r.db.ExecContext(ctx, query, documentID,
		statuses.ParsingStatus, statuses.ReviewStatus,
		statuses.ValidationStatus, statuses.ReconciliationStatus)
	if err != nil {
		return fmt.Errorf("updating document summary statuses: %w", err)
	}
	return nil
}
