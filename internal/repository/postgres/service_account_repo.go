package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"satvos/internal/domain"
	"satvos/internal/port"
)

type serviceAccountRepo struct {
	db *sqlx.DB
}

// NewServiceAccountRepo creates a new ServiceAccountRepository implementation.
func NewServiceAccountRepo(db *sqlx.DB) port.ServiceAccountRepository {
	return &serviceAccountRepo{db: db}
}

func (r *serviceAccountRepo) Create(ctx context.Context, sa *domain.ServiceAccount) error {
	sa.ID = uuid.New()
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO service_accounts (id, tenant_id, name, description, api_key_prefix, api_key_hash,
			is_active, created_by, expires_at, created_at, updated_at)
		VALUES (:id, :tenant_id, :name, :description, :api_key_prefix, :api_key_hash,
			:is_active, :created_by, :expires_at, NOW(), NOW())`,
		sa)
	if err != nil {
		return fmt.Errorf("inserting service account: %w", err)
	}
	return nil
}

func (r *serviceAccountRepo) GetByID(ctx context.Context, tenantID, saID uuid.UUID) (*domain.ServiceAccount, error) {
	var sa domain.ServiceAccount
	err := r.db.GetContext(ctx, &sa,
		"SELECT * FROM service_accounts WHERE id = $1 AND tenant_id = $2", saID, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrServiceAccountNotFound
		}
		return nil, fmt.Errorf("getting service account: %w", err)
	}
	return &sa, nil
}

func (r *serviceAccountRepo) GetByAPIKeyPrefix(ctx context.Context, prefix string) ([]domain.ServiceAccount, error) {
	var accounts []domain.ServiceAccount
	err := r.db.SelectContext(ctx, &accounts,
		"SELECT * FROM service_accounts WHERE api_key_prefix = $1 AND is_active = TRUE", prefix)
	if err != nil {
		return nil, fmt.Errorf("getting service accounts by prefix: %w", err)
	}
	return accounts, nil
}

func (r *serviceAccountRepo) ListByTenant(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.ServiceAccount, int, error) {
	var total int
	err := r.db.GetContext(ctx, &total,
		"SELECT COUNT(*) FROM service_accounts WHERE tenant_id = $1", tenantID)
	if err != nil {
		return nil, 0, fmt.Errorf("counting service accounts: %w", err)
	}

	var accounts []domain.ServiceAccount
	err = r.db.SelectContext(ctx, &accounts,
		"SELECT * FROM service_accounts WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3",
		tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing service accounts: %w", err)
	}
	return accounts, total, nil
}

func (r *serviceAccountRepo) Update(ctx context.Context, sa *domain.ServiceAccount) error {
	_, err := r.db.NamedExecContext(ctx, `
		UPDATE service_accounts SET
			name = :name, description = :description, api_key_prefix = :api_key_prefix,
			api_key_hash = :api_key_hash, is_active = :is_active, expires_at = :expires_at,
			updated_at = NOW()
		WHERE id = :id AND tenant_id = :tenant_id`,
		sa)
	if err != nil {
		return fmt.Errorf("updating service account: %w", err)
	}
	return nil
}

func (r *serviceAccountRepo) Revoke(ctx context.Context, tenantID, saID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx,
		"UPDATE service_accounts SET is_active = FALSE, updated_at = NOW() WHERE id = $1 AND tenant_id = $2",
		saID, tenantID)
	if err != nil {
		return fmt.Errorf("revoking service account: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrServiceAccountNotFound
	}
	return nil
}

func (r *serviceAccountRepo) RotateKey(ctx context.Context, tenantID, saID uuid.UUID, newPrefix, newHash string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE service_accounts SET api_key_prefix = $1, api_key_hash = $2, updated_at = NOW()
		 WHERE id = $3 AND tenant_id = $4 AND is_active = TRUE`,
		newPrefix, newHash, saID, tenantID)
	if err != nil {
		return fmt.Errorf("rotating service account key: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrServiceAccountNotFound
	}
	return nil
}

func (r *serviceAccountRepo) Delete(ctx context.Context, tenantID, saID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM service_accounts WHERE id = $1 AND tenant_id = $2", saID, tenantID)
	if err != nil {
		return fmt.Errorf("deleting service account: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrServiceAccountNotFound
	}
	return nil
}

func (r *serviceAccountRepo) UpdateLastUsed(ctx context.Context, saID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE service_accounts SET last_used_at = NOW() WHERE id = $1", saID)
	if err != nil {
		return fmt.Errorf("updating last_used_at: %w", err)
	}
	return nil
}

// --- Service Account Permissions ---

type serviceAccountPermissionRepo struct {
	db *sqlx.DB
}

// NewServiceAccountPermissionRepo creates a new ServiceAccountPermissionRepository.
func NewServiceAccountPermissionRepo(db *sqlx.DB) port.ServiceAccountPermissionRepository {
	return &serviceAccountPermissionRepo{db: db}
}

func (r *serviceAccountPermissionRepo) Upsert(ctx context.Context, perm *domain.ServiceAccountPermission) error {
	perm.ID = uuid.New()
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO service_account_permissions (id, service_account_id, collection_id, tenant_id, permission, granted_by, created_at)
		VALUES (:id, :service_account_id, :collection_id, :tenant_id, :permission, :granted_by, NOW())
		ON CONFLICT (service_account_id, collection_id)
		DO UPDATE SET permission = :permission, granted_by = :granted_by`,
		perm)
	if err != nil {
		return fmt.Errorf("upserting service account permission: %w", err)
	}
	return nil
}

func (r *serviceAccountPermissionRepo) GetByAccountAndCollection(ctx context.Context, saID, collectionID uuid.UUID) (*domain.ServiceAccountPermission, error) {
	var perm domain.ServiceAccountPermission
	err := r.db.GetContext(ctx, &perm,
		"SELECT * FROM service_account_permissions WHERE service_account_id = $1 AND collection_id = $2",
		saID, collectionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrCollectionPermDenied
		}
		return nil, fmt.Errorf("getting service account permission: %w", err)
	}
	return &perm, nil
}

func (r *serviceAccountPermissionRepo) ListByAccount(ctx context.Context, saID uuid.UUID) ([]domain.ServiceAccountPermission, error) {
	var perms []domain.ServiceAccountPermission
	err := r.db.SelectContext(ctx, &perms,
		"SELECT * FROM service_account_permissions WHERE service_account_id = $1 ORDER BY created_at DESC", saID)
	if err != nil {
		return nil, fmt.Errorf("listing service account permissions: %w", err)
	}
	return perms, nil
}

func (r *serviceAccountPermissionRepo) Delete(ctx context.Context, saID, collectionID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM service_account_permissions WHERE service_account_id = $1 AND collection_id = $2",
		saID, collectionID)
	if err != nil {
		return fmt.Errorf("deleting service account permission: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}
