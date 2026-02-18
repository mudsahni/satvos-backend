package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"satvos/internal/domain"
	"satvos/internal/service"
	"satvos/mocks"
)

func setupServiceAccountService() (
	*mocks.MockServiceAccountRepo,
	*mocks.MockServiceAccountPermissionRepo,
	*mocks.MockCollectionRepo,
	service.ServiceAccountService,
) {
	saRepo := new(mocks.MockServiceAccountRepo)
	permRepo := new(mocks.MockServiceAccountPermissionRepo)
	collectionRepo := new(mocks.MockCollectionRepo)
	svc := service.NewServiceAccountService(saRepo, permRepo, collectionRepo)
	return saRepo, permRepo, collectionRepo, svc
}

func TestServiceAccountService_Create(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		saRepo, _, _, svc := setupServiceAccountService()

		tenantID := uuid.New()
		callerID := uuid.New()

		saRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.ServiceAccount")).Return(nil)

		output, err := svc.Create(context.Background(), &service.CreateServiceAccountInput{
			TenantID:    tenantID,
			Name:        "tally-integration",
			Description: "Tally ERP integration",
			CreatedBy:   callerID,
		})

		require.NoError(t, err)
		assert.NotEmpty(t, output.APIKey)
		assert.True(t, len(output.APIKey) > 10)
		assert.Contains(t, output.APIKey, "sk_")
		assert.Equal(t, "tally-integration", output.ServiceAccount.Name)
		assert.True(t, output.ServiceAccount.IsActive)
		assert.Equal(t, callerID, output.ServiceAccount.CreatedBy)
		saRepo.AssertExpectations(t)
	})

	t.Run("with_expiry", func(t *testing.T) {
		saRepo, _, _, svc := setupServiceAccountService()

		tenantID := uuid.New()
		callerID := uuid.New()
		expiresAt := time.Now().Add(30 * 24 * time.Hour)

		saRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.ServiceAccount")).Return(nil)

		output, err := svc.Create(context.Background(), &service.CreateServiceAccountInput{
			TenantID:  tenantID,
			Name:      "temp-integration",
			ExpiresAt: &expiresAt,
			CreatedBy: callerID,
		})

		require.NoError(t, err)
		assert.NotNil(t, output.ServiceAccount.ExpiresAt)
		saRepo.AssertExpectations(t)
	})
}

func TestServiceAccountService_Authenticate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		saRepo, _, _, svc := setupServiceAccountService()

		// First create a service account to get the key
		saRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.ServiceAccount")).Return(nil)

		output, err := svc.Create(context.Background(), &service.CreateServiceAccountInput{
			TenantID:  uuid.New(),
			Name:      "test-sa",
			CreatedBy: uuid.New(),
		})
		require.NoError(t, err)

		// Set up the lookup mock
		saRepo.On("GetByAPIKeyPrefix", mock.Anything, mock.AnythingOfType("string")).
			Return([]domain.ServiceAccount{*output.ServiceAccount}, nil)
		saRepo.On("UpdateLastUsed", mock.Anything, mock.AnythingOfType("uuid.UUID")).Return(nil).Maybe()

		// Authenticate with the raw key
		sa, err := svc.Authenticate(context.Background(), output.APIKey)
		require.NoError(t, err)
		assert.Equal(t, output.ServiceAccount.ID, sa.ID)
		saRepo.AssertExpectations(t)
	})

	t.Run("invalid_key_format", func(t *testing.T) {
		_, _, _, svc := setupServiceAccountService()

		_, err := svc.Authenticate(context.Background(), "not-a-valid-key")
		assert.ErrorIs(t, err, domain.ErrAPIKeyInvalid)
	})

	t.Run("wrong_key", func(t *testing.T) {
		saRepo, _, _, svc := setupServiceAccountService()

		// Return empty candidates
		saRepo.On("GetByAPIKeyPrefix", mock.Anything, mock.AnythingOfType("string")).
			Return([]domain.ServiceAccount{}, nil)

		_, err := svc.Authenticate(context.Background(), "sk_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab")
		assert.ErrorIs(t, err, domain.ErrAPIKeyInvalid)
		saRepo.AssertExpectations(t)
	})

	t.Run("expired_key", func(t *testing.T) {
		saRepo, _, _, svc := setupServiceAccountService()

		// Create to get real key
		saRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.ServiceAccount")).
			Run(func(args mock.Arguments) {
				sa := args.Get(1).(*domain.ServiceAccount)
				expired := time.Now().Add(-1 * time.Hour)
				sa.ExpiresAt = &expired
			}).Return(nil)

		output, err := svc.Create(context.Background(), &service.CreateServiceAccountInput{
			TenantID:  uuid.New(),
			Name:      "expired-sa",
			CreatedBy: uuid.New(),
		})
		require.NoError(t, err)

		saRepo.On("GetByAPIKeyPrefix", mock.Anything, mock.AnythingOfType("string")).
			Return([]domain.ServiceAccount{*output.ServiceAccount}, nil)

		_, err = svc.Authenticate(context.Background(), output.APIKey)
		assert.ErrorIs(t, err, domain.ErrAPIKeyRevoked)
		saRepo.AssertExpectations(t)
	})
}

