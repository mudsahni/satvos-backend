package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"satvos/internal/domain"
	"satvos/internal/port"
)

type collectionPermissionRepo struct {
	db *sqlx.DB
}

// NewCollectionPermissionRepo creates a new PostgreSQL-backed CollectionPermissionRepository.
func NewCollectionPermissionRepo(db *sqlx.DB) port.CollectionPermissionRepository {
	return &collectionPermissionRepo{db: db}
}

func (r *collectionPermissionRepo) Upsert(ctx context.Context, perm *domain.CollectionPermissionEntry) error {
	if perm.ID == uuid.Nil {
		perm.ID = uuid.New()
	}
	perm.CreatedAt = time.Now().UTC()

	query := `INSERT INTO collection_permissions (id, collection_id, tenant_id, user_id, permission, granted_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (collection_id, user_id)
		DO UPDATE SET permission = EXCLUDED.permission, granted_by = EXCLUDED.granted_by`

	_, err := r.db.ExecContext(ctx, query,
		perm.ID, perm.CollectionID, perm.TenantID, perm.UserID,
		perm.Permission, perm.GrantedBy, perm.CreatedAt)
	if err != nil {
		return fmt.Errorf("collectionPermissionRepo.Upsert: %w", err)
	}
	return nil
}

func (r *collectionPermissionRepo) GetByCollectionAndUser(ctx context.Context, collectionID, userID uuid.UUID) (*domain.CollectionPermissionEntry, error) {
	var perm domain.CollectionPermissionEntry
	err := r.db.GetContext(ctx, &perm,
		"SELECT * FROM collection_permissions WHERE collection_id = $1 AND user_id = $2",
		collectionID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrCollectionPermDenied
		}
		return nil, fmt.Errorf("collectionPermissionRepo.GetByCollectionAndUser: %w", err)
	}
	return &perm, nil
}

func (r *collectionPermissionRepo) GetByUserForCollections(ctx context.Context, userID uuid.UUID, collectionIDs []uuid.UUID) (map[uuid.UUID]domain.CollectionPermission, error) {
	if len(collectionIDs) == 0 {
		return make(map[uuid.UUID]domain.CollectionPermission), nil
	}

	query, args, err := sqlx.In(
		"SELECT collection_id, permission FROM collection_permissions WHERE user_id = ? AND collection_id IN (?)",
		userID, collectionIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("collectionPermissionRepo.GetByUserForCollections: building query: %w", err)
	}
	query = r.db.Rebind(query)

	var rows []struct {
		CollectionID uuid.UUID                   `db:"collection_id"`
		Permission   domain.CollectionPermission `db:"permission"`
	}
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("collectionPermissionRepo.GetByUserForCollections: %w", err)
	}

	result := make(map[uuid.UUID]domain.CollectionPermission, len(rows))
	for i := range rows {
		result[rows[i].CollectionID] = rows[i].Permission
	}
	return result, nil
}

func (r *collectionPermissionRepo) ListByCollection(ctx context.Context, collectionID uuid.UUID, offset, limit int) ([]domain.CollectionPermissionEntry, int, error) {
	var total int
	err := r.db.GetContext(ctx, &total,
		"SELECT COUNT(*) FROM collection_permissions WHERE collection_id = $1", collectionID)
	if err != nil {
		return nil, 0, fmt.Errorf("collectionPermissionRepo.ListByCollection count: %w", err)
	}

	var perms []domain.CollectionPermissionEntry
	err = r.db.SelectContext(ctx, &perms,
		`SELECT * FROM collection_permissions
		 WHERE collection_id = $1
		 ORDER BY created_at ASC LIMIT $2 OFFSET $3`,
		collectionID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("collectionPermissionRepo.ListByCollection: %w", err)
	}
	return perms, total, nil
}

func (r *collectionPermissionRepo) ListCollectionIDs(ctx context.Context, tenantID, userID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.db.SelectContext(ctx, &ids,
		"SELECT DISTINCT collection_id FROM collection_permissions WHERE tenant_id = $1 AND user_id = $2",
		tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("collectionPermissionRepo.ListCollectionIDs: %w", err)
	}
	return ids, nil
}

func (r *collectionPermissionRepo) Delete(ctx context.Context, collectionID, userID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM collection_permissions WHERE collection_id = $1 AND user_id = $2",
		collectionID, userID)
	if err != nil {
		return fmt.Errorf("collectionPermissionRepo.Delete: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}
