package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
)

// MockTallyMasterRepo is a mock implementation of port.TallyMasterRepository.
type MockTallyMasterRepo struct {
	mock.Mock
}

func (m *MockTallyMasterRepo) UpsertLedgers(ctx context.Context, tenantID uuid.UUID, ledgers []domain.TallyLedger) error {
	args := m.Called(ctx, tenantID, ledgers)
	return args.Error(0)
}

func (m *MockTallyMasterRepo) ListLedgers(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyLedger, error) {
	args := m.Called(ctx, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.TallyLedger), args.Error(1)
}

func (m *MockTallyMasterRepo) ListLedgersPaginated(ctx context.Context, tenantID uuid.UUID, parentGroup, taxType, search string, offset, limit int) ([]domain.TallyLedger, int, error) {
	args := m.Called(ctx, tenantID, parentGroup, taxType, search, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.TallyLedger), args.Int(1), args.Error(2)
}

func (m *MockTallyMasterRepo) FindLedgerByGSTIN(ctx context.Context, tenantID uuid.UUID, gstin string) (*domain.TallyLedger, error) {
	args := m.Called(ctx, tenantID, gstin)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.TallyLedger), args.Error(1)
}

func (m *MockTallyMasterRepo) FindTaxLedger(ctx context.Context, tenantID uuid.UUID, taxType string, taxRate float64) (*domain.TallyLedger, error) {
	args := m.Called(ctx, tenantID, taxType, taxRate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.TallyLedger), args.Error(1)
}

func (m *MockTallyMasterRepo) FindPurchaseLedger(ctx context.Context, tenantID uuid.UUID) (*domain.TallyLedger, error) {
	args := m.Called(ctx, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.TallyLedger), args.Error(1)
}

func (m *MockTallyMasterRepo) UpsertStockItems(ctx context.Context, tenantID uuid.UUID, items []domain.TallyStockItem) error {
	args := m.Called(ctx, tenantID, items)
	return args.Error(0)
}

func (m *MockTallyMasterRepo) ListStockItems(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyStockItem, error) {
	args := m.Called(ctx, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.TallyStockItem), args.Error(1)
}

func (m *MockTallyMasterRepo) ListStockItemsPaginated(ctx context.Context, tenantID uuid.UUID, parentGroup, hsnCode, search string, offset, limit int) ([]domain.TallyStockItem, int, error) {
	args := m.Called(ctx, tenantID, parentGroup, hsnCode, search, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.TallyStockItem), args.Int(1), args.Error(2)
}

func (m *MockTallyMasterRepo) FindStockItemByHSN(ctx context.Context, tenantID uuid.UUID, hsnCode string) (*domain.TallyStockItem, error) {
	args := m.Called(ctx, tenantID, hsnCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.TallyStockItem), args.Error(1)
}

func (m *MockTallyMasterRepo) UpsertGodowns(ctx context.Context, tenantID uuid.UUID, godowns []domain.TallyGodown) error {
	args := m.Called(ctx, tenantID, godowns)
	return args.Error(0)
}

func (m *MockTallyMasterRepo) ListGodowns(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyGodown, error) {
	args := m.Called(ctx, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.TallyGodown), args.Error(1)
}

func (m *MockTallyMasterRepo) GetDefaultGodown(ctx context.Context, tenantID uuid.UUID) (*domain.TallyGodown, error) {
	args := m.Called(ctx, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.TallyGodown), args.Error(1)
}

func (m *MockTallyMasterRepo) UpsertUnits(ctx context.Context, tenantID uuid.UUID, units []domain.TallyUnit) error {
	args := m.Called(ctx, tenantID, units)
	return args.Error(0)
}

func (m *MockTallyMasterRepo) ListUnits(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyUnit, error) {
	args := m.Called(ctx, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.TallyUnit), args.Error(1)
}

func (m *MockTallyMasterRepo) FindUnitBySymbol(ctx context.Context, tenantID uuid.UUID, symbol string) (*domain.TallyUnit, error) {
	args := m.Called(ctx, tenantID, symbol)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.TallyUnit), args.Error(1)
}

func (m *MockTallyMasterRepo) UpsertCostCentres(ctx context.Context, tenantID uuid.UUID, centers []domain.TallyCostCentre) error {
	args := m.Called(ctx, tenantID, centers)
	return args.Error(0)
}

func (m *MockTallyMasterRepo) ListCostCentres(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyCostCentre, error) {
	args := m.Called(ctx, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.TallyCostCentre), args.Error(1)
}
