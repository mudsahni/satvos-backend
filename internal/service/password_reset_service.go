package service

import (
	"context"
	"errors"
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

// ForgotPasswordInput is the DTO for forgot-password requests.
type ForgotPasswordInput struct {
	TenantSlug string `json:"tenant_slug" binding:"required"`
	Email      string `json:"email" binding:"required,email"`
}

// ResetPasswordInput is the DTO for reset-password requests.
type ResetPasswordInput struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

// PasswordResetService defines the password reset contract.
type PasswordResetService interface {
	ForgotPassword(ctx context.Context, input ForgotPasswordInput) error
	ResetPassword(ctx context.Context, input ResetPasswordInput) error
}

type passwordResetService struct {
	tenantRepo  port.TenantRepository
	userRepo    port.UserRepository
	emailSender port.EmailSender
	jwtCfg      config.JWTConfig
}

// NewPasswordResetService creates a new PasswordResetService.
func NewPasswordResetService(
	tenantRepo port.TenantRepository,
	userRepo port.UserRepository,
	emailSender port.EmailSender,
	jwtCfg config.JWTConfig,
) PasswordResetService {
	return &passwordResetService{
		tenantRepo:  tenantRepo,
		userRepo:    userRepo,
		emailSender: emailSender,
		jwtCfg:      jwtCfg,
	}
}

func (s *passwordResetService) ForgotPassword(ctx context.Context, input ForgotPasswordInput) error {
	tenant, err := s.tenantRepo.GetBySlug(ctx, input.TenantSlug)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		log.Printf("WARNING: forgot-password tenant lookup error: %v", err)
		return nil
	}
	if !tenant.IsActive {
		return nil
	}

	user, err := s.userRepo.GetByEmail(ctx, tenant.ID, input.Email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		log.Printf("WARNING: forgot-password user lookup error: %v", err)
		return nil
	}
	if !user.IsActive {
		return nil
	}

	// Skip invited users who haven't accepted yet — they need invitation, not reset
	if user.PasswordHash == "" && user.AuthProvider != domain.AuthProviderGoogle {
		return nil
	}

	tokenString, jti, err := s.generateResetToken(user)
	if err != nil {
		log.Printf("WARNING: failed to generate password reset token for %s: %v", user.Email, err)
		return nil
	}

	if err := s.userRepo.SetPasswordResetToken(ctx, tenant.ID, user.ID, jti); err != nil {
		log.Printf("WARNING: failed to store password reset token for %s: %v", user.Email, err)
		return nil
	}

	if err := s.emailSender.SendPasswordResetEmail(ctx, user.Email, user.FullName, tokenString); err != nil {
		log.Printf("WARNING: failed to send password reset email to %s: %v", user.Email, err)
	}

	return nil
}

func (s *passwordResetService) ResetPassword(ctx context.Context, input ResetPasswordInput) error {
	claims, err := s.parseResetToken(input.Token)
	if err != nil {
		return domain.ErrPasswordResetTokenInvalid
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), 12)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	return s.userRepo.ResetPassword(ctx, claims.TenantID, claims.UserID, string(hash), claims.ID)
}

func (s *passwordResetService) generateResetToken(user *domain.User) (tokenString, jti string, err error) {
	now := time.Now()
	jti = uuid.New().String()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			Issuer:    s.jwtCfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
			ID:        jti,
			Audience:  jwt.ClaimStrings{"password-reset"},
		},
		TenantID: user.TenantID,
		UserID:   user.ID,
		Email:    user.Email,
		Role:     user.Role,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err = token.SignedString([]byte(s.jwtCfg.Secret))
	if err != nil {
		return "", "", err
	}
	return tokenString, jti, nil
}

func (s *passwordResetService) parseResetToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtCfg.Secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("parsing reset token: %w", err)
	}
	if !token.Valid {
		return nil, domain.ErrPasswordResetTokenInvalid
	}

	aud, _ := claims.GetAudience()
	found := false
	for _, a := range aud {
		if a == "password-reset" {
			found = true
			break
		}
	}
	if !found {
		return nil, domain.ErrPasswordResetTokenInvalid
	}

	return claims, nil
}