func TestServiceAccountService_RotateAPIKey(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		saRepo, _, _, svc := setupServiceAccountService()

		tenantID := uuid.New()
		saID := uuid.New()

		saRepo.On("RotateKey", mock.Anything, tenantID, saID, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
		saRepo.On("GetByID", mock.Anything, tenantID, saID).Return(&domain.ServiceAccount{
			ID:       saID,
			TenantID: tenantID,
			Name:     "test-sa",
			IsActive: true,
		}, nil)

		output, err := svc.RotateAPIKey(context.Background(), tenantID, saID)
		require.NoError(t, err)
		assert.NotEmpty(t, output.APIKey)
		assert.Contains(t, output.APIKey, "sk_")
		saRepo.AssertExpectations(t)
	})

	t.Run("not_found", func(t *testing.T) {
		saRepo, _, _, svc := setupServiceAccountService()

		tenantID := uuid.New()
		saID := uuid.New()

		// RotateKey returns not found (SA doesn't exist or is revoked)
		saRepo.On("RotateKey", mock.Anything, tenantID, saID, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
			Return(domain.ErrServiceAccountNotFound)

		_, err := svc.RotateAPIKey(context.Background(), tenantID, saID)
		assert.ErrorIs(t, err, domain.ErrServiceAccountNotFound)
		saRepo.AssertExpectations(t)
	})

	t.Run("blocked_on_revoked", func(t *testing.T) {
		saRepo, _, _, svc := setupServiceAccountService()

		tenantID := uuid.New()
		saID := uuid.New()

		// RotateKey checks is_active = TRUE in its WHERE clause, so revoked SA returns not found
		saRepo.On("RotateKey", mock.Anything, tenantID, saID, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
			Return(domain.ErrServiceAccountNotFound)

		_, err := svc.RotateAPIKey(context.Background(), tenantID, saID)
		assert.ErrorIs(t, err, domain.ErrServiceAccountNotFound)
		saRepo.AssertExpectations(t)
	})
}

func TestServiceAccountService_Revoke(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		saRepo, _, _, svc := setupServiceAccountService()

		tenantID := uuid.New()
		saID := uuid.New()

		saRepo.On("Revoke", mock.Anything, tenantID, saID).Return(nil)

		err := svc.Revoke(context.Background(), tenantID, saID)
		require.NoError(t, err)
		saRepo.AssertExpectations(t)
	})

	t.Run("not_found", func(t *testing.T) {
		saRepo, _, _, svc := setupServiceAccountService()

		tenantID := uuid.New()
		saID := uuid.New()

		saRepo.On("Revoke", mock.Anything, tenantID, saID).Return(domain.ErrServiceAccountNotFound)

		err := svc.Revoke(context.Background(), tenantID, saID)
		assert.ErrorIs(t, err, domain.ErrServiceAccountNotFound)
		saRepo.AssertExpectations(t)
	})
}

func TestServiceAccountService_SetPermission(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		saRepo, permRepo, collRepo, svc := setupServiceAccountService()

		tenantID := uuid.New()
		saID := uuid.New()
		collectionID := uuid.New()
		callerID := uuid.New()

		saRepo.On("GetByID", mock.Anything, tenantID, saID).Return(&domain.ServiceAccount{
			ID:       saID,
			TenantID: tenantID,
		}, nil)
		collRepo.On("GetByID", mock.Anything, tenantID, collectionID).Return(&domain.Collection{
			ID:       collectionID,
			TenantID: tenantID,
		}, nil)
		permRepo.On("Upsert", mock.Anything, mock.MatchedBy(func(perm *domain.ServiceAccountPermission) bool {
			return perm.ServiceAccountID == saID &&
				perm.CollectionID == collectionID &&
				perm.Permission == domain.CollectionPermEditor
		})).Return(nil)

		err := svc.SetPermission(context.Background(), &service.SASetPermissionInput{
			TenantID:         tenantID,
			ServiceAccountID: saID,
			CollectionID:     collectionID,
			Permission:       domain.CollectionPermEditor,
			GrantedBy:        callerID,
		})
		require.NoError(t, err)
		saRepo.AssertExpectations(t)
		collRepo.AssertExpectations(t)
		permRepo.AssertExpectations(t)
	})

	t.Run("sa_not_found", func(t *testing.T) {
		saRepo, _, _, svc := setupServiceAccountService()

		tenantID := uuid.New()
		saID := uuid.New()

		saRepo.On("GetByID", mock.Anything, tenantID, saID).Return(nil, domain.ErrServiceAccountNotFound)

		err := svc.SetPermission(context.Background(), &service.SASetPermissionInput{
			TenantID:         tenantID,
			ServiceAccountID: saID,
			CollectionID:     uuid.New(),
			Permission:       domain.CollectionPermViewer,
			GrantedBy:        uuid.New(),
		})
		assert.ErrorIs(t, err, domain.ErrServiceAccountNotFound)
		saRepo.AssertExpectations(t)
	})

	t.Run("collection_not_in_tenant", func(t *testing.T) {
		saRepo, _, collRepo, svc := setupServiceAccountService()

		tenantID := uuid.New()
		saID := uuid.New()
		collectionID := uuid.New()

		saRepo.On("GetByID", mock.Anything, tenantID, saID).Return(&domain.ServiceAccount{
			ID:       saID,
			TenantID: tenantID,
		}, nil)
		collRepo.On("GetByID", mock.Anything, tenantID, collectionID).Return(nil, domain.ErrCollectionNotFound)

		err := svc.SetPermission(context.Background(), &service.SASetPermissionInput{
			TenantID:         tenantID,
			ServiceAccountID: saID,
			CollectionID:     collectionID,
			Permission:       domain.CollectionPermViewer,
			GrantedBy:        uuid.New(),
		})
		assert.ErrorIs(t, err, domain.ErrCollectionNotFound)
		saRepo.AssertExpectations(t)
		collRepo.AssertExpectations(t)
	})
}
