package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
)

// MockServiceAccountPermissionRepo is a mock implementation of domain.ServiceAccountPermissionRepository.
type MockServiceAccountPermissionRepo struct {
	mock.Mock
}

func (m *MockServiceAccountPermissionRepo) Upsert(ctx context.Context, perm *domain.ServiceAccountPermission) error {
	args := m.Called(ctx, perm)
	return args.Error(0)
}

func (m *MockServiceAccountPermissionRepo) GetByAccountAndCollection(ctx context.Context, saID, collectionID uuid.UUID) (*domain.ServiceAccountPermission, error) {
	args := m.Called(ctx, saID, collectionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ServiceAccountPermission), args.Error(1)
}

func (m *MockServiceAccountPermissionRepo) ListByAccount(ctx context.Context, saID uuid.UUID) ([]domain.ServiceAccountPermission, error) {
	args := m.Called(ctx, saID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.ServiceAccountPermission), args.Error(1)
}

func (m *MockServiceAccountPermissionRepo) Delete(ctx context.Context, saID, collectionID uuid.UUID) error {
	args := m.Called(ctx, saID, collectionID)
	return args.Error(0)
}
