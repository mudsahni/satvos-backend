package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/service"
)

// MockSyncService is a mock implementation of service.SyncService.
type MockSyncService struct {
	mock.Mock
}

func (m *MockSyncService) Register(ctx context.Context, tenantID, serviceAccountID uuid.UUID, version, osInfo string) (*domain.ConnectorAgent, error) {
	args := m.Called(ctx, tenantID, serviceAccountID, version, osInfo)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ConnectorAgent), args.Error(1)
}

func (m *MockSyncService) Heartbeat(ctx context.Context, tenantID, serviceAccountID uuid.UUID, tallyConnected bool, tallyCompany string, tallyPort int, version string, agentErrors []string) error {
	args := m.Called(ctx, tenantID, serviceAccountID, tallyConnected, tallyCompany, tallyPort, version, agentErrors)
	return args.Error(0)
}

func (m *MockSyncService) SaveMasters(ctx context.Context, tenantID uuid.UUID, masters *service.MasterPayload) error {
	args := m.Called(ctx, tenantID, masters)
	return args.Error(0)
}

func (m *MockSyncService) ListOutbound(ctx context.Context, tenantID, serviceAccountID uuid.UUID, cursor string, limit int) ([]service.OutboundItem, string, error) {
	args := m.Called(ctx, tenantID, serviceAccountID, cursor, limit)
	return args.Get(0).([]service.OutboundItem), args.String(1), args.Error(2)
}

func (m *MockSyncService) AckOutbound(ctx context.Context, tenantID, serviceAccountID uuid.UUID, results []service.AckResult) error {
	args := m.Called(ctx, tenantID, serviceAccountID, results)
	return args.Error(0)
}

func (m *MockSyncService) SaveInbound(ctx context.Context, tenantID uuid.UUID, vouchers []domain.TallyVoucher) error {
	args := m.Called(ctx, tenantID, vouchers)
	return args.Error(0)
}
