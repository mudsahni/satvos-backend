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

type tallyVoucherRepo struct {
	db *sqlx.DB
}

// NewTallyVoucherRepo creates a new TallyVoucherRepository implementation.
func NewTallyVoucherRepo(db *sqlx.DB) port.TallyVoucherRepository {
	return &tallyVoucherRepo{db: db}
}

func (r *tallyVoucherRepo) UpsertVouchers(ctx context.Context, tenantID uuid.UUID, vouchers []domain.TallyVoucher) error {
	if len(vouchers) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for i := range vouchers {
		vouchers[i].TenantID = tenantID
		vouchers[i].SyncedAt = time.Now().UTC()
		if vouchers[i].ID == uuid.Nil {
			vouchers[i].ID = uuid.New()
		}
		_, err := tx.NamedExecContext(ctx, `
			INSERT INTO tally_vouchers (id, tenant_id, voucher_type, voucher_number, voucher_date,
				party_name, party_gstin, amount, narration, ledger_entries, remote_id, tally_master_id, synced_at)
			VALUES (:id, :tenant_id, :voucher_type, :voucher_number, :voucher_date,
				:party_name, :party_gstin, :amount, :narration, :ledger_entries, :remote_id, :tally_master_id, :synced_at)
			ON CONFLICT (tenant_id, tally_master_id) DO UPDATE SET
				voucher_type = EXCLUDED.voucher_type,
				voucher_number = EXCLUDED.voucher_number,
				voucher_date = EXCLUDED.voucher_date,
				party_name = EXCLUDED.party_name,
				party_gstin = EXCLUDED.party_gstin,
				amount = EXCLUDED.amount,
				narration = EXCLUDED.narration,
				ledger_entries = EXCLUDED.ledger_entries,
				remote_id = EXCLUDED.remote_id,
				synced_at = EXCLUDED.synced_at`,
			&vouchers[i])
		if err != nil {
			return fmt.Errorf("upserting tally voucher: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

func (r *tallyVoucherRepo) ListByTenant(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.TallyVoucher, int, error) {
	var total int
	err := r.db.GetContext(ctx, &total,
		"SELECT COUNT(*) FROM tally_vouchers WHERE tenant_id = $1", tenantID)
	if err != nil {
		return nil, 0, fmt.Errorf("counting tally vouchers: %w", err)
	}

	var vouchers []domain.TallyVoucher
	err = r.db.SelectContext(ctx, &vouchers,
		"SELECT * FROM tally_vouchers WHERE tenant_id = $1 ORDER BY synced_at DESC LIMIT $2 OFFSET $3",
		tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing tally vouchers: %w", err)
	}
	return vouchers, total, nil
}

func (r *tallyVoucherRepo) GetByID(ctx context.Context, tenantID, voucherID uuid.UUID) (*domain.TallyVoucher, error) {
	var voucher domain.TallyVoucher
	err := r.db.GetContext(ctx, &voucher,
		"SELECT * FROM tally_vouchers WHERE id = $1 AND tenant_id = $2", voucherID, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("getting tally voucher: %w", err)
	}
	return &voucher, nil
}
