package port

import (
	"context"

	"github.com/google/uuid"

	"satvos/internal/domain"
)

// SyncRepository manages connector agents and sync events.
type SyncRepository interface {
	// Agent lifecycle
	RegisterAgent(ctx context.Context, agent *domain.ConnectorAgent) error
	GetAgentByServiceAccount(ctx context.Context, tenantID, serviceAccountID uuid.UUID) (*domain.ConnectorAgent, error)
	GetAgentByID(ctx context.Context, tenantID, agentID uuid.UUID) (*domain.ConnectorAgent, error)
	UpdateHeartbeat(ctx context.Context, agentID uuid.UUID, status domain.AgentStatus, tallyCompany string, tallyPort int, version string) error

	// Sync events
	CreateSyncEvent(ctx context.Context, event *domain.SyncEvent) error
	UpdateSyncEventStatus(ctx context.Context, eventID uuid.UUID, status domain.SyncStatus, tallyVoucherID, tallyVoucherNumber, errorMessage string) error
	ListSyncEvents(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.SyncEvent, int, error)
	ListSyncEventsByDocument(ctx context.Context, tenantID, documentID uuid.UUID, offset, limit int) ([]domain.SyncEvent, int, error)

	// Agents
	ListAgents(ctx context.Context, tenantID uuid.UUID) ([]domain.ConnectorAgent, error)

	// Outbound queue
	ListOutboundDocuments(ctx context.Context, tenantID uuid.UUID, cursor string, limit int) ([]domain.Document, string, error)
}
