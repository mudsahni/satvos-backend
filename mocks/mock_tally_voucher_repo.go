package mocks

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
)

// MockTallyVoucherRepo is a mock implementation of port.TallyVoucherRepository.
type MockTallyVoucherRepo struct {
	mock.Mock
}

func (m *MockTallyVoucherRepo) UpsertVouchers(ctx context.Context, tenantID uuid.UUID, vouchers []domain.TallyVoucher) error {
	args := m.Called(ctx, tenantID, vouchers)
	return args.Error(0)
}

func (m *MockTallyVoucherRepo) ListByTenant(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.TallyVoucher, int, error) {
	args := m.Called(ctx, tenantID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.TallyVoucher), args.Int(1), args.Error(2)
}

func (m *MockTallyVoucherRepo) ListByTenantFiltered(ctx context.Context, tenantID uuid.UUID, voucherType, partyGSTIN string, from, to *time.Time, offset, limit int) ([]domain.TallyVoucher, int, error) {
	args := m.Called(ctx, tenantID, voucherType, partyGSTIN, from, to, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.TallyVoucher), args.Int(1), args.Error(2)
}

func (m *MockTallyVoucherRepo) GetByID(ctx context.Context, tenantID, voucherID uuid.UUID) (*domain.TallyVoucher, error) {
	args := m.Called(ctx, tenantID, voucherID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.TallyVoucher), args.Error(1)
}
