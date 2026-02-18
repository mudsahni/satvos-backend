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
	GetByAPIKeyPrefix(ctx context.Context, prefix string) ([]domain.ServiceAccount, error)
	ListByTenant(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.ServiceAccount, int, error)
	Update(ctx context.Context, sa *domain.ServiceAccount) error
	Delete(ctx context.Context, tenantID, saID uuid.UUID) error
	UpdateLastUsed(ctx context.Context, saID uuid.UUID) error
}

// ServiceAccountPermission represents a service account's permission on a collection.
type ServiceAccountPermission struct {
	ID               uuid.UUID                  `db:"id" json:"id"`
	ServiceAccountID uuid.UUID                  `db:"service_account_id" json:"service_account_id"`
	CollectionID     uuid.UUID                  `db:"collection_id" json:"collection_id"`
	TenantID         uuid.UUID                  `db:"tenant_id" json:"tenant_id"`
	Permission       domain.CollectionPermission `db:"permission" json:"permission"`
	GrantedBy        uuid.UUID                  `db:"granted_by" json:"granted_by"`
}

// ServiceAccountPermissionRepository defines persistence operations for service account permissions.
type ServiceAccountPermissionRepository interface {
	Upsert(ctx context.Context, perm *ServiceAccountPermission) error
	GetByAccountAndCollection(ctx context.Context, saID, collectionID uuid.UUID) (*ServiceAccountPermission, error)
	ListByAccount(ctx context.Context, saID uuid.UUID) ([]ServiceAccountPermission, error)
	Delete(ctx context.Context, saID, collectionID uuid.UUID) error
}
