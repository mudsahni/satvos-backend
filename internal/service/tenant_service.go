package service

import (
	"context"
	"log"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/port"
)

// CreateTenantInput is the DTO for creating a tenant.
type CreateTenantInput struct {
	Name string `json:"name" binding:"required"`
	Slug string `json:"slug" binding:"required"`
}

// UpdateTenantInput is the DTO for updating a tenant.
type UpdateTenantInput struct {
	Name     *string `json:"name"`
	Slug     *string `json:"slug"`
	IsActive *bool   `json:"is_active"`
}

// TenantService defines the tenant management contract.
type TenantService interface {
	Create(ctx context.Context, input CreateTenantInput) (*domain.Tenant, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error)
	List(ctx context.Context, offset, limit int) ([]domain.Tenant, int, error)
	Update(ctx context.Context, id uuid.UUID, input UpdateTenantInput) (*domain.Tenant, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type tenantService struct {
	repo  port.TenantRepository
	saSvc ServiceAccountService // optional, nil-safe
}

// NewTenantService creates a new TenantService implementation.
func NewTenantService(repo port.TenantRepository, saSvc ServiceAccountService) TenantService {
	return &tenantService{repo: repo, saSvc: saSvc}
}

func (s *tenantService) Create(ctx context.Context, input CreateTenantInput) (*domain.Tenant, error) {
	tenant := &domain.Tenant{
		Name:     input.Name,
		Slug:     input.Slug,
		IsActive: true,
	}
	if err := s.repo.Create(ctx, tenant); err != nil {
		return nil, err
	}

	// Auto-create inbound_email service account (non-blocking)
	if s.saSvc != nil {
		apiKey, saErr := s.saSvc.EnsureInboundEmailServiceAccount(ctx, tenant.ID, tenant.ID)
		if saErr != nil {
			log.Printf("WARNING: failed to auto-create inbound_email SA for tenant %s: %v", tenant.ID, saErr)
		} else if apiKey != "" {
			log.Printf("Auto-created inbound_email service account for tenant %s", tenant.ID)
		}
	}

	return tenant, nil
}

func (s *tenantService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *tenantService) List(ctx context.Context, offset, limit int) ([]domain.Tenant, int, error) {
	return s.repo.List(ctx, offset, limit)
}

// Update updates a tenant by ID. Callers MUST ensure id is the caller's own tenant ID
// (enforced at the handler layer via JWT tenant_id comparison). The handler also strips
// IsActive before calling this method to prevent tenant admin self-lockout.
func (s *tenantService) Update(ctx context.Context, id uuid.UUID, input UpdateTenantInput) (*domain.Tenant, error) {
	tenant, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Name != nil {
		tenant.Name = *input.Name
	}
	if input.Slug != nil {
		tenant.Slug = *input.Slug
	}
	if input.IsActive != nil {
		tenant.IsActive = *input.IsActive
	}

	if err := s.repo.Update(ctx, tenant); err != nil {
		return nil, err
	}
	return tenant, nil
}

func (s *tenantService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}
