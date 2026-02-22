package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/port"
	"satvos/internal/service"
	"satvos/mocks"
)

func newEmailConfigService() (service.EmailConfigService, *mocks.MockEmailConfigRepo, *mocks.MockTenantRepo, *mocks.MockServiceAccountService) {
	repo := new(mocks.MockEmailConfigRepo)
	tenantRepo := new(mocks.MockTenantRepo)
	saSvc := new(mocks.MockServiceAccountService)
	svc := service.NewEmailConfigService(repo, tenantRepo, saSvc, "https://api.satvos.com")
	return svc, repo, tenantRepo, saSvc
}

func TestEmailConfigService_GetConfig_EntryExists(t *testing.T) {
	svc, repo, tenantRepo, _ := newEmailConfigService()

	tenantID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	existing := &port.TenantEmailConfig{
		TenantSlug:     "acme",
		ServiceAPIKey:  "sk_secret",
		Enabled:        true,
		AllowedSenders: []string{"@acme.com"},
		APIBaseURL:     "https://api.satvos.com",
	}
	repo.On("Get", mock.Anything, "acme").Return(existing, nil)

	out, err := svc.GetConfig(context.Background(), tenantID)

	assert.NoError(t, err)
	assert.Equal(t, "acme", out.TenantSlug)
	assert.True(t, out.Enabled)
	assert.Equal(t, []string{"@acme.com"}, out.AllowedSenders)
	assert.True(t, out.HasServiceAccount)
	tenantRepo.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestEmailConfigService_GetConfig_NoEntry_AutoCreates(t *testing.T) {
	svc, repo, tenantRepo, saSvc := newEmailConfigService()

	tenantID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	// First Get returns not found
	repo.On("Get", mock.Anything, "acme").Return(nil, domain.ErrNotFound).Once()

	// ensureEntry: Get returns not found (same call, reused)
	repo.On("Get", mock.Anything, "acme").Return(nil, domain.ErrNotFound).Once()

	// SA creates new key
	saSvc.On("GetOrCreateInboundEmailKey", mock.Anything, tenantID, uuid.Nil).Return("sk_newkey123", nil)

	// Put the new entry
	repo.On("Put", mock.Anything, mock.AnythingOfType("*port.TenantEmailConfig")).Return(nil)

	// Re-read after auto-create
	created := &port.TenantEmailConfig{
		TenantSlug:     "acme",
		ServiceAPIKey:  "sk_newkey123",
		Enabled:        false,
		AllowedSenders: []string{},
		APIBaseURL:     "https://api.satvos.com",
	}
	repo.On("Get", mock.Anything, "acme").Return(created, nil).Once()

	out, err := svc.GetConfig(context.Background(), tenantID)

	assert.NoError(t, err)
	assert.Equal(t, "acme", out.TenantSlug)
	assert.False(t, out.Enabled)
	assert.Empty(t, out.AllowedSenders)
	assert.True(t, out.HasServiceAccount)
	saSvc.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestEmailConfigService_GetConfig_NoEntry_SAExists_RotatesKey(t *testing.T) {
	svc, repo, tenantRepo, saSvc := newEmailConfigService()

	tenantID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	// First Get returns not found
	repo.On("Get", mock.Anything, "acme").Return(nil, domain.ErrNotFound).Once()
	// ensureEntry: Get returns not found
	repo.On("Get", mock.Anything, "acme").Return(nil, domain.ErrNotFound).Once()

	// GetOrCreateInboundEmailKey handles SA already existing by rotating
	saSvc.On("GetOrCreateInboundEmailKey", mock.Anything, tenantID, uuid.Nil).Return("sk_rotatedkey", nil)

	repo.On("Put", mock.Anything, mock.AnythingOfType("*port.TenantEmailConfig")).Return(nil)

	created := &port.TenantEmailConfig{
		TenantSlug:     "acme",
		ServiceAPIKey:  "sk_rotatedkey",
		Enabled:        false,
		AllowedSenders: []string{},
		APIBaseURL:     "https://api.satvos.com",
	}
	repo.On("Get", mock.Anything, "acme").Return(created, nil).Once()

	out, err := svc.GetConfig(context.Background(), tenantID)

	assert.NoError(t, err)
	assert.Equal(t, "acme", out.TenantSlug)
	assert.True(t, out.HasServiceAccount)
	saSvc.AssertExpectations(t)
}

func TestEmailConfigService_UpdateConfig_UpdatesBothFields(t *testing.T) {
	svc, repo, tenantRepo, _ := newEmailConfigService()

	tenantID := uuid.New()
	callerID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	existing := &port.TenantEmailConfig{
		TenantSlug:     "acme",
		Enabled:        false,
		AllowedSenders: []string{},
		APIBaseURL:     "https://api.satvos.com",
	}
	// ensureEntry check
	repo.On("Get", mock.Anything, "acme").Return(existing, nil).Once()
	// Read existing for merge
	repo.On("Get", mock.Anything, "acme").Return(existing, nil).Once()

	enabled := true
	senders := []string{"@acme.com", "vendor@example.com"}
	repo.On("UpdateConfig", mock.Anything, "acme", true, senders).Return(nil)

	updated := &port.TenantEmailConfig{
		TenantSlug:     "acme",
		Enabled:        true,
		AllowedSenders: []string{"@acme.com", "vendor@example.com"},
		APIBaseURL:     "https://api.satvos.com",
	}
	repo.On("Get", mock.Anything, "acme").Return(updated, nil).Once()

	out, err := svc.UpdateConfig(context.Background(), tenantID, callerID, service.UpdateEmailConfigInput{
		Enabled:        &enabled,
		AllowedSenders: &senders,
	})

	assert.NoError(t, err)
	assert.True(t, out.Enabled)
	assert.Equal(t, []string{"@acme.com", "vendor@example.com"}, out.AllowedSenders)
	repo.AssertExpectations(t)
}

func TestEmailConfigService_UpdateConfig_PartialUpdate(t *testing.T) {
	svc, repo, tenantRepo, _ := newEmailConfigService()

	tenantID := uuid.New()
	callerID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	existing := &port.TenantEmailConfig{
		TenantSlug:     "acme",
		Enabled:        false,
		AllowedSenders: []string{"@old.com"},
		APIBaseURL:     "https://api.satvos.com",
	}
	repo.On("Get", mock.Anything, "acme").Return(existing, nil).Once()
	repo.On("Get", mock.Anything, "acme").Return(existing, nil).Once()

	// Only updating enabled, senders should stay as @old.com
	enabled := true
	repo.On("UpdateConfig", mock.Anything, "acme", true, []string{"@old.com"}).Return(nil)

	updated := &port.TenantEmailConfig{
		TenantSlug:     "acme",
		Enabled:        true,
		AllowedSenders: []string{"@old.com"},
		APIBaseURL:     "https://api.satvos.com",
	}
	repo.On("Get", mock.Anything, "acme").Return(updated, nil).Once()

	out, err := svc.UpdateConfig(context.Background(), tenantID, callerID, service.UpdateEmailConfigInput{
		Enabled: &enabled,
	})

	assert.NoError(t, err)
	assert.True(t, out.Enabled)
	assert.Equal(t, []string{"@old.com"}, out.AllowedSenders)
	repo.AssertExpectations(t)
}

func TestEmailConfigService_UpdateConfig_InvalidAllowedSenders(t *testing.T) {
	svc, repo, tenantRepo, _ := newEmailConfigService()

	tenantID := uuid.New()
	callerID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	existing := &port.TenantEmailConfig{
		TenantSlug:     "acme",
		Enabled:        false,
		AllowedSenders: []string{},
		APIBaseURL:     "https://api.satvos.com",
	}
	repo.On("Get", mock.Anything, "acme").Return(existing, nil).Once()

	badSenders := []string{"not-an-email"}

	_, err := svc.UpdateConfig(context.Background(), tenantID, callerID, service.UpdateEmailConfigInput{
		AllowedSenders: &badSenders,
	})

	assert.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidAllowedSender)
}

func TestEmailConfigService_UpdateConfig_ValidFormats(t *testing.T) {
	svc, repo, tenantRepo, _ := newEmailConfigService()

	tenantID := uuid.New()
	callerID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	existing := &port.TenantEmailConfig{
		TenantSlug:     "acme",
		Enabled:        false,
		AllowedSenders: []string{},
		APIBaseURL:     "https://api.satvos.com",
	}
	// ensureEntry check
	repo.On("Get", mock.Anything, "acme").Return(existing, nil).Once()
	// Read for merge
	repo.On("Get", mock.Anything, "acme").Return(existing, nil).Once()

	validSenders := []string{"*", "@acme.com", "vendor@example.com"}
	repo.On("UpdateConfig", mock.Anything, "acme", false, validSenders).Return(nil)

	updated := &port.TenantEmailConfig{
		TenantSlug:     "acme",
		Enabled:        false,
		AllowedSenders: validSenders,
		APIBaseURL:     "https://api.satvos.com",
	}
	repo.On("Get", mock.Anything, "acme").Return(updated, nil).Once()

	out, err := svc.UpdateConfig(context.Background(), tenantID, callerID, service.UpdateEmailConfigInput{
		AllowedSenders: &validSenders,
	})

	assert.NoError(t, err)
	assert.Equal(t, validSenders, out.AllowedSenders)
	repo.AssertExpectations(t)
}

func TestEmailConfigService_UpdateConfig_NoEntry_AutoCreatesThenUpdates(t *testing.T) {
	svc, repo, tenantRepo, saSvc := newEmailConfigService()

	tenantID := uuid.New()
	callerID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	// ensureEntry: Get returns not found
	repo.On("Get", mock.Anything, "acme").Return(nil, domain.ErrNotFound).Once()
	saSvc.On("GetOrCreateInboundEmailKey", mock.Anything, tenantID, callerID).Return("sk_newkey", nil)
	repo.On("Put", mock.Anything, mock.AnythingOfType("*port.TenantEmailConfig")).Return(nil)

	// Read existing for merge
	created := &port.TenantEmailConfig{
		TenantSlug:     "acme",
		Enabled:        false,
		AllowedSenders: []string{},
		APIBaseURL:     "https://api.satvos.com",
	}
	repo.On("Get", mock.Anything, "acme").Return(created, nil).Once()

	enabled := true
	senders := []string{"@acme.com"}
	repo.On("UpdateConfig", mock.Anything, "acme", true, senders).Return(nil)

	updated := &port.TenantEmailConfig{
		TenantSlug:     "acme",
		Enabled:        true,
		AllowedSenders: senders,
		APIBaseURL:     "https://api.satvos.com",
	}
	repo.On("Get", mock.Anything, "acme").Return(updated, nil).Once()

	out, err := svc.UpdateConfig(context.Background(), tenantID, callerID, service.UpdateEmailConfigInput{
		Enabled:        &enabled,
		AllowedSenders: &senders,
	})

	assert.NoError(t, err)
	assert.True(t, out.Enabled)
	assert.Equal(t, senders, out.AllowedSenders)
	saSvc.AssertExpectations(t)
	repo.AssertExpectations(t)
}
