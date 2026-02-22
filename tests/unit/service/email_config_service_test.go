package service_test

import (
	"context"
	"errors"
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

// --- GetConfig tests ---

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
	tenantRepo.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestEmailConfigService_GetConfig_NoEntry_AutoCreates(t *testing.T) {
	svc, repo, tenantRepo, saSvc := newEmailConfigService()

	tenantID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	// getOrCreateEntry: Get returns not found
	repo.On("Get", mock.Anything, "acme").Return(nil, domain.ErrNotFound).Once()

	// SA creates new key
	saSvc.On("GetOrCreateInboundEmailKey", mock.Anything, tenantID, uuid.Nil).Return("sk_newkey123", nil)

	// Put the new entry
	repo.On("Put", mock.Anything, mock.AnythingOfType("*port.TenantEmailConfig")).Return(nil)

	// Re-read after create
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
	saSvc.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestEmailConfigService_GetConfig_NoEntry_SAExists_RotatesKey(t *testing.T) {
	svc, repo, tenantRepo, saSvc := newEmailConfigService()

	tenantID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	// getOrCreateEntry: Get returns not found
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
	saSvc.AssertExpectations(t)
}

func TestEmailConfigService_GetConfig_TenantNotFound(t *testing.T) {
	svc, _, tenantRepo, _ := newEmailConfigService()

	tenantID := uuid.New()
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(nil, domain.ErrNotFound)

	out, err := svc.GetConfig(context.Background(), tenantID)

	assert.Nil(t, out)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestEmailConfigService_GetConfig_DynamoDBError(t *testing.T) {
	svc, repo, tenantRepo, _ := newEmailConfigService()

	tenantID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	repo.On("Get", mock.Anything, "acme").Return(nil, errors.New("connection refused"))

	out, err := svc.GetConfig(context.Background(), tenantID)

	assert.Nil(t, out)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "getting email config")
}

func TestEmailConfigService_GetConfig_SACreationFails(t *testing.T) {
	svc, repo, tenantRepo, saSvc := newEmailConfigService()

	tenantID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	repo.On("Get", mock.Anything, "acme").Return(nil, domain.ErrNotFound).Once()
	saSvc.On("GetOrCreateInboundEmailKey", mock.Anything, tenantID, uuid.Nil).Return("", errors.New("sa creation failed"))

	out, err := svc.GetConfig(context.Background(), tenantID)

	assert.Nil(t, out)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "getting inbound email API key")
}

func TestEmailConfigService_GetConfig_PutFails(t *testing.T) {
	svc, repo, tenantRepo, saSvc := newEmailConfigService()

	tenantID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	repo.On("Get", mock.Anything, "acme").Return(nil, domain.ErrNotFound).Once()
	saSvc.On("GetOrCreateInboundEmailKey", mock.Anything, tenantID, uuid.Nil).Return("sk_key", nil)
	repo.On("Put", mock.Anything, mock.AnythingOfType("*port.TenantEmailConfig")).Return(errors.New("dynamo write failed"))

	out, err := svc.GetConfig(context.Background(), tenantID)

	assert.Nil(t, out)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "creating email config entry")
}

// --- UpdateConfig tests ---

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
	// getOrCreateEntry returns existing
	repo.On("Get", mock.Anything, "acme").Return(existing, nil).Once()

	enabled := true
	senders := []string{"@acme.com", "vendor@example.com"}
	// After normalization, should be lowercased (already is)
	repo.On("UpdateConfig", mock.Anything, "acme", true, []string{"@acme.com", "vendor@example.com"}).Return(nil)

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

	// getOrCreateEntry: Get returns not found
	repo.On("Get", mock.Anything, "acme").Return(nil, domain.ErrNotFound).Once()
	saSvc.On("GetOrCreateInboundEmailKey", mock.Anything, tenantID, callerID).Return("sk_newkey", nil)
	repo.On("Put", mock.Anything, mock.AnythingOfType("*port.TenantEmailConfig")).Return(nil)

	// Re-read after create in getOrCreateEntry
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

