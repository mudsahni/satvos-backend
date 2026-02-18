package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/port"
	"satvos/internal/service"
)

// MockServiceAccountService is a mock implementation of service.ServiceAccountService.
type MockServiceAccountService struct {
	mock.Mock
}

func (m *MockServiceAccountService) Create(ctx context.Context, input *service.CreateServiceAccountInput) (*service.CreateServiceAccountOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*service.CreateServiceAccountOutput), args.Error(1)
}

func (m *MockServiceAccountService) GetByID(ctx context.Context, tenantID, saID uuid.UUID) (*domain.ServiceAccount, error) {
	args := m.Called(ctx, tenantID, saID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ServiceAccount), args.Error(1)
}

func (m *MockServiceAccountService) List(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.ServiceAccount, int, error) {
	args := m.Called(ctx, tenantID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.ServiceAccount), args.Int(1), args.Error(2)
}

func (m *MockServiceAccountService) RotateAPIKey(ctx context.Context, tenantID, saID uuid.UUID) (*service.RotateAPIKeyOutput, error) {
	args := m.Called(ctx, tenantID, saID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*service.RotateAPIKeyOutput), args.Error(1)
}

func (m *MockServiceAccountService) Revoke(ctx context.Context, tenantID, saID uuid.UUID) error {
	args := m.Called(ctx, tenantID, saID)
	return args.Error(0)
}

func (m *MockServiceAccountService) Delete(ctx context.Context, tenantID, saID uuid.UUID) error {
	args := m.Called(ctx, tenantID, saID)
	return args.Error(0)
}

func (m *MockServiceAccountService) SetPermission(ctx context.Context, input *service.SASetPermissionInput) error {
	args := m.Called(ctx, input)
	return args.Error(0)
}

func (m *MockServiceAccountService) ListPermissions(ctx context.Context, tenantID, saID uuid.UUID) ([]port.ServiceAccountPermission, error) {
	args := m.Called(ctx, tenantID, saID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]port.ServiceAccountPermission), args.Error(1)
}

func (m *MockServiceAccountService) RemovePermission(ctx context.Context, tenantID, saID, collectionID uuid.UUID) error {
	args := m.Called(ctx, tenantID, saID, collectionID)
	return args.Error(0)
}

func (m *MockServiceAccountService) Authenticate(ctx context.Context, rawKey string) (*domain.ServiceAccount, error) {
	args := m.Called(ctx, rawKey)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ServiceAccount), args.Error(1)
}
