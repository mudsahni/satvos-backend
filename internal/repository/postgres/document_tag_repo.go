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
	for _, k := range keys {
		result[k] = []port.TagFacet{}
	}

	if len(keys) == 0 {
		return result, nil
	}

	// nil = all collections (admin/manager/member); empty = no access (return empty facets)
	if collectionIDs != nil && len(collectionIDs) == 0 {
		return result, nil
	}

	const facetLimit = 100

	var rawQuery string
	var inArgs []interface{}
	var err error

	if collectionIDs == nil {
		rawQuery, inArgs, err = sqlx.In(
			`SELECT key, value, count FROM (
				SELECT dt.key, dt.value, COUNT(DISTINCT dt.document_id) AS count,
					ROW_NUMBER() OVER (PARTITION BY dt.key ORDER BY COUNT(DISTINCT dt.document_id) DESC) AS rn
				FROM document_tags dt
				WHERE dt.tenant_id = ? AND dt.key IN (?)
				GROUP BY dt.key, dt.value
			) sub WHERE rn <= ?`,
			tenantID, keys, facetLimit)
	} else {
		rawQuery, inArgs, err = sqlx.In(
			`SELECT key, value, count FROM (
				SELECT dt.key, dt.value, COUNT(DISTINCT dt.document_id) AS count,
					ROW_NUMBER() OVER (PARTITION BY dt.key ORDER BY COUNT(DISTINCT dt.document_id) DESC) AS rn
				FROM document_tags dt
				INNER JOIN documents d ON d.id = dt.document_id
				WHERE dt.tenant_id = ? AND dt.key IN (?) AND d.collection_id IN (?)
				GROUP BY dt.key, dt.value
			) sub WHERE rn <= ?`,
			tenantID, keys, collectionIDs, facetLimit)
	}
	if err != nil {
		return nil, fmt.Errorf("documentTagRepo.FacetsByKeys expand IN: %w", err)
	}
	rawQuery = r.db.Rebind(rawQuery)

	type facetRow struct {
		Key   string `db:"key"`
		Value string `db:"value"`
		Count int    `db:"count"`
	}
	var rows []facetRow
	if err := r.db.SelectContext(ctx, &rows, rawQuery, inArgs...); err != nil {
		return nil, fmt.Errorf("documentTagRepo.FacetsByKeys: %w", err)
	}

	for i := range rows {
		result[rows[i].Key] = append(result[rows[i].Key], port.TagFacet{Value: rows[i].Value, Count: rows[i].Count})
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
