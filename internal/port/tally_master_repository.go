package port

import (
	"context"

	"github.com/google/uuid"

	"satvos/internal/domain"
)

// TallyMasterRepository handles persistence of Tally master data (ledgers, stock items, etc.).
type TallyMasterRepository interface {
	// Ledgers
	UpsertLedgers(ctx context.Context, tenantID uuid.UUID, ledgers []domain.TallyLedger) error
	ListLedgers(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyLedger, error)
	ListLedgersPaginated(ctx context.Context, tenantID uuid.UUID, parentGroup, taxType, search string, offset, limit int) ([]domain.TallyLedger, int, error)
	FindLedgerByGSTIN(ctx context.Context, tenantID uuid.UUID, gstin string) (*domain.TallyLedger, error)
	FindTaxLedger(ctx context.Context, tenantID uuid.UUID, taxType string, taxRate float64) (*domain.TallyLedger, error)
	FindPurchaseLedger(ctx context.Context, tenantID uuid.UUID) (*domain.TallyLedger, error)

	// Stock items
	UpsertStockItems(ctx context.Context, tenantID uuid.UUID, items []domain.TallyStockItem) error
	ListStockItems(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyStockItem, error)
	ListStockItemsPaginated(ctx context.Context, tenantID uuid.UUID, parentGroup, hsnCode, search string, offset, limit int) ([]domain.TallyStockItem, int, error)
	FindStockItemByHSN(ctx context.Context, tenantID uuid.UUID, hsnCode string) (*domain.TallyStockItem, error)

	// Godowns
	UpsertGodowns(ctx context.Context, tenantID uuid.UUID, godowns []domain.TallyGodown) error
	ListGodowns(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyGodown, error)
	GetDefaultGodown(ctx context.Context, tenantID uuid.UUID) (*domain.TallyGodown, error)

	// Units
	UpsertUnits(ctx context.Context, tenantID uuid.UUID, units []domain.TallyUnit) error
	ListUnits(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyUnit, error)
	FindUnitBySymbol(ctx context.Context, tenantID uuid.UUID, symbol string) (*domain.TallyUnit, error)

	// Cost centers
	UpsertCostCentres(ctx context.Context, tenantID uuid.UUID, centers []domain.TallyCostCentre) error
	ListCostCentres(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyCostCentre, error)
}
