package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"satvos/internal/domain"
	"satvos/internal/port"
)

type documentRepo struct {
	db *sqlx.DB
}

// NewDocumentRepo creates a new PostgreSQL-backed DocumentRepository.
func NewDocumentRepo(db *sqlx.DB) port.DocumentRepository {
	return &documentRepo{db: db}
}

func (r *documentRepo) Create(ctx context.Context, doc *domain.Document) error {
	now := time.Now().UTC()
	doc.CreatedAt = now
	doc.UpdatedAt = now

	query := `INSERT INTO documents (
		id, tenant_id, collection_id, file_id, name, document_type,
		parser_model, parser_prompt, structured_data, confidence_scores,
		parsing_status, parsing_error, parsed_at,
		review_status, reviewed_by, reviewed_at, reviewer_notes,
		validation_status, validation_results, reconciliation_status,
		parse_mode, field_provenance,
		secondary_parser_model, parse_attempts, retry_after,
		created_by, created_at, updated_at
	) VALUES (
		$1, $2, $3, $4, $5, $6,
		$7, $8, $9, $10,
		$11, $12, $13,
		$14, $15, $16, $17,
		$18, $19, $20,
		$21, $22,
		$23, $24, $25,
		$26, $27, $28
	)`

	_, err := r.db.ExecContext(ctx, query,
		doc.ID, doc.TenantID, doc.CollectionID, doc.FileID, doc.Name, doc.DocumentType,
		doc.ParserModel, doc.ParserPrompt, doc.StructuredData, doc.ConfidenceScores,
		doc.ParsingStatus, doc.ParsingError, doc.ParsedAt,
		doc.ReviewStatus, doc.ReviewedBy, doc.ReviewedAt, doc.ReviewerNotes,
		doc.ValidationStatus, doc.ValidationResults, doc.ReconciliationStatus,
		doc.ParseMode, doc.FieldProvenance,
		doc.SecondaryParserModel, doc.ParseAttempts, doc.RetryAfter,
		doc.CreatedBy, doc.CreatedAt, doc.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") && strings.Contains(err.Error(), "file_id") {
			return domain.ErrDocumentAlreadyExists
		}
		return fmt.Errorf("documentRepo.Create: %w", err)
	}
	return nil
}

func (r *documentRepo) GetByID(ctx context.Context, tenantID, docID uuid.UUID) (*domain.Document, error) {
	var doc domain.Document
	err := r.db.GetContext(ctx, &doc,
		"SELECT * FROM documents WHERE id = $1 AND tenant_id = $2", docID, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrDocumentNotFound
		}
		return nil, fmt.Errorf("documentRepo.GetByID: %w", err)
	}
	return &doc, nil
}

func (r *documentRepo) GetByFileID(ctx context.Context, tenantID, fileID uuid.UUID) (*domain.Document, error) {
	var doc domain.Document
	err := r.db.GetContext(ctx, &doc,
		"SELECT * FROM documents WHERE file_id = $1 AND tenant_id = $2", fileID, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrDocumentNotFound
		}
		return nil, fmt.Errorf("documentRepo.GetByFileID: %w", err)
	}
	return &doc, nil
}

// appendListFilters appends optional filter clauses to a base WHERE query.
// tableAlias should be empty for unaliased queries or "d." for JOINed queries.
func appendListFilters(baseWhere string, baseArgs []interface{}, filters *port.DocumentListFilters, tableAlias string) (query string, args []interface{}) {
	query, args = baseWhere, baseArgs
	if filters == nil {
		return
	}

	if filters.AssignedTo != nil {
		args = append(args, *filters.AssignedTo)
		query += fmt.Sprintf(" AND %sassigned_to = $%d", tableAlias, len(args))
	}
	if filters.ParsingStatus != nil {
		args = append(args, string(*filters.ParsingStatus))
		query += fmt.Sprintf(" AND %sparsing_status = $%d", tableAlias, len(args))
	}
	if filters.ReviewStatus != nil {
		args = append(args, string(*filters.ReviewStatus))
		query += fmt.Sprintf(" AND %sreview_status = $%d", tableAlias, len(args))
	}
	if filters.ValidationStatus != nil {
		args = append(args, string(*filters.ValidationStatus))
		query += fmt.Sprintf(" AND %svalidation_status = $%d", tableAlias, len(args))
	}
	if filters.ReconciliationStatus != nil {
		args = append(args, string(*filters.ReconciliationStatus))
		query += fmt.Sprintf(" AND %sreconciliation_status = $%d", tableAlias, len(args))
	}
	if filters.SellerName != nil {
		args = append(args, *filters.SellerName)
		query += fmt.Sprintf(
			" AND EXISTS (SELECT 1 FROM document_tags dt WHERE dt.document_id = %sid AND dt.key = 'seller_name' AND dt.value = $%d)",
			tableAlias, len(args))
	}
	if filters.BuyerName != nil {
		args = append(args, *filters.BuyerName)
		query += fmt.Sprintf(
			" AND EXISTS (SELECT 1 FROM document_tags dt WHERE dt.document_id = %sid AND dt.key = 'buyer_name' AND dt.value = $%d)",
			tableAlias, len(args))
	}
	return
}