func TestEmailConfigService_UpdateConfig_TenantNotFound(t *testing.T) {
	svc, _, tenantRepo, _ := newEmailConfigService()

	tenantID := uuid.New()
	callerID := uuid.New()
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(nil, domain.ErrNotFound)

	enabled := true
	_, err := svc.UpdateConfig(context.Background(), tenantID, callerID, service.UpdateEmailConfigInput{
		Enabled: &enabled,
	})

	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// --- Validation edge case tests ---

func TestEmailConfigService_Validation_EmptyString(t *testing.T) {
	svc, repo, tenantRepo, _ := newEmailConfigService()

	tenantID := uuid.New()
	callerID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	existing := &port.TenantEmailConfig{TenantSlug: "acme", AllowedSenders: []string{}}
	repo.On("Get", mock.Anything, "acme").Return(existing, nil).Once()

	badSenders := []string{""}
	_, err := svc.UpdateConfig(context.Background(), tenantID, callerID, service.UpdateEmailConfigInput{
		AllowedSenders: &badSenders,
	})

	assert.ErrorIs(t, err, domain.ErrInvalidAllowedSender)
	assert.Contains(t, err.Error(), "empty entry")
}

func TestEmailConfigService_Validation_AtSignOnly(t *testing.T) {
	svc, repo, tenantRepo, _ := newEmailConfigService()

	tenantID := uuid.New()
	callerID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	existing := &port.TenantEmailConfig{TenantSlug: "acme", AllowedSenders: []string{}}
	repo.On("Get", mock.Anything, "acme").Return(existing, nil).Once()

	badSenders := []string{"@"}
	_, err := svc.UpdateConfig(context.Background(), tenantID, callerID, service.UpdateEmailConfigInput{
		AllowedSenders: &badSenders,
	})

	assert.ErrorIs(t, err, domain.ErrInvalidAllowedSender)
}

func TestEmailConfigService_Validation_AtDot(t *testing.T) {
	svc, repo, tenantRepo, _ := newEmailConfigService()

	tenantID := uuid.New()
	callerID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	existing := &port.TenantEmailConfig{TenantSlug: "acme", AllowedSenders: []string{}}
	repo.On("Get", mock.Anything, "acme").Return(existing, nil).Once()

	badSenders := []string{"@."}
	_, err := svc.UpdateConfig(context.Background(), tenantID, callerID, service.UpdateEmailConfigInput{
		AllowedSenders: &badSenders,
	})

	assert.ErrorIs(t, err, domain.ErrInvalidAllowedSender)
}

func TestEmailConfigService_Validation_DisplayNameRejected(t *testing.T) {
	svc, repo, tenantRepo, _ := newEmailConfigService()

	tenantID := uuid.New()
	callerID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	existing := &port.TenantEmailConfig{TenantSlug: "acme", AllowedSenders: []string{}}
	repo.On("Get", mock.Anything, "acme").Return(existing, nil).Once()

	badSenders := []string{"John Doe <john@example.com>"}
	_, err := svc.UpdateConfig(context.Background(), tenantID, callerID, service.UpdateEmailConfigInput{
		AllowedSenders: &badSenders,
	})

	assert.ErrorIs(t, err, domain.ErrInvalidAllowedSender)
	assert.Contains(t, err.Error(), "plain email address")
}

func TestEmailConfigService_Validation_WhitespaceTrimmingAndLowercase(t *testing.T) {
	svc, repo, tenantRepo, _ := newEmailConfigService()

	tenantID := uuid.New()
	callerID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	existing := &port.TenantEmailConfig{TenantSlug: "acme", AllowedSenders: []string{}}
	repo.On("Get", mock.Anything, "acme").Return(existing, nil).Once()

	// Input has whitespace and mixed case — should be normalized
	senders := []string{" @ACME.COM ", " User@Example.COM "}
	// After normalization: lowercase, trimmed
	repo.On("UpdateConfig", mock.Anything, "acme", false, []string{"@acme.com", "user@example.com"}).Return(nil)

	updated := &port.TenantEmailConfig{
		TenantSlug:     "acme",
		AllowedSenders: []string{"@acme.com", "user@example.com"},
	}
	repo.On("Get", mock.Anything, "acme").Return(updated, nil).Once()

	out, err := svc.UpdateConfig(context.Background(), tenantID, callerID, service.UpdateEmailConfigInput{
		AllowedSenders: &senders,
	})

	assert.NoError(t, err)
	assert.Equal(t, []string{"@acme.com", "user@example.com"}, out.AllowedSenders)
}

func TestEmailConfigService_Validation_Deduplication(t *testing.T) {
	svc, repo, tenantRepo, _ := newEmailConfigService()

	tenantID := uuid.New()
	callerID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "acme"}
	tenantRepo.On("GetByID", mock.Anything, tenantID).Return(tenant, nil)

	existing := &port.TenantEmailConfig{TenantSlug: "acme", AllowedSenders: []string{}}
	repo.On("Get", mock.Anything, "acme").Return(existing, nil).Once()

	// Duplicates should be removed
	senders := []string{"@acme.com", "@ACME.COM", "user@test.com", "user@test.com"}
	// After normalization: deduplicated
	repo.On("UpdateConfig", mock.Anything, "acme", false, []string{"@acme.com", "user@test.com"}).Return(nil)

	updated := &port.TenantEmailConfig{
		TenantSlug:     "acme",
		AllowedSenders: []string{"@acme.com", "user@test.com"},
	}
	repo.On("Get", mock.Anything, "acme").Return(updated, nil).Once()

	out, err := svc.UpdateConfig(context.Background(), tenantID, callerID, service.UpdateEmailConfigInput{
		AllowedSenders: &senders,
	})

	assert.NoError(t, err)
	assert.Equal(t, []string{"@acme.com", "user@test.com"}, out.AllowedSenders)
}
