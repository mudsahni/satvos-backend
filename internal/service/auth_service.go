package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"satvos/internal/config"
	"satvos/internal/domain"
	"satvos/internal/port"
)

// Claims represents the JWT claims with tenant context.
type Claims struct {
	jwt.RegisteredClaims
	TenantID uuid.UUID       `json:"tenant_id"`
	UserID   uuid.UUID       `json:"user_id"`
	Email    string          `json:"email"`
	Role     domain.UserRole `json:"role"`
}

// TokenPair holds access and refresh tokens.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// LoginInput is the DTO for login requests.
type LoginInput struct {
	TenantSlug string `json:"tenant_slug" binding:"required"`
	Email      string `json:"email" binding:"required,email"`
	Password   string `json:"password" binding:"required,min=8"`
}

// RefreshInput is the DTO for token refresh requests.
type RefreshInput struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// AuthService defines the authentication contract.
type AuthService interface {
	Login(ctx context.Context, input LoginInput) (*TokenPair, error)
	RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error)
	ValidateToken(tokenString string) (*Claims, error)
	GenerateTokenPairForUser(user *domain.User) (*TokenPair, error)
}

type authService struct {
	userRepo   port.UserRepository
	tenantRepo port.TenantRepository
	cfg        config.JWTConfig
}

// NewAuthService creates a new AuthService implementation.
func NewAuthService(
	userRepo port.UserRepository,
	tenantRepo port.TenantRepository,
	cfg config.JWTConfig,
) AuthService {
	return &authService{
		userRepo:   userRepo,
		tenantRepo: tenantRepo,
		cfg:        cfg,
	}
}

func (s *authService) Login(ctx context.Context, input LoginInput) (*TokenPair, error) {
	tenant, err := s.tenantRepo.GetBySlug(ctx, input.TenantSlug)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("auth.Login: %w", err)
	}
	if !tenant.IsActive {
		return nil, domain.ErrTenantInactive
	}

	user, err := s.userRepo.GetByEmail(ctx, tenant.ID, input.Email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("auth.Login: %w", err)
	}
	if !user.IsActive {
		return nil, domain.ErrUserInactive
	}

	if user.PasswordHash == "" {
		if user.AuthProvider == domain.AuthProviderGoogle {
			return nil, domain.ErrPasswordLoginNotAllowed
		}
		return nil, domain.ErrInvitationPending
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	return s.generateTokenPair(user)
}

func (s *authService) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := s.validateTokenString(refreshToken, "refresh")
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	user, err := s.userRepo.GetByID(ctx, claims.TenantID, claims.UserID)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}
	if !user.IsActive {
		return nil, domain.ErrUserInactive
	}

	return s.generateTokenPair(user)
}

func (s *authService) ValidateToken(tokenString string) (*Claims, error) {
	return s.validateTokenString(tokenString, "access")
}

func (s *authService) GenerateTokenPairForUser(user *domain.User) (*TokenPair, error) {
	return s.generateTokenPair(user)
}

func (s *authService) generateTokenPair(user *domain.User) (*TokenPair, error) {
	now := time.Now()
	accessExpiry := now.Add(s.cfg.AccessTokenExpiry)
	refreshExpiry := now.Add(s.cfg.RefreshTokenExpiry)

	accessClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			Issuer:    s.cfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(accessExpiry),
			ID:        uuid.New().String(),
			Audience:  jwt.ClaimStrings{"access"},
		},
		TenantID: user.TenantID,
		UserID:   user.ID,
		Email:    user.Email,
		Role:     user.Role,
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(s.cfg.Secret))
	if err != nil {
		return nil, fmt.Errorf("signing access token: %w", err)
	}

	refreshClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			Issuer:    s.cfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(refreshExpiry),
			ID:        uuid.New().String(),
			Audience:  jwt.ClaimStrings{"refresh"},
		},
		TenantID: user.TenantID,
		UserID:   user.ID,
		Email:    user.Email,
		Role:     user.Role,
	}

	refreshTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshTokenObj.SignedString([]byte(s.cfg.Secret))
	if err != nil {
		return nil, fmt.Errorf("signing refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
		ExpiresAt:    accessExpiry,
	}, nil
}

func (s *authService) validateTokenString(tokenString, audience string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.cfg.Secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("parsing token: %w", err)
	}
	if !token.Valid {
		return nil, domain.ErrUnauthorized
	}

	// Validate audience
	aud, _ := claims.GetAudience()
	found := false
	for _, a := range aud {
		if a == audience {
			found = true
			break
		}
	}
	if !found {
		return nil, domain.ErrUnauthorized
	}

	return claims, nil
}
