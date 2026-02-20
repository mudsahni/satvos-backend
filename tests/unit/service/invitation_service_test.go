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

var invitationJWTCfg = config.JWTConfig{
	Secret:             "test-secret-key-for-testing-only",
	AccessTokenExpiry:  15 * time.Minute,
	RefreshTokenExpiry: 168 * time.Hour,
	Issuer:             "satvos-test",
}

func setupInvitationService() (
	service.InvitationService,
	*mocks.MockUserRepo,
	*mocks.MockEmailSender,
	*mocks.MockAuthService,
) {
	userRepo := new(mocks.MockUserRepo)
	emailSender := new(mocks.MockEmailSender)
	authSvc := new(mocks.MockAuthService)

	svc := service.NewInvitationService(userRepo, emailSender, authSvc, invitationJWTCfg)

	return svc, userRepo, emailSender, authSvc
}

func TestInvitationService_SendInvitation(t *testing.T) {
	svc, userRepo, emailSender, _ := setupInvitationService()
	ctx := context.Background()

	tenantID := uuid.New()
	userID := uuid.New()

	user := &domain.User{
		ID:       userID,
		TenantID: tenantID,
		Email:    "newuser@test.com",
		FullName: "New User",
		Role:     domain.RoleMember,
	}

	userRepo.On("SetPasswordResetToken", ctx, tenantID, userID, mock.AnythingOfType("string")).Return(nil)
	emailSender.On("SendInvitationEmail", ctx, "newuser@test.com", "New User", "member", mock.AnythingOfType("string")).Return(nil)

	err := svc.SendInvitation(ctx, user)

	assert.NoError(t, err)
	userRepo.AssertExpectations(t)
	emailSender.AssertExpectations(t)
}

func TestInvitationService_SendInvitation_EmailFailureNonBlocking(t *testing.T) {
	svc, userRepo, emailSender, _ := setupInvitationService()
	ctx := context.Background()

	tenantID := uuid.New()
	userID := uuid.New()

	user := &domain.User{
		ID:       userID,
		TenantID: tenantID,
		Email:    "newuser@test.com",
		FullName: "New User",
		Role:     domain.RoleMember,
	}

	userRepo.On("SetPasswordResetToken", ctx, tenantID, userID, mock.AnythingOfType("string")).Return(nil)
	emailSender.On("SendInvitationEmail", ctx, "newuser@test.com", "New User", "member", mock.AnythingOfType("string")).
		Return(assert.AnError)

	err := svc.SendInvitation(ctx, user)

	assert.NoError(t, err) // Email failure is non-blocking
	userRepo.AssertExpectations(t)
	emailSender.AssertExpectations(t)
}

func TestInvitationService_AcceptInvitation_HappyPath(t *testing.T) {
	svc, userRepo, emailSender, authSvc := setupInvitationService()
	ctx := context.Background()

	tenantID := uuid.New()
	userID := uuid.New()

	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "newuser@test.com",
		FullName:     "New User",
		Role:         domain.RoleMember,
		IsActive:     true,
		PasswordHash: "", // Not yet accepted
	}

	// First: send invitation to get a valid token
	var capturedTokenID string
	var capturedToken string
	userRepo.On("SetPasswordResetToken", ctx, tenantID, userID, mock.AnythingOfType("string")).
		Run(func(args mock.Arguments) {
			capturedTokenID = args.Get(3).(string)
		}).Return(nil)
	emailSender.On("SendInvitationEmail", ctx, "newuser@test.com", "New User", "member", mock.AnythingOfType("string")).
		Run(func(args mock.Arguments) {
			capturedToken = args.Get(4).(string)
		}).Return(nil)

	err := svc.SendInvitation(ctx, user)
	assert.NoError(t, err)
	assert.NotEmpty(t, capturedToken)
	assert.NotEmpty(t, capturedTokenID)

	// Now: accept invitation
	userRepo.On("GetByID", ctx, tenantID, userID).Return(user, nil).Once()
	userRepo.On("AcceptInvitation", ctx, tenantID, userID, mock.AnythingOfType("string"), capturedTokenID).Return(nil)

	acceptedUser := &domain.User{
		ID:            userID,
		TenantID:      tenantID,
		Email:         "newuser@test.com",
		FullName:      "New User",
		Role:          domain.RoleMember,
		IsActive:      true,
		PasswordHash:  "$2a$12$...",
		EmailVerified: true,
	}
	userRepo.On("GetByID", ctx, tenantID, userID).Return(acceptedUser, nil).Once()

	tokenPair := &service.TokenPair{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(15 * time.Minute),
	}
	authSvc.On("GenerateTokenPairForUser", acceptedUser).Return(tokenPair, nil)

	output, err := svc.AcceptInvitation(ctx, service.AcceptInvitationInput{
		Token:    capturedToken,
		Password: "newpassword123",
	})

	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.Equal(t, acceptedUser, output.User)
	assert.Equal(t, tokenPair, output.Tokens)
	userRepo.AssertExpectations(t)
	authSvc.AssertExpectations(t)
}

