package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/port"
)

// VoucherBuilderService builds smart-matched VoucherDefs from parsed documents.
// Defined in voucher_builder.go
type VoucherBuilderService interface {
	Build(ctx context.Context, tenantID uuid.UUID, doc *domain.Document) (*domain.VoucherDef, error)
	BuildWithOverrides(ctx context.Context, tenantID uuid.UUID, doc *domain.Document, overrides *domain.VoucherOverrides) (*domain.VoucherDef, error)
}

// MasterPayload groups all Tally master data types for bulk save.
type MasterPayload struct {
	Ledgers     []domain.TallyLedger     `json:"ledgers"`
	StockItems  []domain.TallyStockItem  `json:"stock_items"`
	Godowns     []domain.TallyGodown     `json:"godowns"`
	Units       []domain.TallyUnit       `json:"units"`
	CostCentres []domain.TallyCostCentre `json:"cost_centres"` //nolint:misspell // Tally uses British spelling
}

// OutboundItem is a document queued for export to Tally, enriched with a VoucherDef.
type OutboundItem struct {
	DocumentID     uuid.UUID          `json:"document_id"`
	StructuredData json.RawMessage    `json:"structured_data"`
	VoucherDef     *domain.VoucherDef `json:"voucher_def"`
	SyncEventID    uuid.UUID          `json:"sync_event_id"`
}

// AckResult is the agent's acknowledgement of a single outbound sync attempt.
type AckResult struct {
	SyncEventID        uuid.UUID `json:"sync_event_id"`
	DocumentID         uuid.UUID `json:"document_id"`
	Success            bool      `json:"success"`
	TallyVoucherID     string    `json:"tally_voucher_id,omitempty"`
	TallyVoucherNumber string    `json:"tally_voucher_number,omitempty"`
	ErrorMessage       string    `json:"error_message,omitempty"`
}

// SyncService manages connector agent lifecycle and bi-directional sync.
type SyncService interface {
	Register(ctx context.Context, tenantID, serviceAccountID uuid.UUID, version, osInfo string) (*domain.ConnectorAgent, error)
	Heartbeat(ctx context.Context, tenantID, serviceAccountID uuid.UUID, tallyConnected bool, tallyCompany string, tallyPort int, version string, agentErrors []string) error
	SaveMasters(ctx context.Context, tenantID uuid.UUID, masters *MasterPayload) error
	ListOutbound(ctx context.Context, tenantID, serviceAccountID uuid.UUID, cursor string, limit int) ([]OutboundItem, string, error)
	AckOutbound(ctx context.Context, tenantID, serviceAccountID uuid.UUID, results []AckResult) error
	SaveInbound(ctx context.Context, tenantID uuid.UUID, vouchers []domain.TallyVoucher) error
}

type syncService struct {
	syncRepo       port.SyncRepository
	masterRepo     port.TallyMasterRepository
	voucherRepo    port.TallyVoucherRepository
	voucherBuilder VoucherBuilderService
}

// NewSyncService creates a new SyncService implementation.
func NewSyncService(
	syncRepo port.SyncRepository,
	masterRepo port.TallyMasterRepository,
	voucherRepo port.TallyVoucherRepository,
	voucherBuilder VoucherBuilderService,
) SyncService {
	return &syncService{
		syncRepo:       syncRepo,
		masterRepo:     masterRepo,
		voucherRepo:    voucherRepo,
		voucherBuilder: voucherBuilder,
	}
}

func (s *syncService) Register(ctx context.Context, tenantID, serviceAccountID uuid.UUID, version, osInfo string) (*domain.ConnectorAgent, error) {
	existing, err := s.syncRepo.GetAgentByServiceAccount(ctx, tenantID, serviceAccountID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, domain.ErrAgentNotFound) {
		return nil, err
	}

	agent := &domain.ConnectorAgent{
		TenantID:         tenantID,
		ServiceAccountID: serviceAccountID,
		AgentVersion:     version,
		OSInfo:           osInfo,
		Status:           domain.AgentStatusRegistered,
	}
	if err := s.syncRepo.RegisterAgent(ctx, agent); err != nil {
		return nil, err
	}
	return agent, nil
}

