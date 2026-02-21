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

// AcceptInvitationInput is the DTO for accepting an invitation.
type AcceptInvitationInput struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password" binding:"required,min=8"`
}

// AcceptInvitationOutput holds the result of accepting an invitation.
type AcceptInvitationOutput struct {
	User   *domain.User `json:"user"`
	Tokens *TokenPair   `json:"tokens"`
}

// InvitationService defines the invitation contract.
type InvitationService interface {
	SendInvitation(ctx context.Context, user *domain.User) error
	ResendInvitation(ctx context.Context, tenantID, userID uuid.UUID) error
	AcceptInvitation(ctx context.Context, input AcceptInvitationInput) (*AcceptInvitationOutput, error)
}

type invitationService struct {
	userRepo    port.UserRepository
	emailSender port.EmailSender
	authSvc     AuthService
	jwtCfg      config.JWTConfig
}

// NewInvitationService creates a new InvitationService.
func NewInvitationService(
	userRepo port.UserRepository,
	emailSender port.EmailSender,
	authSvc AuthService,
	jwtCfg config.JWTConfig,
) InvitationService {
	return &invitationService{
		userRepo:    userRepo,
		emailSender: emailSender,
		authSvc:     authSvc,
		jwtCfg:      jwtCfg,
	}
}

func (s *invitationService) SendInvitation(ctx context.Context, user *domain.User) error {
	tokenString, jti, err := s.generateInvitationToken(user)
	if err != nil {
		return fmt.Errorf("generating invitation token: %w", err)
	}

	if err := s.userRepo.SetPasswordResetToken(ctx, user.TenantID, user.ID, jti); err != nil {
		return fmt.Errorf("storing invitation token: %w", err)
	}

	if err := s.emailSender.SendInvitationEmail(ctx, user.Email, user.FullName, string(user.Role), tokenString); err != nil {
		log.Printf("WARNING: failed to send invitation email to %s: %v", user.Email, err)
	}

	return nil
}

func (s *invitationService) ResendInvitation(ctx context.Context, tenantID, userID uuid.UUID) error {
	user, err := s.userRepo.GetByID(ctx, tenantID, userID)
	if err != nil {
		return err
	}

	// Only resend if user hasn't set a password yet (still pending)
	if user.PasswordHash != "" {
		return domain.ErrInvitationAlreadyAccepted
	}
	if user.AuthProvider == domain.AuthProviderGoogle {
		return domain.ErrInvitationAlreadyAccepted
	}

	return s.SendInvitation(ctx, user)
}

func (s *invitationService) AcceptInvitation(ctx context.Context, input AcceptInvitationInput) (*AcceptInvitationOutput, error) {
	claims, err := s.parseInvitationToken(input.Token)
	if err != nil {
		return nil, domain.ErrInvitationTokenInvalid
	}

	user, err := s.userRepo.GetByID(ctx, claims.TenantID, claims.UserID)
	if err != nil {
		return nil, domain.ErrInvitationTokenInvalid
	}

	if !user.IsActive {
		return nil, domain.ErrUserInactive
	}

	if user.PasswordHash != "" {
		return nil, domain.ErrInvitationTokenInvalid
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), 12)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	if err := s.userRepo.AcceptInvitation(ctx, claims.TenantID, claims.UserID, string(hash), claims.ID); err != nil {
		return nil, err
	}

	// Re-fetch user for updated fields
	user, err = s.userRepo.GetByID(ctx, claims.TenantID, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("re-fetching user after invitation: %w", err)
	}

	tokens, err := s.authSvc.GenerateTokenPairForUser(user)
	if err != nil {
		return nil, fmt.Errorf("generating tokens: %w", err)
	}

	return &AcceptInvitationOutput{
		User:   user,
		Tokens: tokens,
	}, nil
}

func (s *invitationService) generateInvitationToken(user *domain.User) (tokenString, jti string, err error) {
	now := time.Now()
	jti = uuid.New().String()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			Issuer:    s.jwtCfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(72 * time.Hour)),
			ID:        jti,
			Audience:  jwt.ClaimStrings{"invitation"},
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

func (s *invitationService) parseInvitationToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtCfg.Secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("parsing invitation token: %w", err)
	}
	if !token.Valid {
		return nil, domain.ErrInvitationTokenInvalid
	}

	aud, _ := claims.GetAudience()
	found := false
	for _, a := range aud {
		if a == "invitation" {
			found = true
			break
		}
	}
	if !found {
		return nil, domain.ErrInvitationTokenInvalid
	}

	return claims, nil
}
