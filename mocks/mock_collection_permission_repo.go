package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
)

// MockCollectionPermissionRepo is a mock implementation of port.CollectionPermissionRepository.
type MockCollectionPermissionRepo struct {
	mock.Mock
}

func (m *MockCollectionPermissionRepo) Upsert(ctx context.Context, perm *domain.CollectionPermissionEntry) error {
	args := m.Called(ctx, perm)
	return args.Error(0)
}

func (m *MockCollectionPermissionRepo) GetByCollectionAndUser(ctx context.Context, collectionID, userID uuid.UUID) (*domain.CollectionPermissionEntry, error) {
	args := m.Called(ctx, collectionID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.CollectionPermissionEntry), args.Error(1)
}

func (m *MockCollectionPermissionRepo) GetByUserForCollections(ctx context.Context, userID uuid.UUID, collectionIDs []uuid.UUID) (map[uuid.UUID]domain.CollectionPermission, error) {
	args := m.Called(ctx, userID, collectionIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[uuid.UUID]domain.CollectionPermission), args.Error(1)
}

func (m *MockCollectionPermissionRepo) ListByCollection(ctx context.Context, collectionID uuid.UUID, offset, limit int) ([]domain.CollectionPermissionEntry, int, error) {
	args := m.Called(ctx, collectionID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.CollectionPermissionEntry), args.Int(1), args.Error(2)
}

func (m *MockCollectionPermissionRepo) ListCollectionIDs(ctx context.Context, tenantID, userID uuid.UUID) ([]uuid.UUID, error) {
	args := m.Called(ctx, tenantID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]uuid.UUID), args.Error(1)
}

func (m *MockCollectionPermissionRepo) Delete(ctx context.Context, collectionID, userID uuid.UUID) error {
	args := m.Called(ctx, collectionID, userID)
	return args.Error(0)
}