func (r *documentRepo) ListByCollection(ctx context.Context, tenantID, collectionID uuid.UUID, filters *port.DocumentListFilters, offset, limit int) ([]domain.Document, int, error) {
	baseWhere := "WHERE tenant_id = $1 AND collection_id = $2"
	args := make([]interface{}, 0, 11) // 2 base + up to 7 filters + limit + offset
	args = append(args, tenantID, collectionID)
	baseWhere, args = appendListFilters(baseWhere, args, filters, "")

	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM documents "+baseWhere, args...); err != nil {
		return nil, 0, fmt.Errorf("documentRepo.ListByCollection count: %w", err)
	}

	selectQuery := fmt.Sprintf("SELECT * FROM documents %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		baseWhere, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	var docs []domain.Document
	if err := r.db.SelectContext(ctx, &docs, selectQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("documentRepo.ListByCollection: %w", err)
	}
	return docs, total, nil
}

func (r *documentRepo) ListByTenant(ctx context.Context, tenantID uuid.UUID, filters *port.DocumentListFilters, offset, limit int) ([]domain.Document, int, error) {
	baseWhere := "WHERE tenant_id = $1"
	args := make([]interface{}, 0, 10) // 1 base + up to 7 filters + limit + offset
	args = append(args, tenantID)
	baseWhere, args = appendListFilters(baseWhere, args, filters, "")

	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM documents "+baseWhere, args...); err != nil {
		return nil, 0, fmt.Errorf("documentRepo.ListByTenant count: %w", err)
	}

	selectQuery := fmt.Sprintf("SELECT * FROM documents %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		baseWhere, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	var docs []domain.Document
	if err := r.db.SelectContext(ctx, &docs, selectQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("documentRepo.ListByTenant: %w", err)
	}
	return docs, total, nil
}

func (r *documentRepo) ListByUserCollections(ctx context.Context, tenantID, userID uuid.UUID, filters *port.DocumentListFilters, offset, limit int) ([]domain.Document, int, error) {
	baseWhere := `WHERE d.tenant_id = $1 AND cp.user_id = $2`
	args := make([]interface{}, 0, 11) // 2 base + up to 7 filters + limit + offset
	args = append(args, tenantID, userID)
	baseWhere, args = appendListFilters(baseWhere, args, filters, "d.")

	fromJoin := `FROM documents d
		 INNER JOIN collection_permissions cp ON cp.collection_id = d.collection_id`

	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) "+fromJoin+" "+baseWhere, args...); err != nil {
		return nil, 0, fmt.Errorf("documentRepo.ListByUserCollections count: %w", err)
	}

	selectQuery := fmt.Sprintf("SELECT d.* %s %s ORDER BY d.created_at DESC LIMIT $%d OFFSET $%d",
		fromJoin, baseWhere, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	var docs []domain.Document
	if err := r.db.SelectContext(ctx, &docs, selectQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("documentRepo.ListByUserCollections: %w", err)
	}
	return docs, total, nil
}

