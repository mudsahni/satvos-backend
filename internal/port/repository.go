package port

import (
	"context"

	"github.com/google/uuid"

	"satvos/internal/domain"
)

// TenantRepository defines the contract for tenant persistence.
type TenantRepository interface {
	Create(ctx context.Context, tenant *domain.Tenant) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error)
	List(ctx context.Context, offset, limit int) ([]domain.Tenant, int, error)
	Update(ctx context.Context, tenant *domain.Tenant) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// UserRepository defines the contract for user persistence.
// All query methods include tenantID to enforce tenant isolation at the data layer.
type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, tenantID, userID uuid.UUID) (*domain.User, error)
	GetByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*domain.User, error)
	ListByTenant(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.User, int, error)
	Update(ctx context.Context, user *domain.User) error
	Delete(ctx context.Context, tenantID, userID uuid.UUID) error
	CheckAndIncrementQuota(ctx context.Context, tenantID, userID uuid.UUID) error
	SetEmailVerified(ctx context.Context, tenantID, userID uuid.UUID) error
	SetPasswordResetToken(ctx context.Context, tenantID, userID uuid.UUID, tokenID string) error
	ResetPassword(ctx context.Context, tenantID, userID uuid.UUID, passwordHash, expectedTokenID string) error
	GetByProviderID(ctx context.Context, tenantID uuid.UUID, provider domain.AuthProvider, providerUserID string) (*domain.User, error)
	LinkProvider(ctx context.Context, tenantID, userID uuid.UUID, provider domain.AuthProvider, providerUserID string) error
	AcceptInvitation(ctx context.Context, tenantID, userID uuid.UUID, passwordHash, expectedTokenID string) error
}

// FileMetaRepository defines the contract for file metadata persistence.
// All query methods include tenantID for tenant isolation.
type FileMetaRepository interface {
	Create(ctx context.Context, meta *domain.FileMeta) error
	GetByID(ctx context.Context, tenantID, fileID uuid.UUID) (*domain.FileMeta, error)
	ListByTenant(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.FileMeta, int, error)
	ListByUploader(ctx context.Context, tenantID, userID uuid.UUID, offset, limit int) ([]domain.FileMeta, int, error)
	UpdateStatus(ctx context.Context, tenantID, fileID uuid.UUID, status domain.FileStatus) error
	Delete(ctx context.Context, tenantID, fileID uuid.UUID) error
}