func TestInvitationService_AcceptInvitation_AlreadyAccepted(t *testing.T) {
	svc, userRepo, emailSender, _ := setupInvitationService()
	ctx := context.Background()

	tenantID := uuid.New()
	userID := uuid.New()

	// First: create user and send invitation
	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "user@test.com",
		FullName:     "Test User",
		Role:         domain.RoleMember,
		IsActive:     true,
		PasswordHash: "", // pending
	}

	var capturedToken string
	userRepo.On("SetPasswordResetToken", ctx, tenantID, userID, mock.AnythingOfType("string")).Return(nil)
	emailSender.On("SendInvitationEmail", ctx, "user@test.com", "Test User", "member", mock.AnythingOfType("string")).
		Run(func(args mock.Arguments) {
			capturedToken = args.Get(4).(string)
		}).Return(nil)

	_ = svc.SendInvitation(ctx, user)

	// User already has password — accepted
	acceptedUser := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "user@test.com",
		FullName:     "Test User",
		Role:         domain.RoleMember,
		IsActive:     true,
		PasswordHash: "$2a$12$already-accepted",
	}
	userRepo.On("GetByID", ctx, tenantID, userID).Return(acceptedUser, nil)

	output, err := svc.AcceptInvitation(ctx, service.AcceptInvitationInput{
		Token:    capturedToken,
		Password: "anotherpassword",
	})

	assert.Nil(t, output)
	assert.ErrorIs(t, err, domain.ErrInvitationTokenInvalid)
}

func TestInvitationService_AcceptInvitation_Inactive(t *testing.T) {
	svc, userRepo, emailSender, _ := setupInvitationService()
	ctx := context.Background()

	tenantID := uuid.New()
	userID := uuid.New()

	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "user@test.com",
		FullName:     "Test User",
		Role:         domain.RoleMember,
		IsActive:     true, // active when invitation was sent
		PasswordHash: "",
	}

	var capturedToken string
	userRepo.On("SetPasswordResetToken", ctx, tenantID, userID, mock.AnythingOfType("string")).Return(nil)
	emailSender.On("SendInvitationEmail", ctx, "user@test.com", "Test User", "member", mock.AnythingOfType("string")).
		Run(func(args mock.Arguments) {
			capturedToken = args.Get(4).(string)
		}).Return(nil)

	_ = svc.SendInvitation(ctx, user)

	// User deactivated before accepting
	deactivatedUser := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "user@test.com",
		FullName:     "Test User",
		Role:         domain.RoleMember,
		IsActive:     false,
		PasswordHash: "",
	}
	userRepo.On("GetByID", ctx, tenantID, userID).Return(deactivatedUser, nil)

	output, err := svc.AcceptInvitation(ctx, service.AcceptInvitationInput{
		Token:    capturedToken,
		Password: "newpassword123",
	})

	assert.Nil(t, output)
	assert.ErrorIs(t, err, domain.ErrUserInactive)
}