func (r *documentRepo) ListByServiceAccountCollections(ctx context.Context, tenantID, saID uuid.UUID, filters *port.DocumentListFilters, offset, limit int) ([]domain.Document, int, error) {
	baseWhere := `WHERE d.tenant_id = $1`
	args := make([]interface{}, 0, 11) // 2 base + up to 7 filters + limit + offset
	args = append(args, tenantID, saID)
	baseWhere, args = appendListFilters(baseWhere, args, filters, "d.")

	fromJoin := `FROM documents d
		 INNER JOIN service_account_permissions sap
		     ON sap.collection_id = d.collection_id AND sap.service_account_id = $2 AND sap.tenant_id = $1`

	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) "+fromJoin+" "+baseWhere, args...); err != nil {
		return nil, 0, fmt.Errorf("documentRepo.ListByServiceAccountCollections count: %w", err)
	}

	selectQuery := fmt.Sprintf("SELECT d.* %s %s ORDER BY d.created_at DESC LIMIT $%d OFFSET $%d",
		fromJoin, baseWhere, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	var docs []domain.Document
	if err := r.db.SelectContext(ctx, &docs, selectQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("documentRepo.ListByServiceAccountCollections: %w", err)
	}
	return docs, total, nil
}

func (r *documentRepo) UpdateStructuredData(ctx context.Context, doc *domain.Document) error {
	doc.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx,
		`UPDATE documents SET
			structured_data = $1, confidence_scores = $2,
			parsing_status = $3, parsing_error = $4, parsed_at = $5,
			parser_model = $6, parser_prompt = $7,
			field_provenance = $8,
			secondary_parser_model = $9, parse_attempts = $10,
			retry_after = $11,
			updated_at = $12
		 WHERE id = $13 AND tenant_id = $14`,
		doc.StructuredData, doc.ConfidenceScores,
		doc.ParsingStatus, doc.ParsingError, doc.ParsedAt,
		doc.ParserModel, doc.ParserPrompt,
		doc.FieldProvenance,
		doc.SecondaryParserModel, doc.ParseAttempts,
		doc.RetryAfter,
		doc.UpdatedAt,
		doc.ID, doc.TenantID)
	if err != nil {
		return fmt.Errorf("documentRepo.UpdateStructuredData: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrDocumentNotFound
	}
	return nil
}

func (r *documentRepo) UpdateReviewStatus(ctx context.Context, doc *domain.Document) error {
	doc.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx,
		`UPDATE documents SET
			review_status = $1, reviewed_by = $2, reviewed_at = $3,
			reviewer_notes = $4, sync_review_status = $5, updated_at = $6
		 WHERE id = $7 AND tenant_id = $8`,
		doc.ReviewStatus, doc.ReviewedBy, doc.ReviewedAt,
		doc.ReviewerNotes, string(doc.SyncReviewStatus), doc.UpdatedAt,
		doc.ID, doc.TenantID)
	if err != nil {
		return fmt.Errorf("documentRepo.UpdateReviewStatus: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrDocumentNotFound
	}
	return nil
}

func (r *documentRepo) UpdateAssignment(ctx context.Context, doc *domain.Document) error {
	doc.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx,
		`UPDATE documents SET
			assigned_to = $1, assigned_at = $2, assigned_by = $3, updated_at = $4
		 WHERE id = $5 AND tenant_id = $6`,
		doc.AssignedTo, doc.AssignedAt, doc.AssignedBy, doc.UpdatedAt,
		doc.ID, doc.TenantID)
	if err != nil {
		return fmt.Errorf("documentRepo.UpdateAssignment: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrDocumentNotFound
	}
	return nil
}

func (r *documentRepo) ListReviewQueue(ctx context.Context, tenantID, userID uuid.UUID, offset, limit int) ([]domain.Document, int, error) {
	baseWhere := "WHERE tenant_id = $1 AND assigned_to = $2 AND parsing_status = 'completed' AND review_status = 'pending'"

	var total int
	err := r.db.GetContext(ctx, &total,
		"SELECT COUNT(*) FROM documents "+baseWhere, tenantID, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("documentRepo.ListReviewQueue count: %w", err)
	}

	var docs []domain.Document
	err = r.db.SelectContext(ctx, &docs,
		"SELECT * FROM documents "+baseWhere+" ORDER BY assigned_at ASC LIMIT $3 OFFSET $4",
		tenantID, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("documentRepo.ListReviewQueue: %w", err)
	}
	return docs, total, nil
}

func (r *documentRepo) UpdateValidationResults(ctx context.Context, doc *domain.Document) error {
	doc.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx,
		`UPDATE documents SET validation_results = $1, validation_status = $2, reconciliation_status = $3, updated_at = $4
		 WHERE id = $5 AND tenant_id = $6`,
		doc.ValidationResults, doc.ValidationStatus, doc.ReconciliationStatus, doc.UpdatedAt, doc.ID, doc.TenantID)
	if err != nil {
		return fmt.Errorf("documentRepo.UpdateValidationResults: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrDocumentNotFound
	}
	return nil
}

