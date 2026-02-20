package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"

	"satvos/internal/config"
	"satvos/internal/domain"
	"satvos/internal/service"
	"satvos/mocks"
)

var testJWTCfg = config.JWTConfig{
	Secret:             "test-secret-key-for-testing-only",
	AccessTokenExpiry:  15 * time.Minute,
	RefreshTokenExpiry: 168 * time.Hour,
	Issuer:             "satvos-test",
}

func setupPasswordResetService() (
	service.PasswordResetService,
	*mocks.MockTenantRepo,
	*mocks.MockUserRepo,
	*mocks.MockEmailSender,
) {
	tenantRepo := new(mocks.MockTenantRepo)
	userRepo := new(mocks.MockUserRepo)
	emailSender := new(mocks.MockEmailSender)

	svc := service.NewPasswordResetService(tenantRepo, userRepo, emailSender, testJWTCfg)

	return svc, tenantRepo, userRepo, emailSender
}

func TestForgotPassword_Success(t *testing.T) {
	svc, tenantRepo, userRepo, emailSender := setupPasswordResetService()
	ctx := context.Background()
	tenantID := uuid.New()
	userID := uuid.New()

	tenant := &domain.Tenant{ID: tenantID, Slug: "test-tenant", IsActive: true}
	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "user@test.com",
		FullName:     "Test User",
		Role:         domain.RoleMember,
		IsActive:     true,
		PasswordHash: "$2a$12$existing-hash",
	}

	tenantRepo.On("GetBySlug", ctx, "test-tenant").Return(tenant, nil)
	userRepo.On("GetByEmail", ctx, tenantID, "user@test.com").Return(user, nil)
	userRepo.On("SetPasswordResetToken", ctx, tenantID, userID, mock.AnythingOfType("string")).Return(nil)
	emailSender.On("SendPasswordResetEmail", ctx, "user@test.com", "Test User", mock.AnythingOfType("string")).Return(nil)

	err := svc.ForgotPassword(ctx, service.ForgotPasswordInput{
		TenantSlug: "test-tenant",
		Email:      "user@test.com",
	})

	assert.NoError(t, err)
	tenantRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
	emailSender.AssertExpectations(t)
}

func TestForgotPassword_UserNotFound(t *testing.T) {
	svc, tenantRepo, userRepo, emailSender := setupPasswordResetService()
	ctx := context.Background()
	tenantID := uuid.New()

	tenant := &domain.Tenant{ID: tenantID, Slug: "test-tenant", IsActive: true}

	tenantRepo.On("GetBySlug", ctx, "test-tenant").Return(tenant, nil)
	userRepo.On("GetByEmail", ctx, tenantID, "nonexistent@test.com").Return(nil, domain.ErrNotFound)

	err := svc.ForgotPassword(ctx, service.ForgotPasswordInput{
		TenantSlug: "test-tenant",
		Email:      "nonexistent@test.com",
	})

	assert.NoError(t, err) // Returns nil — no leak
	emailSender.AssertNotCalled(t, "SendPasswordResetEmail")
}

func TestForgotPassword_TenantNotFound(t *testing.T) {
	svc, tenantRepo, _, emailSender := setupPasswordResetService()
	ctx := context.Background()

	tenantRepo.On("GetBySlug", ctx, "nonexistent").Return(nil, domain.ErrNotFound)

	err := svc.ForgotPassword(ctx, service.ForgotPasswordInput{
		TenantSlug: "nonexistent",
		Email:      "user@test.com",
	})

	assert.NoError(t, err) // Returns nil — no leak
	emailSender.AssertNotCalled(t, "SendPasswordResetEmail")
}

func TestForgotPassword_InactiveUser(t *testing.T) {
	svc, tenantRepo, userRepo, emailSender := setupPasswordResetService()
	ctx := context.Background()
	tenantID := uuid.New()

	tenant := &domain.Tenant{ID: tenantID, Slug: "test-tenant", IsActive: true}
	user := &domain.User{
		ID:       uuid.New(),
		TenantID: tenantID,
		Email:    "user@test.com",
		FullName: "Test User",
		IsActive: false,
	}

	tenantRepo.On("GetBySlug", ctx, "test-tenant").Return(tenant, nil)
	userRepo.On("GetByEmail", ctx, tenantID, "user@test.com").Return(user, nil)

	err := svc.ForgotPassword(ctx, service.ForgotPasswordInput{
		TenantSlug: "test-tenant",
		Email:      "user@test.com",
	})

	assert.NoError(t, err) // Returns nil — no leak
	emailSender.AssertNotCalled(t, "SendPasswordResetEmail")
}

