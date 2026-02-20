package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"

	"satvos/internal/config"
	"satvos/internal/domain"
	"satvos/internal/service"
	"satvos/mocks"
)

func testJWTConfig() config.JWTConfig {
	return config.JWTConfig{
		Secret:             "test-secret-key-for-unit-tests",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 168 * time.Hour,
		Issuer:             "satvos-test",
	}
}

func hashPassword(password string) string {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(hash)
}

func TestAuthService_Login_Success(t *testing.T) {
	tenantRepo := new(mocks.MockTenantRepo)
	userRepo := new(mocks.MockUserRepo)
	cfg := testJWTConfig()
	svc := service.NewAuthService(userRepo, tenantRepo, cfg)

	tenantID := uuid.New()
	userID := uuid.New()
	tenant := &domain.Tenant{
		ID:       tenantID,
		Name:     "Test Tenant",
		Slug:     "test-tenant",
		IsActive: true,
	}
	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "user@test.com",
		PasswordHash: hashPassword("password123"),
		FullName:     "Test User",
		Role:         domain.RoleMember,
		IsActive:     true,
	}

	tenantRepo.On("GetBySlug", mock.Anything, "test-tenant").Return(tenant, nil)
	userRepo.On("GetByEmail", mock.Anything, tenantID, "user@test.com").Return(user, nil)

	result, err := svc.Login(context.Background(), service.LoginInput{
		TenantSlug: "test-tenant",
		Email:      "user@test.com",
		Password:   "password123",
	})

	assert.NoError(t, err)
	assert.NotEmpty(t, result.AccessToken)
	assert.NotEmpty(t, result.RefreshToken)
	assert.True(t, result.ExpiresAt.After(time.Now()))

	tenantRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestAuthService_Login_InvalidPassword(t *testing.T) {
	tenantRepo := new(mocks.MockTenantRepo)
	userRepo := new(mocks.MockUserRepo)
	cfg := testJWTConfig()
	svc := service.NewAuthService(userRepo, tenantRepo, cfg)

	tenantID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "test-tenant", IsActive: true}
	user := &domain.User{
		ID:           uuid.New(),
		TenantID:     tenantID,
		Email:        "user@test.com",
		PasswordHash: hashPassword("correct-password"),
		IsActive:     true,
	}

	tenantRepo.On("GetBySlug", mock.Anything, "test-tenant").Return(tenant, nil)
	userRepo.On("GetByEmail", mock.Anything, tenantID, "user@test.com").Return(user, nil)

	result, err := svc.Login(context.Background(), service.LoginInput{
		TenantSlug: "test-tenant",
		Email:      "user@test.com",
		Password:   "wrong-password",
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
}

func TestAuthService_Login_TenantNotFound(t *testing.T) {
	tenantRepo := new(mocks.MockTenantRepo)
	userRepo := new(mocks.MockUserRepo)
	cfg := testJWTConfig()
	svc := service.NewAuthService(userRepo, tenantRepo, cfg)

	tenantRepo.On("GetBySlug", mock.Anything, "nonexistent").Return(nil, domain.ErrNotFound)

	result, err := svc.Login(context.Background(), service.LoginInput{
		TenantSlug: "nonexistent",
		Email:      "user@test.com",
		Password:   "password123",
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
}

func TestAuthService_Login_InactiveTenant(t *testing.T) {
	tenantRepo := new(mocks.MockTenantRepo)
	userRepo := new(mocks.MockUserRepo)
	cfg := testJWTConfig()
	svc := service.NewAuthService(userRepo, tenantRepo, cfg)

	tenant := &domain.Tenant{ID: uuid.New(), Slug: "inactive", IsActive: false}
	tenantRepo.On("GetBySlug", mock.Anything, "inactive").Return(tenant, nil)

	result, err := svc.Login(context.Background(), service.LoginInput{
		TenantSlug: "inactive",
		Email:      "user@test.com",
		Password:   "password123",
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrTenantInactive)
}

func TestAuthService_Login_InactiveUser(t *testing.T) {
	tenantRepo := new(mocks.MockTenantRepo)
	userRepo := new(mocks.MockUserRepo)
	cfg := testJWTConfig()
	svc := service.NewAuthService(userRepo, tenantRepo, cfg)

	tenantID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "test-tenant", IsActive: true}
	user := &domain.User{
		ID:           uuid.New(),
		TenantID:     tenantID,
		Email:        "user@test.com",
		PasswordHash: hashPassword("password123"),
		IsActive:     false,
	}

	tenantRepo.On("GetBySlug", mock.Anything, "test-tenant").Return(tenant, nil)
	userRepo.On("GetByEmail", mock.Anything, tenantID, "user@test.com").Return(user, nil)

	result, err := svc.Login(context.Background(), service.LoginInput{
		TenantSlug: "test-tenant",
		Email:      "user@test.com",
		Password:   "password123",
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrUserInactive)
}

func TestAuthService_Login_UserNotFound(t *testing.T) {
	tenantRepo := new(mocks.MockTenantRepo)
	userRepo := new(mocks.MockUserRepo)
	cfg := testJWTConfig()
	svc := service.NewAuthService(userRepo, tenantRepo, cfg)

	tenantID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "test-tenant", IsActive: true}

	tenantRepo.On("GetBySlug", mock.Anything, "test-tenant").Return(tenant, nil)
	userRepo.On("GetByEmail", mock.Anything, tenantID, "nobody@test.com").Return(nil, domain.ErrNotFound)

	result, err := svc.Login(context.Background(), service.LoginInput{
		TenantSlug: "test-tenant",
		Email:      "nobody@test.com",
		Password:   "password123",
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
}

func TestAuthService_ValidateToken_Success(t *testing.T) {
	tenantRepo := new(mocks.MockTenantRepo)
	userRepo := new(mocks.MockUserRepo)
	cfg := testJWTConfig()
	svc := service.NewAuthService(userRepo, tenantRepo, cfg)

	tenantID := uuid.New()
	userID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "test-tenant", IsActive: true}
	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "user@test.com",
		PasswordHash: hashPassword("password123"),
		FullName:     "Test User",
		Role:         domain.RoleMember,
		IsActive:     true,
	}

	tenantRepo.On("GetBySlug", mock.Anything, "test-tenant").Return(tenant, nil)
	userRepo.On("GetByEmail", mock.Anything, tenantID, "user@test.com").Return(user, nil)

	tokenPair, err := svc.Login(context.Background(), service.LoginInput{
		TenantSlug: "test-tenant",
		Email:      "user@test.com",
		Password:   "password123",
	})
	assert.NoError(t, err)

	claims, err := svc.ValidateToken(tokenPair.AccessToken)
	assert.NoError(t, err)
	assert.Equal(t, tenantID, claims.TenantID)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, "user@test.com", claims.Email)
	assert.Equal(t, domain.RoleMember, claims.Role)
}

func TestAuthService_ValidateToken_InvalidSignature(t *testing.T) {
	tenantRepo := new(mocks.MockTenantRepo)
	userRepo := new(mocks.MockUserRepo)
	cfg := testJWTConfig()
	svc := service.NewAuthService(userRepo, tenantRepo, cfg)

	claims, err := svc.ValidateToken("invalid.token.string")
	assert.Nil(t, claims)
	assert.Error(t, err)
}

func TestAuthService_Login_OAuthOnlyUser(t *testing.T) {
	tenantRepo := new(mocks.MockTenantRepo)
	userRepo := new(mocks.MockUserRepo)
	cfg := testJWTConfig()
	svc := service.NewAuthService(userRepo, tenantRepo, cfg)

	tenantID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "test-tenant", IsActive: true}
	user := &domain.User{
		ID:           uuid.New(),
		TenantID:     tenantID,
		Email:        "oauth@test.com",
		PasswordHash: "", // OAuth-only — no password
		AuthProvider: domain.AuthProviderGoogle,
		IsActive:     true,
	}

	tenantRepo.On("GetBySlug", mock.Anything, "test-tenant").Return(tenant, nil)
	userRepo.On("GetByEmail", mock.Anything, tenantID, "oauth@test.com").Return(user, nil)

	result, err := svc.Login(context.Background(), service.LoginInput{
		TenantSlug: "test-tenant",
		Email:      "oauth@test.com",
		Password:   "anypassword",
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrPasswordLoginNotAllowed)
}

func TestAuthService_Login_InvitedUser(t *testing.T) {
	tenantRepo := new(mocks.MockTenantRepo)
	userRepo := new(mocks.MockUserRepo)
	cfg := testJWTConfig()
	svc := service.NewAuthService(userRepo, tenantRepo, cfg)

	tenantID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "test-tenant", IsActive: true}
	user := &domain.User{
		ID:           uuid.New(),
		TenantID:     tenantID,
		Email:        "invited@test.com",
		PasswordHash: "", // Invited — no password yet
		IsActive:     true,
	}

	tenantRepo.On("GetBySlug", mock.Anything, "test-tenant").Return(tenant, nil)
	userRepo.On("GetByEmail", mock.Anything, tenantID, "invited@test.com").Return(user, nil)

	result, err := svc.Login(context.Background(), service.LoginInput{
		TenantSlug: "test-tenant",
		Email:      "invited@test.com",
		Password:   "anypassword",
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrInvitationPending)
}

func TestAuthService_RefreshToken_Success(t *testing.T) {
	tenantRepo := new(mocks.MockTenantRepo)
	userRepo := new(mocks.MockUserRepo)
	cfg := testJWTConfig()
	svc := service.NewAuthService(userRepo, tenantRepo, cfg)

	tenantID := uuid.New()
	userID := uuid.New()
	tenant := &domain.Tenant{ID: tenantID, Slug: "test-tenant", IsActive: true}
	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "user@test.com",
		PasswordHash: hashPassword("password123"),
		FullName:     "Test User",
		Role:         domain.RoleMember,
		IsActive:     true,
	}

	tenantRepo.On("GetBySlug", mock.Anything, "test-tenant").Return(tenant, nil)
	userRepo.On("GetByEmail", mock.Anything, tenantID, "user@test.com").Return(user, nil)
	userRepo.On("GetByID", mock.Anything, tenantID, userID).Return(user, nil)

	tokenPair, err := svc.Login(context.Background(), service.LoginInput{
		TenantSlug: "test-tenant",
		Email:      "user@test.com",
		Password:   "password123",
	})
	assert.NoError(t, err)

	newTokenPair, err := svc.RefreshToken(context.Background(), tokenPair.RefreshToken)
	assert.NoError(t, err)
	assert.NotEmpty(t, newTokenPair.AccessToken)
	assert.NotEmpty(t, newTokenPair.RefreshToken)
	assert.NotEqual(t, tokenPair.AccessToken, newTokenPair.AccessToken)
}
