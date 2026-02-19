package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/service"
	"satvos/mocks"
)

func TestTenantService_Create_Success(t *testing.T) {
	repo := new(mocks.MockTenantRepo)
	svc := service.NewTenantService(repo, nil)

	repo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Tenant")).Return(nil)

	tenant, err := svc.Create(context.Background(), service.CreateTenantInput{
		Name: "Acme Corp",
		Slug: "acme-corp",
	})

	assert.NoError(t, err)
	assert.Equal(t, "Acme Corp", tenant.Name)
	assert.Equal(t, "acme-corp", tenant.Slug)
	assert.True(t, tenant.IsActive)
	repo.AssertExpectations(t)
}

func TestTenantService_Create_DuplicateSlug(t *testing.T) {
	repo := new(mocks.MockTenantRepo)
	svc := service.NewTenantService(repo, nil)

	repo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Tenant")).Return(domain.ErrDuplicateTenantSlug)

	tenant, err := svc.Create(context.Background(), service.CreateTenantInput{
		Name: "Acme Corp",
		Slug: "existing-slug",
	})

	assert.Nil(t, tenant)
	assert.ErrorIs(t, err, domain.ErrDuplicateTenantSlug)
}

func TestTenantService_GetByID_Success(t *testing.T) {
	repo := new(mocks.MockTenantRepo)
	svc := service.NewTenantService(repo, nil)

	tenantID := uuid.New()
	expected := &domain.Tenant{ID: tenantID, Name: "Acme Corp", Slug: "acme-corp", IsActive: true}
	repo.On("GetByID", mock.Anything, tenantID).Return(expected, nil)

	tenant, err := svc.GetByID(context.Background(), tenantID)

	assert.NoError(t, err)
	assert.Equal(t, expected, tenant)
}

func TestTenantService_GetByID_NotFound(t *testing.T) {
	repo := new(mocks.MockTenantRepo)
	svc := service.NewTenantService(repo, nil)

	tenantID := uuid.New()
	repo.On("GetByID", mock.Anything, tenantID).Return(nil, domain.ErrNotFound)

	tenant, err := svc.GetByID(context.Background(), tenantID)

	assert.Nil(t, tenant)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestTenantService_List_Success(t *testing.T) {
	repo := new(mocks.MockTenantRepo)
	svc := service.NewTenantService(repo, nil)

	expected := []domain.Tenant{
		{ID: uuid.New(), Name: "Tenant A"},
		{ID: uuid.New(), Name: "Tenant B"},
	}
	repo.On("List", mock.Anything, 0, 20).Return(expected, 2, nil)

	tenants, total, err := svc.List(context.Background(), 0, 20)

	assert.NoError(t, err)
	assert.Len(t, tenants, 2)
	assert.Equal(t, 2, total)
}

func TestTenantService_Update_Success(t *testing.T) {
	repo := new(mocks.MockTenantRepo)
	svc := service.NewTenantService(repo, nil)

	tenantID := uuid.New()
	existing := &domain.Tenant{ID: tenantID, Name: "Old Name", Slug: "old-slug", IsActive: true}
	newName := "New Name"

	repo.On("GetByID", mock.Anything, tenantID).Return(existing, nil)
	repo.On("Update", mock.Anything, mock.AnythingOfType("*domain.Tenant")).Return(nil)

	tenant, err := svc.Update(context.Background(), tenantID, service.UpdateTenantInput{
		Name: &newName,
	})

	assert.NoError(t, err)
	assert.Equal(t, "New Name", tenant.Name)
	repo.AssertExpectations(t)
}

func TestTenantService_Delete_Success(t *testing.T) {
	repo := new(mocks.MockTenantRepo)
	svc := service.NewTenantService(repo, nil)

	tenantID := uuid.New()
	repo.On("Delete", mock.Anything, tenantID).Return(nil)

	err := svc.Delete(context.Background(), tenantID)

	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestTenantService_Delete_NotFound(t *testing.T) {
	repo := new(mocks.MockTenantRepo)
	svc := service.NewTenantService(repo, nil)

	tenantID := uuid.New()
	repo.On("Delete", mock.Anything, tenantID).Return(domain.ErrNotFound)

	err := svc.Delete(context.Background(), tenantID)

	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestTenantService_Create_AutoCreatesSA(t *testing.T) {
	repo := new(mocks.MockTenantRepo)
	saSvc := new(mocks.MockServiceAccountService)
	svc := service.NewTenantService(repo, saSvc)

	repo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Tenant")).Return(nil)
	saSvc.On("EnsureInboundEmailServiceAccount", mock.Anything, mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("uuid.UUID")).
		Return("sk_test_key", nil)

	tenant, err := svc.Create(context.Background(), service.CreateTenantInput{
		Name: "Acme Corp",
		Slug: "acme-corp",
	})

	assert.NoError(t, err)
	assert.NotNil(t, tenant)
	saSvc.AssertCalled(t, "EnsureInboundEmailServiceAccount", mock.Anything, tenant.ID, tenant.ID)
}

func TestTenantService_Create_SAFailureNonBlocking(t *testing.T) {
	repo := new(mocks.MockTenantRepo)
	saSvc := new(mocks.MockServiceAccountService)
	svc := service.NewTenantService(repo, saSvc)

	repo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Tenant")).Return(nil)
	saSvc.On("EnsureInboundEmailServiceAccount", mock.Anything, mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("uuid.UUID")).
		Return("", assert.AnError)

	tenant, err := svc.Create(context.Background(), service.CreateTenantInput{
		Name: "Acme Corp",
		Slug: "acme-corp",
	})

	// Tenant creation should still succeed
	assert.NoError(t, err)
	assert.NotNil(t, tenant)
	assert.Equal(t, "Acme Corp", tenant.Name)
}