func (r *documentRepo) ClaimQueued(ctx context.Context, limit int) ([]domain.Document, error) {
	var docs []domain.Document
	err := r.db.SelectContext(ctx, &docs,
		`UPDATE documents
		 SET parsing_status = 'processing', updated_at = NOW()
		 WHERE id IN (
		     SELECT id FROM documents
		     WHERE parsing_status = 'queued' AND retry_after <= NOW()
		     ORDER BY retry_after ASC
		     LIMIT $1
		     FOR UPDATE SKIP LOCKED
		 )
		 RETURNING *`,
		limit)
	if err != nil {
		return nil, fmt.Errorf("documentRepo.ClaimQueued: %w", err)
	}
	return docs, nil
}

func (r *documentRepo) ResetStaleProcessing(ctx context.Context, staleBefore time.Time) (int, error) {
	result, err := r.db.ExecContext(ctx,
		`UPDATE documents
		 SET parsing_status = 'queued', retry_after = NOW(), updated_at = NOW()
		 WHERE parsing_status = 'processing' AND updated_at < $1`,
		staleBefore)
	if err != nil {
		return 0, fmt.Errorf("documentRepo.ResetStaleProcessing: %w", err)
	}
	rows, _ := result.RowsAffected()
	return int(rows), nil
}

func (r *documentRepo) Delete(ctx context.Context, tenantID, docID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM documents WHERE id = $1 AND tenant_id = $2",
		docID, tenantID)
	if err != nil {
		return fmt.Errorf("documentRepo.Delete: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrDocumentNotFound
	}
	return nil
}

func (r *documentRepo) ListSyncReviewQueue(ctx context.Context, tenantID uuid.UUID, filters *port.SyncReviewFilters, offset, limit int) ([]domain.Document, int, error) {
	baseWhere := "WHERE tenant_id = $1 AND sync_review_status = $2"
	status := domain.SyncReviewPending
	if filters != nil && filters.Status != "" {
		status = filters.Status
	}
	args := []interface{}{tenantID, string(status)}

	if filters != nil && filters.CollectionID != nil {
		args = append(args, *filters.CollectionID)
		baseWhere += fmt.Sprintf(" AND collection_id = $%d", len(args))
	}

	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM documents "+baseWhere, args...); err != nil {
		return nil, 0, fmt.Errorf("documentRepo.ListSyncReviewQueue count: %w", err)
	}

	selectQuery := fmt.Sprintf("SELECT * FROM documents %s ORDER BY updated_at DESC LIMIT $%d OFFSET $%d",
		baseWhere, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	var docs []domain.Document
	if err := r.db.SelectContext(ctx, &docs, selectQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("documentRepo.ListSyncReviewQueue: %w", err)
	}
	return docs, total, nil
}

func (r *documentRepo) BatchUpdateSyncReviewStatus(ctx context.Context, tenantID uuid.UUID, docIDs []uuid.UUID, status domain.SyncReviewStatus, approvedBy *uuid.UUID) error {
	now := time.Now().UTC()

	var approvedAt *time.Time
	if status == domain.SyncReviewApproved {
		approvedAt = &now
	}

	result, err := r.db.ExecContext(ctx,
		`UPDATE documents SET
			sync_review_status = $1, sync_approved_by = $2, sync_approved_at = $3, updated_at = $4
		 WHERE id = ANY($5) AND tenant_id = $6`,
		string(status), approvedBy, approvedAt, now, pq.Array(docIDs), tenantID)
	if err != nil {
		return fmt.Errorf("documentRepo.BatchUpdateSyncReviewStatus: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrDocumentNotFound
	}
	return nil
}

func (r *documentRepo) UpdateVoucherOverrides(ctx context.Context, tenantID, docID uuid.UUID, overrides json.RawMessage) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx,
		`UPDATE documents SET voucher_overrides = $1, updated_at = $2
		 WHERE id = $3 AND tenant_id = $4`,
		overrides, now, docID, tenantID)
	if err != nil {
		return fmt.Errorf("documentRepo.UpdateVoucherOverrides: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrDocumentNotFound
	}
	return nil
}
