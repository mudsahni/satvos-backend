package service

import (
	"context"
	"errors"
	"fmt"
	"log"

	"satvos/internal/config"
	"satvos/internal/domain"
	"satvos/internal/port"
)

// SocialLoginInput is the DTO for social login requests.
type SocialLoginInput struct {
	Provider string `json:"provider" binding:"required"`
	IDToken  string `json:"id_token" binding:"required"`
}

// SocialLoginOutput contains the results of a social login.
type SocialLoginOutput struct {
	User       *domain.User       `json:"user"`
	Collection *domain.Collection `json:"collection,omitempty"`
	Tokens     *TokenPair         `json:"tokens"`
	IsNewUser  bool               `json:"is_new_user"`
}

// SocialAuthService defines the social authentication contract.
type SocialAuthService interface {
	SocialLogin(ctx context.Context, input SocialLoginInput) (*SocialLoginOutput, error)
}

type socialAuthService struct {
	verifiers   map[string]port.SocialTokenVerifier
	tenantRepo  port.TenantRepository
	userRepo    port.UserRepository
	collRepo    port.CollectionRepository
	permRepo    port.CollectionPermissionRepository
	authSvc     AuthService
	freeTierCfg config.FreeTierConfig
}

// NewSocialAuthService creates a new SocialAuthService.
func NewSocialAuthService(
	verifiers map[string]port.SocialTokenVerifier,
	tenantRepo port.TenantRepository,
	userRepo port.UserRepository,
	collRepo port.CollectionRepository,
	permRepo port.CollectionPermissionRepository,
	authSvc AuthService,
	freeTierCfg config.FreeTierConfig,
) SocialAuthService {
	return &socialAuthService{
		verifiers:   verifiers,
		tenantRepo:  tenantRepo,
		userRepo:    userRepo,
		collRepo:    collRepo,
		permRepo:    permRepo,
		authSvc:     authSvc,
		freeTierCfg: freeTierCfg,
	}
}

func (s *socialAuthService) SocialLogin(ctx context.Context, input SocialLoginInput) (*SocialLoginOutput, error) {
	// 1. Look up verifier for provider
	verifier, ok := s.verifiers[input.Provider]
	if !ok {
		return nil, fmt.Errorf("unsupported social auth provider: %s", input.Provider)
	}

	// 2. Verify ID token
	claims, err := verifier.VerifyIDToken(ctx, input.IDToken)
	if err != nil {
		return nil, domain.ErrSocialAuthTokenInvalid
	}

	// 3. Reject if email not verified by provider
	if !claims.EmailVerified {
		return nil, fmt.Errorf("email not verified by %s", input.Provider)
	}

	// 4. Look up free-tier tenant
	tenant, err := s.tenantRepo.GetBySlug(ctx, s.freeTierCfg.TenantSlug)
	if err != nil {
		return nil, fmt.Errorf("looking up free tier tenant: %w", err)
	}
	if !tenant.IsActive {
		return nil, domain.ErrTenantInactive
	}

	provider := domain.AuthProvider(input.Provider)

	// 5. Try GetByProviderID — returning user
	existingUser, err := s.userRepo.GetByProviderID(ctx, tenant.ID, provider, claims.Subject)
	if err == nil {
		// Returning user
		if !existingUser.IsActive {
			return nil, domain.ErrUserInactive
		}
		tokens, tokenErr := s.authSvc.GenerateTokenPairForUser(existingUser)
		if tokenErr != nil {
			return nil, fmt.Errorf("generating tokens: %w", tokenErr)
		}
		return &SocialLoginOutput{
			User:      existingUser,
			Tokens:    tokens,
			IsNewUser: false,
		}, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("looking up provider user: %w", err)
	}

	// 6. Try GetByEmail — auto-link existing email user
	existingUser, err = s.userRepo.GetByEmail(ctx, tenant.ID, claims.Email)
	if err == nil {
		// Existing email user — link provider
		if !existingUser.IsActive {
			return nil, domain.ErrUserInactive
		}
		if linkErr := s.userRepo.LinkProvider(ctx, tenant.ID, existingUser.ID, provider, claims.Subject); linkErr != nil {
			return nil, fmt.Errorf("linking provider: %w", linkErr)
		}
		// Set email verified if not already
		if !existingUser.EmailVerified {
			if verifyErr := s.userRepo.SetEmailVerified(ctx, tenant.ID, existingUser.ID); verifyErr != nil {
				log.Printf("WARNING: failed to set email verified for linked user %s: %v", existingUser.ID, verifyErr)
			}
		}
		tokens, tokenErr := s.authSvc.GenerateTokenPairForUser(existingUser)
		if tokenErr != nil {
			return nil, fmt.Errorf("generating tokens: %w", tokenErr)
		}
		return &SocialLoginOutput{
			User:      existingUser,
			Tokens:    tokens,
			IsNewUser: false,
		}, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("looking up email user: %w", err)
	}

	// 7. New user — create account
	sub := claims.Subject
	user := &domain.User{
		TenantID:             tenant.ID,
		Email:                claims.Email,
		PasswordHash:         "",
		FullName:             claims.FullName,
		Role:                 domain.RoleFree,
		IsActive:             true,
		MonthlyDocumentLimit: s.freeTierCfg.MonthlyLimit,
		EmailVerified:        true,
		AuthProvider:         provider,
		ProviderUserID:       &sub,
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err // ErrDuplicateEmail propagates naturally
	}

	// Create personal collection
	collection, err := CreatePersonalCollection(ctx, s.collRepo, s.permRepo, tenant.ID, user.ID, claims.FullName)
	if err != nil {
		return nil, err
	}

	tokens, err := s.authSvc.GenerateTokenPairForUser(user)
	if err != nil {
		return nil, fmt.Errorf("generating tokens: %w", err)
	}

	return &SocialLoginOutput{
		User:       user,
		Collection: collection,
		Tokens:     tokens,
		IsNewUser:  true,
	}, nil
}
