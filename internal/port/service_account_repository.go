package port

import (
	"context"

	"github.com/google/uuid"

	"satvos/internal/domain"
)

// ServiceAccountRepository defines persistence operations for service accounts.
type ServiceAccountRepository interface {
	Create(ctx context.Context, sa *domain.ServiceAccount) error
	GetByID(ctx context.Context, tenantID, saID uuid.UUID) (*domain.ServiceAccount, error)
	GetByName(ctx context.Context, tenantID uuid.UUID, name string) (*domain.ServiceAccount, error)
	GetByAPIKeyPrefix(ctx context.Context, prefix string) ([]domain.ServiceAccount, error)
	ListByTenant(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.ServiceAccount, int, error)
	Update(ctx context.Context, sa *domain.ServiceAccount) error
	Revoke(ctx context.Context, tenantID, saID uuid.UUID) error
	RotateKey(ctx context.Context, tenantID, saID uuid.UUID, newPrefix, newHash string) (*domain.ServiceAccount, error)
	Delete(ctx context.Context, tenantID, saID uuid.UUID) error
	UpdateLastUsed(ctx context.Context, saID uuid.UUID) error
}

// ServiceAccountPermissionRepository defines persistence operations for service account permissions.
type ServiceAccountPermissionRepository interface {
	Upsert(ctx context.Context, perm *domain.ServiceAccountPermission) error
	GetByAccountAndCollection(ctx context.Context, saID, collectionID uuid.UUID) (*domain.ServiceAccountPermission, error)
	GetByAccountForCollections(ctx context.Context, saID uuid.UUID, collectionIDs []uuid.UUID) (map[uuid.UUID]domain.CollectionPermission, error)
	ListByAccount(ctx context.Context, tenantID, saID uuid.UUID) ([]domain.ServiceAccountPermission, error)
	Delete(ctx context.Context, tenantID, saID, collectionID uuid.UUID) error
}