func TestForgotPassword_InactiveTenant(t *testing.T) {
	svc, tenantRepo, _, emailSender := setupPasswordResetService()
	ctx := context.Background()
	tenantID := uuid.New()

	tenant := &domain.Tenant{ID: tenantID, Slug: "test-tenant", IsActive: false}

	tenantRepo.On("GetBySlug", ctx, "test-tenant").Return(tenant, nil)

	err := svc.ForgotPassword(ctx, service.ForgotPasswordInput{
		TenantSlug: "test-tenant",
		Email:      "user@test.com",
	})

	assert.NoError(t, err) // Returns nil — no leak
	emailSender.AssertNotCalled(t, "SendPasswordResetEmail")
}

func TestForgotPassword_EmailSendFailure(t *testing.T) {
	svc, tenantRepo, userRepo, emailSender := setupPasswordResetService()
	ctx := context.Background()
	tenantID := uuid.New()
	userID := uuid.New()

	tenant := &domain.Tenant{ID: tenantID, Slug: "test-tenant", IsActive: true}
	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "user@test.com",
		FullName:     "Test User",
		Role:         domain.RoleMember,
		IsActive:     true,
		PasswordHash: "$2a$12$existing-hash",
	}

	tenantRepo.On("GetBySlug", ctx, "test-tenant").Return(tenant, nil)
	userRepo.On("GetByEmail", ctx, tenantID, "user@test.com").Return(user, nil)
	userRepo.On("SetPasswordResetToken", ctx, tenantID, userID, mock.AnythingOfType("string")).Return(nil)
	emailSender.On("SendPasswordResetEmail", ctx, "user@test.com", "Test User", mock.AnythingOfType("string")).
		Return(assert.AnError)

	err := svc.ForgotPassword(ctx, service.ForgotPasswordInput{
		TenantSlug: "test-tenant",
		Email:      "user@test.com",
	})

	assert.NoError(t, err) // Logs warning but still returns nil
	emailSender.AssertExpectations(t)
}

func TestResetPassword_Success(t *testing.T) {
	tenantRepo := new(mocks.MockTenantRepo)
	userRepo := new(mocks.MockUserRepo)
	emailSender := new(mocks.MockEmailSender)
	ctx := context.Background()
	tenantID := uuid.New()
	userID := uuid.New()

	svc := service.NewPasswordResetService(tenantRepo, userRepo, emailSender, testJWTCfg)

	tenant := &domain.Tenant{ID: tenantID, Slug: "test-tenant", IsActive: true}
	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "user@test.com",
		FullName:     "Test User",
		Role:         domain.RoleMember,
		IsActive:     true,
		PasswordHash: "$2a$12$existing-hash",
	}

	tenantRepo.On("GetBySlug", ctx, "test-tenant").Return(tenant, nil)
	userRepo.On("GetByEmail", ctx, tenantID, "user@test.com").Return(user, nil)

	// Capture the token ID stored in DB and the full token from the email
	var capturedTokenID string
	var capturedToken string
	userRepo.On("SetPasswordResetToken", ctx, tenantID, userID, mock.AnythingOfType("string")).
		Run(func(args mock.Arguments) {
			capturedTokenID = args.Get(3).(string)
		}).Return(nil)
	emailSender.On("SendPasswordResetEmail", ctx, "user@test.com", "Test User", mock.AnythingOfType("string")).
		Run(func(args mock.Arguments) {
			capturedToken = args.Get(3).(string)
		}).Return(nil)

	// Trigger forgot password to get a valid token
	err := svc.ForgotPassword(ctx, service.ForgotPasswordInput{
		TenantSlug: "test-tenant",
		Email:      "user@test.com",
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, capturedToken)
	assert.NotEmpty(t, capturedTokenID)

	// Now reset password with the captured token
	userRepo.On("ResetPassword", ctx, tenantID, userID, mock.AnythingOfType("string"), capturedTokenID).Return(nil)

	err = svc.ResetPassword(ctx, service.ResetPasswordInput{
		Token:       capturedToken,
		NewPassword: "newpassword123",
	})

	assert.NoError(t, err)
	userRepo.AssertExpectations(t)
}

func TestResetPassword_InvalidToken(t *testing.T) {
	svc, _, _, _ := setupPasswordResetService()
	ctx := context.Background()

	err := svc.ResetPassword(ctx, service.ResetPasswordInput{
		Token:       "malformed-token-string",
		NewPassword: "newpassword123",
	})

	assert.ErrorIs(t, err, domain.ErrPasswordResetTokenInvalid)
}

