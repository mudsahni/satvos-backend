package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"satvos/internal/port"
)

// MockEmailConfigRepo is a mock implementation of port.TenantEmailConfigRepository.
type MockEmailConfigRepo struct {
	mock.Mock
}

func (m *MockEmailConfigRepo) Get(ctx context.Context, tenantSlug string) (*port.TenantEmailConfig, error) {
	args := m.Called(ctx, tenantSlug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*port.TenantEmailConfig), args.Error(1)
}

func (m *MockEmailConfigRepo) Put(ctx context.Context, item *port.TenantEmailConfig) error {
	args := m.Called(ctx, item)
	return args.Error(0)
}

func (m *MockEmailConfigRepo) UpdateConfig(ctx context.Context, tenantSlug string, enabled bool, allowedSenders []string) error {
	args := m.Called(ctx, tenantSlug, enabled, allowedSenders)
	return args.Error(0)
}
