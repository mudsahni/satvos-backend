package port

import (
	"context"

	"github.com/google/uuid"

	"satvos/internal/domain"
)

// CollectionRepository defines the contract for collection persistence.
type CollectionRepository interface {
	Create(ctx context.Context, collection *domain.Collection) error
	GetByID(ctx context.Context, tenantID, collectionID uuid.UUID) (*domain.Collection, error)
	ListByUser(ctx context.Context, tenantID, userID uuid.UUID, offset, limit int) ([]domain.Collection, int, error)
	ListByServiceAccount(ctx context.Context, tenantID, saID uuid.UUID, offset, limit int) ([]domain.Collection, int, error)
	ListByTenant(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.Collection, int, error)
	Update(ctx context.Context, collection *domain.Collection) error
	Delete(ctx context.Context, tenantID, collectionID uuid.UUID) error
}

// CollectionPermissionRepository defines the contract for collection permission persistence.
type CollectionPermissionRepository interface {
	Upsert(ctx context.Context, perm *domain.CollectionPermissionEntry) error
	GetByCollectionAndUser(ctx context.Context, collectionID, userID uuid.UUID) (*domain.CollectionPermissionEntry, error)
	GetByUserForCollections(ctx context.Context, userID uuid.UUID, collectionIDs []uuid.UUID) (map[uuid.UUID]domain.CollectionPermission, error)
	ListByCollection(ctx context.Context, collectionID uuid.UUID, offset, limit int) ([]domain.CollectionPermissionEntry, int, error)
	ListCollectionIDs(ctx context.Context, tenantID, userID uuid.UUID) ([]uuid.UUID, error)
	Delete(ctx context.Context, collectionID, userID uuid.UUID) error
}

// CollectionFileRepository defines the contract for collection-file association persistence.
type CollectionFileRepository interface {
	Add(ctx context.Context, cf *domain.CollectionFile) error
	Remove(ctx context.Context, collectionID, fileID uuid.UUID) error
	ListByCollection(ctx context.Context, tenantID, collectionID uuid.UUID, offset, limit int) ([]domain.FileMeta, int, error)
}
