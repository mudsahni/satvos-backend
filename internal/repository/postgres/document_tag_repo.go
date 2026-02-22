package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"satvos/internal/domain"
	"satvos/internal/port"
)

type documentTagRepo struct {
	db *sqlx.DB
}

// NewDocumentTagRepo creates a new PostgreSQL-backed DocumentTagRepository.
func NewDocumentTagRepo(db *sqlx.DB) port.DocumentTagRepository {
	return &documentTagRepo{db: db}
}

func (r *documentTagRepo) CreateBatch(ctx context.Context, tags []domain.DocumentTag) error {
	if len(tags) == 0 {
		return nil
	}

	now := time.Now().UTC()
	valueStrings := make([]string, 0, len(tags))
	valueArgs := make([]interface{}, 0, len(tags)*6)

	for i, tag := range tags {
		tag.CreatedAt = now
		base := i * 6
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5, base+6))
		source := tag.Source
		if source == "" {
			source = "user"
		}
		valueArgs = append(valueArgs, tag.ID, tag.DocumentID, tag.TenantID, tag.Key, tag.Value, source)
	}

	query := fmt.Sprintf(
		`INSERT INTO document_tags (id, document_id, tenant_id, key, value, source) VALUES %s`,
		strings.Join(valueStrings, ", "))

	_, err := r.db.ExecContext(ctx, query, valueArgs...)
	if err != nil {
		return fmt.Errorf("documentTagRepo.CreateBatch: %w", err)
	}
	return nil
}

func (r *documentTagRepo) ListByDocument(ctx context.Context, documentID uuid.UUID) ([]domain.DocumentTag, error) {
	var tags []domain.DocumentTag
	err := r.db.SelectContext(ctx, &tags,
		"SELECT * FROM document_tags WHERE document_id = $1 ORDER BY key, value",
		documentID)
	if err != nil {
		return nil, fmt.Errorf("documentTagRepo.ListByDocument: %w", err)
	}
	return tags, nil
}

func (r *documentTagRepo) SearchByTag(ctx context.Context, tenantID uuid.UUID, key, value string, offset, limit int) ([]domain.Document, int, error) {
	var total int
	err := r.db.GetContext(ctx, &total,
		`SELECT COUNT(DISTINCT d.id) FROM documents d
		 INNER JOIN document_tags dt ON dt.document_id = d.id
		 WHERE dt.tenant_id = $1 AND dt.key = $2 AND dt.value = $3`,
		tenantID, key, value)
	if err != nil {
		return nil, 0, fmt.Errorf("documentTagRepo.SearchByTag count: %w", err)
	}

	var docs []domain.Document
	err = r.db.SelectContext(ctx, &docs,
		`SELECT DISTINCT d.* FROM documents d
		 INNER JOIN document_tags dt ON dt.document_id = d.id
		 WHERE dt.tenant_id = $1 AND dt.key = $2 AND dt.value = $3
		 ORDER BY d.created_at DESC LIMIT $4 OFFSET $5`,
		tenantID, key, value, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("documentTagRepo.SearchByTag: %w", err)
	}
	return docs, total, nil
}

func (r *documentTagRepo) DeleteByID(ctx context.Context, documentID, tagID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM document_tags WHERE id = $1 AND document_id = $2", tagID, documentID)
	if err != nil {
		return fmt.Errorf("documentTagRepo.DeleteByID: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *documentTagRepo) DeleteByDocument(ctx context.Context, documentID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM document_tags WHERE document_id = $1", documentID)
	if err != nil {
		return fmt.Errorf("documentTagRepo.DeleteByDocument: %w", err)
	}
	return nil
}

func (r *documentTagRepo) FacetsByKeys(ctx context.Context, tenantID uuid.UUID, keys []string, collectionIDs []uuid.UUID) (map[string][]port.TagFacet, error) {
	result := make(map[string][]port.TagFacet, len(keys))
	for _, key := range keys {
		var facets []port.TagFacet
		if len(collectionIDs) == 0 {
			err := r.db.SelectContext(ctx, &facets,
				`SELECT dt.value, COUNT(DISTINCT dt.document_id) as count
				 FROM document_tags dt
				 WHERE dt.tenant_id = $1 AND dt.key = $2
				 GROUP BY dt.value ORDER BY count DESC`,
				tenantID, key)
			if err != nil {
				return nil, fmt.Errorf("documentTagRepo.FacetsByKeys: %w", err)
			}
		} else {
			query, args, err := sqlx.In(
				`SELECT dt.value, COUNT(DISTINCT dt.document_id) as count
				 FROM document_tags dt
				 INNER JOIN documents d ON d.id = dt.document_id
				 WHERE dt.tenant_id = ? AND dt.key = ? AND d.collection_id IN (?)
				 GROUP BY dt.value ORDER BY count DESC`,
				tenantID, key, collectionIDs)
			if err != nil {
				return nil, fmt.Errorf("documentTagRepo.FacetsByKeys expand IN: %w", err)
			}
			query = r.db.Rebind(query)
			if err := r.db.SelectContext(ctx, &facets, query, args...); err != nil {
				return nil, fmt.Errorf("documentTagRepo.FacetsByKeys: %w", err)
			}
		}
		result[key] = facets
	}
	return result, nil
}

func (r *documentTagRepo) DeleteByDocumentAndSource(ctx context.Context, documentID uuid.UUID, source string) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM document_tags WHERE document_id = $1 AND source = $2", documentID, source)
	if err != nil {
		return fmt.Errorf("documentTagRepo.DeleteByDocumentAndSource: %w", err)
	}
	return nil
}