func TestInvitationService_AcceptInvitation_InvalidToken(t *testing.T) {
	svc, _, _, _ := setupInvitationService()
	ctx := context.Background()

	output, err := svc.AcceptInvitation(ctx, service.AcceptInvitationInput{
		Token:    "malformed-token-string",
		Password: "newpassword123",
	})

	assert.Nil(t, output)
	assert.ErrorIs(t, err, domain.ErrInvitationTokenInvalid)
}

func TestInvitationService_AcceptInvitation_ExpiredToken(t *testing.T) {
	svc, _, _, _ := setupInvitationService()
	ctx := context.Background()

	// Create an expired invitation token
	now := time.Now()
	claims := &service.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			Issuer:    invitationJWTCfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(now.Add(-96 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-24 * time.Hour)), // expired
			ID:        uuid.New().String(),
			Audience:  jwt.ClaimStrings{"invitation"},
		},
		TenantID: uuid.New(),
		UserID:   uuid.New(),
		Email:    "user@test.com",
		Role:     domain.RoleMember,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(invitationJWTCfg.Secret))

	output, err := svc.AcceptInvitation(ctx, service.AcceptInvitationInput{
		Token:    tokenString,
		Password: "newpassword123",
	})

	assert.Nil(t, output)
	assert.ErrorIs(t, err, domain.ErrInvitationTokenInvalid)
}

func TestInvitationService_AcceptInvitation_WrongAudience(t *testing.T) {
	svc, _, _, _ := setupInvitationService()
	ctx := context.Background()

	// Create a token with password-reset audience (not invitation)
	now := time.Now()
	claims := &service.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			Issuer:    invitationJWTCfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(72 * time.Hour)),
			ID:        uuid.New().String(),
			Audience:  jwt.ClaimStrings{"password-reset"},
		},
		TenantID: uuid.New(),
		UserID:   uuid.New(),
		Email:    "user@test.com",
		Role:     domain.RoleMember,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(invitationJWTCfg.Secret))

	output, err := svc.AcceptInvitation(ctx, service.AcceptInvitationInput{
		Token:    tokenString,
		Password: "newpassword123",
	})

	assert.Nil(t, output)
	assert.ErrorIs(t, err, domain.ErrInvitationTokenInvalid)
}

