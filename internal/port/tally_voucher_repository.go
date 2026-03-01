package port

import (
	"context"
	"time"

	"github.com/google/uuid"

	"satvos/internal/domain"
)

// TallyVoucherRepository handles persistence of inbound Tally vouchers.
type TallyVoucherRepository interface {
	UpsertVouchers(ctx context.Context, tenantID uuid.UUID, vouchers []domain.TallyVoucher) error
	ListByTenant(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.TallyVoucher, int, error)
	ListByTenantFiltered(ctx context.Context, tenantID uuid.UUID, voucherType, partyGSTIN string, from, to *time.Time, offset, limit int) ([]domain.TallyVoucher, int, error)
	GetByID(ctx context.Context, tenantID, voucherID uuid.UUID) (*domain.TallyVoucher, error)
}
