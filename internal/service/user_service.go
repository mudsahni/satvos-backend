package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"satvos/internal/domain"
	"satvos/internal/port"
)

// CreateUserInput is the DTO for creating a user.
type CreateUserInput struct {
	Email    string          `json:"email" binding:"required,email"`
	Password string          `json:"password" binding:"omitempty,min=8"`
	FullName string          `json:"full_name" binding:"required"`
	Role     domain.UserRole `json:"role" binding:"required"`
}

// UpdateUserInput is the DTO for updating a user.
type UpdateUserInput struct {
	Email    *string          `json:"email"`
	FullName *string          `json:"full_name"`
	Role     *domain.UserRole `json:"role"`
	IsActive *bool            `json:"is_active"`
}

// UserService defines the user management contract.
type UserService interface {
	Create(ctx context.Context, tenantID uuid.UUID, input CreateUserInput) (*domain.User, error)
	GetByID(ctx context.Context, tenantID, userID uuid.UUID) (*domain.User, error)
	List(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.User, int, error)
	Update(ctx context.Context, tenantID, userID uuid.UUID, input UpdateUserInput) (*domain.User, error)
	Delete(ctx context.Context, tenantID, userID uuid.UUID) error
}

type userService struct {
	repo port.UserRepository
}

// NewUserService creates a new UserService implementation.
func NewUserService(repo port.UserRepository) UserService {
	return &userService{repo: repo}
}

func (s *userService) Create(ctx context.Context, tenantID uuid.UUID, input CreateUserInput) (*domain.User, error) {
	if !domain.ValidUserRoles[input.Role] {
		return nil, domain.ErrInsufficientRole
	}

	user := &domain.User{
		TenantID: tenantID,
		Email:    input.Email,
		FullName: input.FullName,
		Role:     input.Role,
		IsActive: true,
	}

	if input.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), 12)
		if err != nil {
			return nil, fmt.Errorf("hashing password: %w", err)
		}
		user.PasswordHash = string(hash)
		user.EmailVerified = true
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *userService) GetByID(ctx context.Context, tenantID, userID uuid.UUID) (*domain.User, error) {
	return s.repo.GetByID(ctx, tenantID, userID)
}

func (s *userService) List(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.User, int, error) {
	return s.repo.ListByTenant(ctx, tenantID, offset, limit)
}

func (s *userService) Update(ctx context.Context, tenantID, userID uuid.UUID, input UpdateUserInput) (*domain.User, error) {
	user, err := s.repo.GetByID(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}

	if input.Email != nil {
		user.Email = *input.Email
	}
	if input.FullName != nil {
		user.FullName = *input.FullName
	}
	if input.Role != nil {
		if !domain.ValidUserRoles[*input.Role] {
			return nil, domain.ErrInsufficientRole
		}
		user.Role = *input.Role
	}
	if input.IsActive != nil {
		user.IsActive = *input.IsActive
	}

	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *userService) Delete(ctx context.Context, tenantID, userID uuid.UUID) error {
	return s.repo.Delete(ctx, tenantID, userID)
}
