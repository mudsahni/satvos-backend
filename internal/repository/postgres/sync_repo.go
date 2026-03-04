package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"satvos/internal/domain"
	"satvos/internal/port"
)

type syncRepo struct {
	db *sqlx.DB
}

// NewSyncRepo creates a new SyncRepository implementation.
func NewSyncRepo(db *sqlx.DB) port.SyncRepository {
	return &syncRepo{db: db}
}

func (r *syncRepo) RegisterAgent(ctx context.Context, agent *domain.ConnectorAgent) error {
	agent.ID = uuid.New()
	agent.RegisteredAt = time.Now().UTC()
	agent.Status = domain.AgentStatusRegistered
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO connector_agents (id, tenant_id, service_account_id, agent_version, tally_company,
			tally_port, os_info, status, last_heartbeat, registered_at)
		VALUES (:id, :tenant_id, :service_account_id, :agent_version, :tally_company,
			:tally_port, :os_info, :status, :last_heartbeat, :registered_at)`,
		agent)
	if err != nil {
		return fmt.Errorf("inserting connector agent: %w", err)
	}
	return nil
}

func (r *syncRepo) GetAgentByServiceAccount(ctx context.Context, tenantID, serviceAccountID uuid.UUID) (*domain.ConnectorAgent, error) {
	var agent domain.ConnectorAgent
	err := r.db.GetContext(ctx, &agent,
		"SELECT * FROM connector_agents WHERE tenant_id = $1 AND service_account_id = $2", tenantID, serviceAccountID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrAgentNotFound
		}
		return nil, fmt.Errorf("getting connector agent by service account: %w", err)
	}
	return &agent, nil
}

func (r *syncRepo) GetAgentByID(ctx context.Context, tenantID, agentID uuid.UUID) (*domain.ConnectorAgent, error) {
	var agent domain.ConnectorAgent
	err := r.db.GetContext(ctx, &agent,
		"SELECT * FROM connector_agents WHERE tenant_id = $1 AND id = $2", tenantID, agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrAgentNotFound
		}
		return nil, fmt.Errorf("getting connector agent by id: %w", err)
	}
	return &agent, nil
}

func (r *syncRepo) UpdateHeartbeat(ctx context.Context, agentID uuid.UUID, status domain.AgentStatus, tallyCompany string, tallyPort int, version string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE connector_agents SET
			status = $1, last_heartbeat = NOW(), tally_company = $2, tally_port = $3, agent_version = $4
		WHERE id = $5`,
		status, tallyCompany, tallyPort, version, agentID)
	if err != nil {
		return fmt.Errorf("updating agent heartbeat: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrAgentNotFound
	}
	return nil
}

func (r *syncRepo) CreateSyncEvent(ctx context.Context, event *domain.SyncEvent) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	event.CreatedAt = time.Now().UTC()
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO sync_events (id, tenant_id, agent_id, document_id, direction, status,
			tally_voucher_id, tally_voucher_number, error_message, created_at)
		VALUES (:id, :tenant_id, :agent_id, :document_id, :direction, :status,
			:tally_voucher_id, :tally_voucher_number, :error_message, :created_at)`,
		event)
	if err != nil {
		return fmt.Errorf("inserting sync event: %w", err)
	}

	// Update document sync_status for outbound events
	if event.DocumentID != nil && event.Direction == domain.SyncDirectionOutbound {
		var docSyncStatus domain.DocumentSyncStatus
		switch event.Status {
		case domain.SyncStatusSuccess:
			docSyncStatus = domain.DocumentSyncStatusSynced
		case domain.SyncStatusFailed:
			docSyncStatus = domain.DocumentSyncStatusFailed
		default:
			docSyncStatus = domain.DocumentSyncStatusPending
		}
		_, _ = r.db.ExecContext(ctx,
			"UPDATE documents SET sync_status = $1 WHERE id = $2",
			docSyncStatus, *event.DocumentID)
	}
	return nil
}

func (r *syncRepo) UpdateSyncEventStatus(ctx context.Context, eventID uuid.UUID, status domain.SyncStatus, tallyVoucherID, tallyVoucherNumber, errorMessage string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE sync_events SET
			status = $1, tally_voucher_id = $2, tally_voucher_number = $3, error_message = $4
		WHERE id = $5`,
		status, tallyVoucherID, tallyVoucherNumber, errorMessage, eventID)
	if err != nil {
		return fmt.Errorf("updating sync event status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return domain.ErrNotFound
	}

	// Update document sync_status based on the new event status
	var docSyncStatus domain.DocumentSyncStatus
	switch status {
	case domain.SyncStatusSuccess:
		docSyncStatus = domain.DocumentSyncStatusSynced
	case domain.SyncStatusFailed:
		docSyncStatus = domain.DocumentSyncStatusFailed
	default:
		return nil
	}
	if docSyncStatus == domain.DocumentSyncStatusFailed {
		// Only set failed if no success event exists for this document
		_, _ = r.db.ExecContext(ctx, `
			UPDATE documents SET sync_status = $1
			WHERE id = (SELECT document_id FROM sync_events WHERE id = $2)
				AND sync_status != 'synced'`,
			docSyncStatus, eventID)
	} else {
		_, _ = r.db.ExecContext(ctx, `
			UPDATE documents SET sync_status = $1
			WHERE id = (SELECT document_id FROM sync_events WHERE id = $2)`,
			docSyncStatus, eventID)
	}
	return nil
}

