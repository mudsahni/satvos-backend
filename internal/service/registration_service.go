package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"satvos/internal/config"
	"satvos/internal/domain"
	"satvos/internal/port"
)

// RegisterInput is the DTO for free-tier self-registration.
type RegisterInput struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	FullName string `json:"full_name" binding:"required"`
}

// RegisterOutput contains the results of a successful registration.
type RegisterOutput struct {
	User       *domain.User       `json:"user"`
	Collection *domain.Collection `json:"collection"`
	Tokens     *TokenPair         `json:"tokens"`
}

// RegistrationService defines the self-registration contract for free-tier users.
type RegistrationService interface {
	Register(ctx context.Context, input RegisterInput) (*RegisterOutput, error)
	VerifyEmail(ctx context.Context, token string) error
	ResendVerification(ctx context.Context, tenantID, userID uuid.UUID) error
}

type registrationService struct {
	tenantRepo  port.TenantRepository
	userRepo    port.UserRepository
	collRepo    port.CollectionRepository
	permRepo    port.CollectionPermissionRepository
	authSvc     AuthService
	emailSender port.EmailSender
	jwtCfg      config.JWTConfig
	freeTierCfg config.FreeTierConfig
}

// NewRegistrationService creates a new RegistrationService.
func NewRegistrationService(
	tenantRepo port.TenantRepository,
	userRepo port.UserRepository,
	collRepo port.CollectionRepository,
	permRepo port.CollectionPermissionRepository,
	authSvc AuthService,
	emailSender port.EmailSender,
	jwtCfg config.JWTConfig,
	freeTierCfg config.FreeTierConfig,
) RegistrationService {
	return &registrationService{
		tenantRepo:  tenantRepo,
		userRepo:    userRepo,
		collRepo:    collRepo,
		permRepo:    permRepo,
		authSvc:     authSvc,
		emailSender: emailSender,
		jwtCfg:      jwtCfg,
		freeTierCfg: freeTierCfg,
	}
}

func (s *registrationService) Register(ctx context.Context, input RegisterInput) (*RegisterOutput, error) {
	// Look up the shared free-tier tenant
	tenant, err := s.tenantRepo.GetBySlug(ctx, s.freeTierCfg.TenantSlug)
	if err != nil {
		return nil, fmt.Errorf("looking up free tier tenant: %w", err)
	}
	if !tenant.IsActive {
		return nil, domain.ErrTenantInactive
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), 12)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	// Create user with free role, quota, and unverified email
	user := &domain.User{
		TenantID:             tenant.ID,
		Email:                input.Email,
		PasswordHash:         string(hash),
		FullName:             input.FullName,
		Role:                 domain.RoleFree,
		IsActive:             true,
		MonthlyDocumentLimit: s.freeTierCfg.MonthlyLimit,
		EmailVerified:        false,
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err // ErrDuplicateEmail propagates naturally
	}

	// Create personal collection
	collection, err := CreatePersonalCollection(ctx, s.collRepo, s.permRepo, tenant.ID, user.ID, input.FullName)
	if err != nil {
		return nil, err
	}

	// Generate tokens by logging in
	tokens, err := s.authSvc.Login(ctx, LoginInput{
		TenantSlug: s.freeTierCfg.TenantSlug,
		Email:      input.Email,
		Password:   input.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("generating tokens: %w", err)
	}

	// Send verification email (non-blocking — don't fail registration)
	verifyToken, tokenErr := s.generateVerificationToken(user)
	if tokenErr != nil {
		log.Printf("WARNING: failed to generate verification token for %s: %v", user.Email, tokenErr)
	} else if sendErr := s.emailSender.SendVerificationEmail(ctx, user.Email, user.FullName, verifyToken); sendErr != nil {
		log.Printf("WARNING: failed to send verification email to %s: %v", user.Email, sendErr)
	}

	return &RegisterOutput{
		User:       user,
		Collection: collection,
		Tokens:     tokens,
	}, nil
}

func (s *registrationService) VerifyEmail(ctx context.Context, tokenString string) error {
	claims, err := s.parseVerificationToken(tokenString)
	if err != nil {
		return domain.ErrUnauthorized
	}

	user, err := s.userRepo.GetByID(ctx, claims.TenantID, claims.UserID)
	if err != nil {
		return err
	}

	// Idempotent — already verified
	if user.EmailVerified {
		return nil
	}

	return s.userRepo.SetEmailVerified(ctx, claims.TenantID, claims.UserID)
}

func (s *registrationService) ResendVerification(ctx context.Context, tenantID, userID uuid.UUID) error {
	user, err := s.userRepo.GetByID(ctx, tenantID, userID)
	if err != nil {
		return err
	}

	// Already verified — no-op
	if user.EmailVerified {
		return nil
	}

	verifyToken, err := s.generateVerificationToken(user)
	if err != nil {
		return fmt.Errorf("generating verification token: %w", err)
	}

	return s.emailSender.SendVerificationEmail(ctx, user.Email, user.FullName, verifyToken)
}

func (s *registrationService) generateVerificationToken(user *domain.User) (string, error) {
	now := time.Now()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			Issuer:    s.jwtCfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
			ID:        uuid.New().String(),
			Audience:  jwt.ClaimStrings{"email-verification"},
		},
		TenantID: user.TenantID,
		UserID:   user.ID,
		Email:    user.Email,
		Role:     user.Role,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtCfg.Secret))
}

func (s *registrationService) parseVerificationToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtCfg.Secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("parsing verification token: %w", err)
	}
	if !token.Valid {
		return nil, domain.ErrUnauthorized
	}

	// Validate audience
	aud, _ := claims.GetAudience()
	found := false
	for _, a := range aud {
		if a == "email-verification" {
			found = true
			break
		}
	}
	if !found {
		return nil, domain.ErrUnauthorized
	}

	return claims, nil
}
