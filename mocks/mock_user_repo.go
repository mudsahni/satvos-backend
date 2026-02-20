package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
)

// MockUserRepo is a mock implementation of port.UserRepository.
type MockUserRepo struct {
	mock.Mock
}

func (m *MockUserRepo) Create(ctx context.Context, user *domain.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserRepo) GetByID(ctx context.Context, tenantID, userID uuid.UUID) (*domain.User, error) {
	args := m.Called(ctx, tenantID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *MockUserRepo) GetByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*domain.User, error) {
	args := m.Called(ctx, tenantID, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *MockUserRepo) ListByTenant(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.User, int, error) {
	args := m.Called(ctx, tenantID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.User), args.Int(1), args.Error(2)
}

func (m *MockUserRepo) Update(ctx context.Context, user *domain.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserRepo) Delete(ctx context.Context, tenantID, userID uuid.UUID) error {
	args := m.Called(ctx, tenantID, userID)
	return args.Error(0)
}

func (m *MockUserRepo) CheckAndIncrementQuota(ctx context.Context, tenantID, userID uuid.UUID) error {
	args := m.Called(ctx, tenantID, userID)
	return args.Error(0)
}

func (m *MockUserRepo) SetEmailVerified(ctx context.Context, tenantID, userID uuid.UUID) error {
	args := m.Called(ctx, tenantID, userID)
	return args.Error(0)
}

func (m *MockUserRepo) SetPasswordResetToken(ctx context.Context, tenantID, userID uuid.UUID, tokenID string) error {
	args := m.Called(ctx, tenantID, userID, tokenID)
	return args.Error(0)
}

func (m *MockUserRepo) ResetPassword(ctx context.Context, tenantID, userID uuid.UUID, passwordHash, expectedTokenID string) error {
	args := m.Called(ctx, tenantID, userID, passwordHash, expectedTokenID)
	return args.Error(0)
}

func (m *MockUserRepo) GetByProviderID(ctx context.Context, tenantID uuid.UUID, provider domain.AuthProvider, providerUserID string) (*domain.User, error) {
	args := m.Called(ctx, tenantID, provider, providerUserID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *MockUserRepo) LinkProvider(ctx context.Context, tenantID, userID uuid.UUID, provider domain.AuthProvider, providerUserID string) error {
	args := m.Called(ctx, tenantID, userID, provider, providerUserID)
	return args.Error(0)
}

func (m *MockUserRepo) AcceptInvitation(ctx context.Context, tenantID, userID uuid.UUID, passwordHash, expectedTokenID string) error {
	args := m.Called(ctx, tenantID, userID, passwordHash, expectedTokenID)
	return args.Error(0)
}
