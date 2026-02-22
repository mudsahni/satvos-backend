package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/service"
)

// MockEmailConfigService is a mock implementation of service.EmailConfigService.
type MockEmailConfigService struct {
	mock.Mock
}

func (m *MockEmailConfigService) GetConfig(ctx context.Context, tenantID uuid.UUID) (*service.EmailConfigOutput, error) {
	args := m.Called(ctx, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*service.EmailConfigOutput), args.Error(1)
}

func (m *MockEmailConfigService) UpdateConfig(ctx context.Context, tenantID, callerID uuid.UUID, input service.UpdateEmailConfigInput) (*service.EmailConfigOutput, error) {
	args := m.Called(ctx, tenantID, callerID, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*service.EmailConfigOutput), args.Error(1)
}