func (r *syncRepo) ListSyncEvents(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.SyncEvent, int, error) {
	var total int
	err := r.db.GetContext(ctx, &total,
		"SELECT COUNT(*) FROM sync_events WHERE tenant_id = $1", tenantID)
	if err != nil {
		return nil, 0, fmt.Errorf("counting sync events: %w", err)
	}

	var events []domain.SyncEvent
	err = r.db.SelectContext(ctx, &events,
		"SELECT * FROM sync_events WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3",
		tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing sync events: %w", err)
	}
	return events, total, nil
}

func (r *syncRepo) ListSyncEventsByDocument(ctx context.Context, tenantID, documentID uuid.UUID, offset, limit int) ([]domain.SyncEvent, int, error) {
	var total int
	err := r.db.GetContext(ctx, &total,
		"SELECT COUNT(*) FROM sync_events WHERE tenant_id = $1 AND document_id = $2", tenantID, documentID)
	if err != nil {
		return nil, 0, fmt.Errorf("counting sync events by document: %w", err)
	}

	var events []domain.SyncEvent
	err = r.db.SelectContext(ctx, &events,
		"SELECT * FROM sync_events WHERE tenant_id = $1 AND document_id = $2 ORDER BY created_at DESC LIMIT $3 OFFSET $4",
		tenantID, documentID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing sync events by document: %w", err)
	}
	return events, total, nil
}

func (r *syncRepo) ListAgents(ctx context.Context, tenantID uuid.UUID) ([]domain.ConnectorAgent, error) {
	var agents []domain.ConnectorAgent
	err := r.db.SelectContext(ctx, &agents,
		"SELECT * FROM connector_agents WHERE tenant_id = $1 ORDER BY registered_at DESC", tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing connector agents: %w", err)
	}
	return agents, nil
}

func (r *syncRepo) ListOutboundDocuments(ctx context.Context, tenantID uuid.UUID, cursor string, limit int) ([]domain.Document, string, error) {
	var docs []domain.Document
	var err error

	if cursor == "" {
		err = r.db.SelectContext(ctx, &docs, `
			SELECT d.* FROM documents d
			WHERE d.tenant_id = $1
				AND d.parsing_status = 'completed'
				AND d.review_status = 'approved'
				AND NOT EXISTS (
					SELECT 1 FROM sync_events se
					WHERE se.document_id = d.id
						AND se.direction = 'outbound'
						AND se.status = 'success'
				)
			ORDER BY d.id ASC
			LIMIT $2`,
			tenantID, limit)
	} else {
		cursorID, parseErr := uuid.Parse(cursor)
		if parseErr != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", parseErr)
		}
		err = r.db.SelectContext(ctx, &docs, `
			SELECT d.* FROM documents d
			WHERE d.tenant_id = $1
				AND d.parsing_status = 'completed'
				AND d.review_status = 'approved'
				AND NOT EXISTS (
					SELECT 1 FROM sync_events se
					WHERE se.document_id = d.id
						AND se.direction = 'outbound'
						AND se.status = 'success'
				)
				AND d.id > $2
			ORDER BY d.id ASC
			LIMIT $3`,
			tenantID, cursorID, limit)
	}
	if err != nil {
		return nil, "", fmt.Errorf("listing outbound documents: %w", err)
	}

	var nextCursor string
	if len(docs) == limit {
		nextCursor = docs[len(docs)-1].ID.String()
	}
	return docs, nextCursor, nil
}