func TestResetPassword_ExpiredToken(t *testing.T) {
	svc, _, _, _ := setupPasswordResetService()
	ctx := context.Background()

	// Create an expired token manually
	now := time.Now()
	claims := &service.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			Issuer:    testJWTCfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-1 * time.Hour)), // expired 1h ago
			ID:        uuid.New().String(),
			Audience:  jwt.ClaimStrings{"password-reset"},
		},
		TenantID: uuid.New(),
		UserID:   uuid.New(),
		Email:    "user@test.com",
		Role:     domain.RoleMember,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testJWTCfg.Secret))

	err := svc.ResetPassword(ctx, service.ResetPasswordInput{
		Token:       tokenString,
		NewPassword: "newpassword123",
	})

	assert.ErrorIs(t, err, domain.ErrPasswordResetTokenInvalid)
}

func TestResetPassword_WrongAudience(t *testing.T) {
	svc, _, _, _ := setupPasswordResetService()
	ctx := context.Background()

	// Create a token with email-verification audience
	now := time.Now()
	claims := &service.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			Issuer:    testJWTCfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
			ID:        uuid.New().String(),
			Audience:  jwt.ClaimStrings{"email-verification"},
		},
		TenantID: uuid.New(),
		UserID:   uuid.New(),
		Email:    "user@test.com",
		Role:     domain.RoleMember,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testJWTCfg.Secret))

	err := svc.ResetPassword(ctx, service.ResetPasswordInput{
		Token:       tokenString,
		NewPassword: "newpassword123",
	})

	assert.ErrorIs(t, err, domain.ErrPasswordResetTokenInvalid)
}

func TestResetPassword_TokenAlreadyUsed(t *testing.T) {
	tenantRepo := new(mocks.MockTenantRepo)
	userRepo := new(mocks.MockUserRepo)
	emailSender := new(mocks.MockEmailSender)
	ctx := context.Background()
	tenantID := uuid.New()
	userID := uuid.New()

	svc := service.NewPasswordResetService(tenantRepo, userRepo, emailSender, testJWTCfg)

	tenant := &domain.Tenant{ID: tenantID, Slug: "test-tenant", IsActive: true}
	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "user@test.com",
		FullName:     "Test User",
		Role:         domain.RoleMember,
		IsActive:     true,
		PasswordHash: "$2a$12$existing-hash",
	}

	tenantRepo.On("GetBySlug", ctx, "test-tenant").Return(tenant, nil)
	userRepo.On("GetByEmail", ctx, tenantID, "user@test.com").Return(user, nil)

	var capturedTokenID string
	var capturedToken string
	userRepo.On("SetPasswordResetToken", ctx, tenantID, userID, mock.AnythingOfType("string")).
		Run(func(args mock.Arguments) {
			capturedTokenID = args.Get(3).(string)
		}).Return(nil)
	emailSender.On("SendPasswordResetEmail", ctx, "user@test.com", "Test User", mock.AnythingOfType("string")).
		Run(func(args mock.Arguments) {
			capturedToken = args.Get(3).(string)
		}).Return(nil)

	_ = svc.ForgotPassword(ctx, service.ForgotPasswordInput{
		TenantSlug: "test-tenant",
		Email:      "user@test.com",
	})

	// First reset succeeds
	userRepo.On("ResetPassword", ctx, tenantID, userID, mock.AnythingOfType("string"), capturedTokenID).
		Return(nil).Once()

	err := svc.ResetPassword(ctx, service.ResetPasswordInput{
		Token:       capturedToken,
		NewPassword: "newpassword123",
	})
	assert.NoError(t, err)

	// Second reset with same token fails (token_id cleared in DB)
	userRepo.On("ResetPassword", ctx, tenantID, userID, mock.AnythingOfType("string"), capturedTokenID).
		Return(domain.ErrPasswordResetTokenInvalid)

	err = svc.ResetPassword(ctx, service.ResetPasswordInput{
		Token:       capturedToken,
		NewPassword: "anotherpassword",
	})
	assert.ErrorIs(t, err, domain.ErrPasswordResetTokenInvalid)
}