func (s *syncService) Heartbeat(ctx context.Context, tenantID, serviceAccountID uuid.UUID, tallyConnected bool, tallyCompany string, tallyPort int, version string, agentErrors []string) error {
	agent, err := s.syncRepo.GetAgentByServiceAccount(ctx, tenantID, serviceAccountID)
	if err != nil {
		return err
	}

	status := domain.AgentStatusOffline
	if tallyConnected {
		status = domain.AgentStatusOnline
	}

	for _, agentErr := range agentErrors {
		log.Printf("WARNING: connector agent %s reported error: %s", agent.ID, agentErr)
	}

	return s.syncRepo.UpdateHeartbeat(ctx, agent.ID, status, tallyCompany, tallyPort, version)
}

func (s *syncService) SaveMasters(ctx context.Context, tenantID uuid.UUID, masters *MasterPayload) error {
	if len(masters.Ledgers) > 0 {
		if err := s.masterRepo.UpsertLedgers(ctx, tenantID, masters.Ledgers); err != nil {
			return err
		}
	}
	if len(masters.StockItems) > 0 {
		if err := s.masterRepo.UpsertStockItems(ctx, tenantID, masters.StockItems); err != nil {
			return err
		}
	}
	if len(masters.Godowns) > 0 {
		if err := s.masterRepo.UpsertGodowns(ctx, tenantID, masters.Godowns); err != nil {
			return err
		}
	}
	if len(masters.Units) > 0 {
		if err := s.masterRepo.UpsertUnits(ctx, tenantID, masters.Units); err != nil {
			return err
		}
	}
	if len(masters.CostCentres) > 0 {
		if err := s.masterRepo.UpsertCostCentres(ctx, tenantID, masters.CostCentres); err != nil {
			return err
		}
	}
	return nil
}

func (s *syncService) ListOutbound(ctx context.Context, tenantID, serviceAccountID uuid.UUID, cursor string, limit int) ([]OutboundItem, string, error) {
	if limit < 1 || limit > 50 {
		limit = 50
	}

	agent, err := s.syncRepo.GetAgentByServiceAccount(ctx, tenantID, serviceAccountID)
	if err != nil {
		return nil, "", err
	}

	docs, nextCursor, err := s.syncRepo.ListOutboundDocuments(ctx, tenantID, cursor, limit)
	if err != nil {
		return nil, "", err
	}

	items := make([]OutboundItem, 0, len(docs))
	for i := range docs {
		doc := &docs[i]

		// Apply voucher overrides if present
		var overrides *domain.VoucherOverrides
		if doc.VoucherOverrides != nil && len(*doc.VoucherOverrides) > 0 {
			overrides = &domain.VoucherOverrides{}
			if jsonErr := json.Unmarshal(*doc.VoucherOverrides, overrides); jsonErr != nil {
				log.Printf("WARNING: syncService.ListOutbound: failed to unmarshal voucher overrides for doc %s: %v", doc.ID, jsonErr)
			}
		}

		vDef, buildErr := s.voucherBuilder.BuildWithOverrides(ctx, tenantID, doc, overrides)
		if buildErr != nil {
			log.Printf("WARNING: syncService.ListOutbound: failed to build voucher def for doc %s: %v", doc.ID, buildErr)
			continue
		}

		eventID := uuid.New()
		event := &domain.SyncEvent{
			ID:         eventID,
			TenantID:   tenantID,
			AgentID:    agent.ID,
			DocumentID: &doc.ID,
			Direction:  domain.SyncDirectionOutbound,
			Status:     domain.SyncStatusPending,
		}
		if createErr := s.syncRepo.CreateSyncEvent(ctx, event); createErr != nil {
			log.Printf("WARNING: syncService.ListOutbound: failed to create sync event for doc %s: %v", doc.ID, createErr)
			continue
		}

		items = append(items, OutboundItem{
			DocumentID:     doc.ID,
			StructuredData: doc.StructuredData,
			VoucherDef:     vDef,
			SyncEventID:    eventID,
		})
	}

	return items, nextCursor, nil
}

func (s *syncService) AckOutbound(ctx context.Context, tenantID, serviceAccountID uuid.UUID, results []AckResult) error {
	for i := range results {
		r := &results[i]
		status := domain.SyncStatusFailed
		if r.Success {
			status = domain.SyncStatusSuccess
		}
		if err := s.syncRepo.UpdateSyncEventStatus(ctx, r.SyncEventID, status, r.TallyVoucherID, r.TallyVoucherNumber, r.ErrorMessage); err != nil {
			return err
		}
	}
	return nil
}

func (s *syncService) SaveInbound(ctx context.Context, tenantID uuid.UUID, vouchers []domain.TallyVoucher) error {
	return s.voucherRepo.UpsertVouchers(ctx, tenantID, vouchers)
}
