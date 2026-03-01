package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
)

// MockSyncRepo is a mock implementation of port.SyncRepository.
type MockSyncRepo struct {
	mock.Mock
}

func (m *MockSyncRepo) RegisterAgent(ctx context.Context, agent *domain.ConnectorAgent) error {
	args := m.Called(ctx, agent)
	return args.Error(0)
}

func (m *MockSyncRepo) GetAgentByServiceAccount(ctx context.Context, tenantID, serviceAccountID uuid.UUID) (*domain.ConnectorAgent, error) {
	args := m.Called(ctx, tenantID, serviceAccountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ConnectorAgent), args.Error(1)
}

func (m *MockSyncRepo) GetAgentByID(ctx context.Context, tenantID, agentID uuid.UUID) (*domain.ConnectorAgent, error) {
	args := m.Called(ctx, tenantID, agentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ConnectorAgent), args.Error(1)
}

func (m *MockSyncRepo) UpdateHeartbeat(ctx context.Context, agentID uuid.UUID, status domain.AgentStatus, tallyCompany string, tallyPort int, version string) error {
	args := m.Called(ctx, agentID, status, tallyCompany, tallyPort, version)
	return args.Error(0)
}

func (m *MockSyncRepo) CreateSyncEvent(ctx context.Context, event *domain.SyncEvent) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *MockSyncRepo) UpdateSyncEventStatus(ctx context.Context, eventID uuid.UUID, status domain.SyncStatus, tallyVoucherID, tallyVoucherNumber, errorMessage string) error {
	args := m.Called(ctx, eventID, status, tallyVoucherID, tallyVoucherNumber, errorMessage)
	return args.Error(0)
}

func (m *MockSyncRepo) ListSyncEvents(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.SyncEvent, int, error) {
	args := m.Called(ctx, tenantID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.SyncEvent), args.Int(1), args.Error(2)
}

func (m *MockSyncRepo) ListSyncEventsByDocument(ctx context.Context, tenantID, documentID uuid.UUID, offset, limit int) ([]domain.SyncEvent, int, error) {
	args := m.Called(ctx, tenantID, documentID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.SyncEvent), args.Int(1), args.Error(2)
}

func (m *MockSyncRepo) ListAgents(ctx context.Context, tenantID uuid.UUID) ([]domain.ConnectorAgent, error) {
	args := m.Called(ctx, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.ConnectorAgent), args.Error(1)
}

func (m *MockSyncRepo) ListOutboundDocuments(ctx context.Context, tenantID uuid.UUID, cursor string, limit int) ([]domain.Document, string, error) {
	args := m.Called(ctx, tenantID, cursor, limit)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]domain.Document), args.String(1), args.Error(2)
}