func TestResetPassword_NewTokenInvalidatesOld(t *testing.T) {
	tenantRepo := new(mocks.MockTenantRepo)
	userRepo := new(mocks.MockUserRepo)
	emailSender := new(mocks.MockEmailSender)
	ctx := context.Background()
	tenantID := uuid.New()
	userID := uuid.New()

	svc := service.NewPasswordResetService(tenantRepo, userRepo, emailSender, testJWTCfg)

	tenant := &domain.Tenant{ID: tenantID, Slug: "test-tenant", IsActive: true}
	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "user@test.com",
		FullName:     "Test User",
		Role:         domain.RoleMember,
		IsActive:     true,
		PasswordHash: "$2a$12$existing-hash",
	}

	tenantRepo.On("GetBySlug", ctx, "test-tenant").Return(tenant, nil)
	userRepo.On("GetByEmail", ctx, tenantID, "user@test.com").Return(user, nil)

	// Capture tokens from two forgot-password calls
	var firstToken, secondToken string
	var firstTokenID, secondTokenID string
	callCount := 0
	userRepo.On("SetPasswordResetToken", ctx, tenantID, userID, mock.AnythingOfType("string")).
		Run(func(args mock.Arguments) {
			callCount++
			if callCount == 1 {
				firstTokenID = args.Get(3).(string)
			} else {
				secondTokenID = args.Get(3).(string)
			}
		}).Return(nil)

	emailCallCount := 0
	emailSender.On("SendPasswordResetEmail", ctx, "user@test.com", "Test User", mock.AnythingOfType("string")).
		Run(func(args mock.Arguments) {
			emailCallCount++
			if emailCallCount == 1 {
				firstToken = args.Get(3).(string)
			} else {
				secondToken = args.Get(3).(string)
			}
		}).Return(nil)

	input := service.ForgotPasswordInput{
		TenantSlug: "test-tenant",
		Email:      "user@test.com",
	}

	_ = svc.ForgotPassword(ctx, input)
	_ = svc.ForgotPassword(ctx, input)

	assert.NotEqual(t, firstTokenID, secondTokenID)

	// Old token (first) should fail because DB now has the second token ID
	userRepo.On("ResetPassword", ctx, tenantID, userID, mock.AnythingOfType("string"), firstTokenID).
		Return(domain.ErrPasswordResetTokenInvalid)

	err := svc.ResetPassword(ctx, service.ResetPasswordInput{
		Token:       firstToken,
		NewPassword: "newpassword123",
	})
	assert.ErrorIs(t, err, domain.ErrPasswordResetTokenInvalid)

	// New token (second) should succeed
	userRepo.On("ResetPassword", ctx, tenantID, userID, mock.AnythingOfType("string"), secondTokenID).
		Return(nil)

	err = svc.ResetPassword(ctx, service.ResetPasswordInput{
		Token:       secondToken,
		NewPassword: "newpassword123",
	})
	assert.NoError(t, err)
}

func TestResetPassword_VerifyBcryptHash(t *testing.T) {
	tenantRepo := new(mocks.MockTenantRepo)
	userRepo := new(mocks.MockUserRepo)
	emailSender := new(mocks.MockEmailSender)
	ctx := context.Background()
	tenantID := uuid.New()
	userID := uuid.New()

	svc := service.NewPasswordResetService(tenantRepo, userRepo, emailSender, testJWTCfg)

	tenant := &domain.Tenant{ID: tenantID, Slug: "test-tenant", IsActive: true}
	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "user@test.com",
		FullName:     "Test User",
		Role:         domain.RoleMember,
		IsActive:     true,
		PasswordHash: "$2a$12$existing-hash",
	}

	tenantRepo.On("GetBySlug", ctx, "test-tenant").Return(tenant, nil)
	userRepo.On("GetByEmail", ctx, tenantID, "user@test.com").Return(user, nil)

	var capturedTokenID string
	var capturedToken string
	userRepo.On("SetPasswordResetToken", ctx, tenantID, userID, mock.AnythingOfType("string")).
		Run(func(args mock.Arguments) {
			capturedTokenID = args.Get(3).(string)
		}).Return(nil)
	emailSender.On("SendPasswordResetEmail", ctx, "user@test.com", "Test User", mock.AnythingOfType("string")).
		Run(func(args mock.Arguments) {
			capturedToken = args.Get(3).(string)
		}).Return(nil)

	_ = svc.ForgotPassword(ctx, service.ForgotPasswordInput{
		TenantSlug: "test-tenant",
		Email:      "user@test.com",
	})

	// Capture the password hash that's passed to ResetPassword
	var capturedHash string
	userRepo.On("ResetPassword", ctx, tenantID, userID, mock.AnythingOfType("string"), capturedTokenID).
		Run(func(args mock.Arguments) {
			capturedHash = args.Get(3).(string)
		}).Return(nil)

	newPassword := "mysecurenewpassword"
	err := svc.ResetPassword(ctx, service.ResetPasswordInput{
		Token:       capturedToken,
		NewPassword: newPassword,
	})
	assert.NoError(t, err)

	// Verify the hash is valid bcrypt of the new password
	assert.NotEmpty(t, capturedHash)
	err = bcrypt.CompareHashAndPassword([]byte(capturedHash), []byte(newPassword))
	assert.NoError(t, err)
}
