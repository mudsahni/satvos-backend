package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
)

// MockServiceAccountRepo is a mock implementation of port.ServiceAccountRepository.
type MockServiceAccountRepo struct {
	mock.Mock
}

func (m *MockServiceAccountRepo) Create(ctx context.Context, sa *domain.ServiceAccount) error {
	args := m.Called(ctx, sa)
	return args.Error(0)
}

func (m *MockServiceAccountRepo) GetByID(ctx context.Context, tenantID, saID uuid.UUID) (*domain.ServiceAccount, error) {
	args := m.Called(ctx, tenantID, saID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ServiceAccount), args.Error(1)
}

func (m *MockServiceAccountRepo) GetByAPIKeyPrefix(ctx context.Context, prefix string) ([]domain.ServiceAccount, error) {
	args := m.Called(ctx, prefix)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.ServiceAccount), args.Error(1)
}

func (m *MockServiceAccountRepo) ListByTenant(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.ServiceAccount, int, error) {
	args := m.Called(ctx, tenantID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.ServiceAccount), args.Int(1), args.Error(2)
}

func (m *MockServiceAccountRepo) Update(ctx context.Context, sa *domain.ServiceAccount) error {
	args := m.Called(ctx, sa)
	return args.Error(0)
}

func (m *MockServiceAccountRepo) Revoke(ctx context.Context, tenantID, saID uuid.UUID) error {
	args := m.Called(ctx, tenantID, saID)
	return args.Error(0)
}

func (m *MockServiceAccountRepo) RotateKey(ctx context.Context, tenantID, saID uuid.UUID, newPrefix, newHash string) error {
	args := m.Called(ctx, tenantID, saID, newPrefix, newHash)
	return args.Error(0)
}

func (m *MockServiceAccountRepo) Delete(ctx context.Context, tenantID, saID uuid.UUID) error {
	args := m.Called(ctx, tenantID, saID)
	return args.Error(0)
}

func (m *MockServiceAccountRepo) UpdateLastUsed(ctx context.Context, saID uuid.UUID) error {
	args := m.Called(ctx, saID)
	return args.Error(0)
}
