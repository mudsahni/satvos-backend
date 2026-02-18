package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
)

// MockCollectionRepo is a mock implementation of port.CollectionRepository.
type MockCollectionRepo struct {
	mock.Mock
}

func (m *MockCollectionRepo) Create(ctx context.Context, collection *domain.Collection) error {
	args := m.Called(ctx, collection)
	return args.Error(0)
}

func (m *MockCollectionRepo) GetByID(ctx context.Context, tenantID, collectionID uuid.UUID) (*domain.Collection, error) {
	args := m.Called(ctx, tenantID, collectionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Collection), args.Error(1)
}

func (m *MockCollectionRepo) ListByUser(ctx context.Context, tenantID, userID uuid.UUID, offset, limit int) ([]domain.Collection, int, error) {
	args := m.Called(ctx, tenantID, userID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Collection), args.Int(1), args.Error(2)
}

func (m *MockCollectionRepo) ListByServiceAccount(ctx context.Context, tenantID, saID uuid.UUID, offset, limit int) ([]domain.Collection, int, error) {
	args := m.Called(ctx, tenantID, saID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Collection), args.Int(1), args.Error(2)
}

func (m *MockCollectionRepo) ListByTenant(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.Collection, int, error) {
	args := m.Called(ctx, tenantID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Collection), args.Int(1), args.Error(2)
}

func (m *MockCollectionRepo) Update(ctx context.Context, collection *domain.Collection) error {
	args := m.Called(ctx, collection)
	return args.Error(0)
}

func (m *MockCollectionRepo) Delete(ctx context.Context, tenantID, collectionID uuid.UUID) error {
	args := m.Called(ctx, tenantID, collectionID)
	return args.Error(0)
}