func TestInvitationService_AcceptInvitation_VerifyBcryptHash(t *testing.T) {
	svc, userRepo, emailSender, authSvc := setupInvitationService()
	ctx := context.Background()

	tenantID := uuid.New()
	userID := uuid.New()

	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "user@test.com",
		FullName:     "Test User",
		Role:         domain.RoleMember,
		IsActive:     true,
		PasswordHash: "",
	}

	var capturedTokenID string
	var capturedToken string
	userRepo.On("SetPasswordResetToken", ctx, tenantID, userID, mock.AnythingOfType("string")).
		Run(func(args mock.Arguments) {
			capturedTokenID = args.Get(3).(string)
		}).Return(nil)
	emailSender.On("SendInvitationEmail", ctx, "user@test.com", "Test User", "member", mock.AnythingOfType("string")).
		Run(func(args mock.Arguments) {
			capturedToken = args.Get(4).(string)
		}).Return(nil)

	_ = svc.SendInvitation(ctx, user)

	userRepo.On("GetByID", ctx, tenantID, userID).Return(user, nil)

	var capturedHash string
	userRepo.On("AcceptInvitation", ctx, tenantID, userID, mock.AnythingOfType("string"), capturedTokenID).
		Run(func(args mock.Arguments) {
			capturedHash = args.Get(3).(string)
		}).Return(nil)

	acceptedUser := &domain.User{
		ID:            userID,
		TenantID:      tenantID,
		Email:         "user@test.com",
		FullName:      "Test User",
		Role:          domain.RoleMember,
		IsActive:      true,
		PasswordHash:  "hashed",
		EmailVerified: true,
	}

	// Override second GetByID call
	userRepo.ExpectedCalls = append(userRepo.ExpectedCalls[:0], userRepo.ExpectedCalls...)
	// Re-set: first GetByID returns pending user, second returns accepted
	userRepo.ExpectedCalls = nil
	userRepo.On("SetPasswordResetToken", ctx, tenantID, userID, mock.AnythingOfType("string")).Return(nil)
	userRepo.On("GetByID", ctx, tenantID, userID).Return(user, nil).Once()
	userRepo.On("AcceptInvitation", ctx, tenantID, userID, mock.AnythingOfType("string"), capturedTokenID).
		Run(func(args mock.Arguments) {
			capturedHash = args.Get(3).(string)
		}).Return(nil)
	userRepo.On("GetByID", ctx, tenantID, userID).Return(acceptedUser, nil).Once()

	tokenPair := &service.TokenPair{
		AccessToken:  "at",
		RefreshToken: "rt",
		ExpiresAt:    time.Now().Add(15 * time.Minute),
	}
	authSvc.On("GenerateTokenPairForUser", acceptedUser).Return(tokenPair, nil)

	newPassword := "mynewsecurepassword"
	output, err := svc.AcceptInvitation(ctx, service.AcceptInvitationInput{
		Token:    capturedToken,
		Password: newPassword,
	})

	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.NotEmpty(t, capturedHash)
	err = bcrypt.CompareHashAndPassword([]byte(capturedHash), []byte(newPassword))
	assert.NoError(t, err)
}

func TestInvitationService_ResendInvitation(t *testing.T) {
	svc, userRepo, emailSender, _ := setupInvitationService()
	ctx := context.Background()

	tenantID := uuid.New()
	userID := uuid.New()

	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "pending@test.com",
		FullName:     "Pending User",
		Role:         domain.RoleMember,
		IsActive:     true,
		PasswordHash: "", // still pending
	}

	userRepo.On("GetByID", ctx, tenantID, userID).Return(user, nil)
	userRepo.On("SetPasswordResetToken", ctx, tenantID, userID, mock.AnythingOfType("string")).Return(nil)
	emailSender.On("SendInvitationEmail", ctx, "pending@test.com", "Pending User", "member", mock.AnythingOfType("string")).Return(nil)

	err := svc.ResendInvitation(ctx, tenantID, userID)

	assert.NoError(t, err)
	userRepo.AssertExpectations(t)
	emailSender.AssertExpectations(t)
}

func TestInvitationService_ResendInvitation_AlreadyAccepted(t *testing.T) {
	svc, userRepo, _, _ := setupInvitationService()
	ctx := context.Background()

	tenantID := uuid.New()
	userID := uuid.New()

	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "accepted@test.com",
		FullName:     "Accepted User",
		Role:         domain.RoleMember,
		IsActive:     true,
		PasswordHash: "$2a$12$hash", // already accepted
	}

	userRepo.On("GetByID", ctx, tenantID, userID).Return(user, nil)

	err := svc.ResendInvitation(ctx, tenantID, userID)

	assert.ErrorIs(t, err, domain.ErrInvitationTokenInvalid)
}

func TestInvitationService_ResendInvitation_GoogleUser(t *testing.T) {
	svc, userRepo, _, _ := setupInvitationService()
	ctx := context.Background()

	tenantID := uuid.New()
	userID := uuid.New()

	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        "google@test.com",
		FullName:     "Google User",
		Role:         domain.RoleFree,
		IsActive:     true,
		PasswordHash: "",
		AuthProvider: domain.AuthProviderGoogle,
	}

	userRepo.On("GetByID", ctx, tenantID, userID).Return(user, nil)

	err := svc.ResendInvitation(ctx, tenantID, userID)

	assert.ErrorIs(t, err, domain.ErrInvitationTokenInvalid)
}
