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

type tallyMasterRepo struct {
	db *sqlx.DB
}

// NewTallyMasterRepo creates a new TallyMasterRepository implementation.
func NewTallyMasterRepo(db *sqlx.DB) port.TallyMasterRepository {
	return &tallyMasterRepo{db: db}
}

// --- Ledgers ---

func (r *tallyMasterRepo) UpsertLedgers(ctx context.Context, tenantID uuid.UUID, ledgers []domain.TallyLedger) error {
	if len(ledgers) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning ledger upsert tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	for i := range ledgers {
		ledgers[i].TenantID = tenantID
		ledgers[i].SyncedAt = now
		if ledgers[i].ID == uuid.Nil {
			ledgers[i].ID = uuid.New()
		}
		_, err := tx.NamedExecContext(ctx, `
			INSERT INTO tally_ledgers (id, tenant_id, name, parent_group, gstin, state, tax_type, tax_rate, is_revenue, synced_at)
			VALUES (:id, :tenant_id, :name, :parent_group, :gstin, :state, :tax_type, :tax_rate, :is_revenue, :synced_at)
			ON CONFLICT (tenant_id, name) DO UPDATE SET
				parent_group = EXCLUDED.parent_group,
				gstin        = EXCLUDED.gstin,
				state        = EXCLUDED.state,
				tax_type     = EXCLUDED.tax_type,
				tax_rate     = EXCLUDED.tax_rate,
				is_revenue   = EXCLUDED.is_revenue,
				synced_at    = EXCLUDED.synced_at`,
			&ledgers[i])
		if err != nil {
			return fmt.Errorf("upserting ledger %q: %w", ledgers[i].Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing ledger upsert tx: %w", err)
	}
	return nil
}

func (r *tallyMasterRepo) ListLedgers(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyLedger, error) {
	var ledgers []domain.TallyLedger
	err := r.db.SelectContext(ctx, &ledgers,
		"SELECT * FROM tally_ledgers WHERE tenant_id = $1 ORDER BY name", tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing ledgers: %w", err)
	}
	return ledgers, nil
}

func (r *tallyMasterRepo) ListLedgersPaginated(ctx context.Context, tenantID uuid.UUID, parentGroup, taxType, search string, offset, limit int) ([]domain.TallyLedger, int, error) {
	where := "WHERE tenant_id = $1"
	args := []interface{}{tenantID}
	n := 2

	if parentGroup != "" {
		where += fmt.Sprintf(" AND parent_group = $%d", n)
		args = append(args, parentGroup)
		n++
	}
	if taxType != "" {
		where += fmt.Sprintf(" AND tax_type = $%d", n)
		args = append(args, taxType)
		n++
	}
	if search != "" {
		where += fmt.Sprintf(" AND (name ILIKE $%d OR gstin ILIKE $%d)", n, n)
		args = append(args, "%"+search+"%")
		n++
	}

	var total int
	err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM tally_ledgers "+where, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting ledgers: %w", err)
	}

	query := fmt.Sprintf("SELECT * FROM tally_ledgers %s ORDER BY name ASC LIMIT $%d OFFSET $%d", where, n, n+1)
	args = append(args, limit, offset)

	var ledgers []domain.TallyLedger
	err = r.db.SelectContext(ctx, &ledgers, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing ledgers paginated: %w", err)
	}
	return ledgers, total, nil
}

func (r *tallyMasterRepo) FindLedgerByGSTIN(ctx context.Context, tenantID uuid.UUID, gstin string) (*domain.TallyLedger, error) {
	var ledger domain.TallyLedger
	err := r.db.GetContext(ctx, &ledger,
		"SELECT * FROM tally_ledgers WHERE tenant_id = $1 AND gstin = $2 LIMIT 1", tenantID, gstin)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding ledger by GSTIN: %w", err)
	}
	return &ledger, nil
}

func (r *tallyMasterRepo) FindTaxLedger(ctx context.Context, tenantID uuid.UUID, taxType string, taxRate float64) (*domain.TallyLedger, error) {
	var ledger domain.TallyLedger
	err := r.db.GetContext(ctx, &ledger,
		"SELECT * FROM tally_ledgers WHERE tenant_id = $1 AND tax_type = $2 AND tax_rate = $3 LIMIT 1",
		tenantID, taxType, taxRate)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding tax ledger: %w", err)
	}
	return &ledger, nil
}

func (r *tallyMasterRepo) FindPurchaseLedger(ctx context.Context, tenantID uuid.UUID) (*domain.TallyLedger, error) {
	var ledger domain.TallyLedger
	err := r.db.GetContext(ctx, &ledger,
		"SELECT * FROM tally_ledgers WHERE tenant_id = $1 AND parent_group = 'Purchase Accounts' LIMIT 1",
		tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding purchase ledger: %w", err)
	}
	return &ledger, nil
}

// --- Stock Items ---

func (r *tallyMasterRepo) UpsertStockItems(ctx context.Context, tenantID uuid.UUID, items []domain.TallyStockItem) error {
	if len(items) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning stock item upsert tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	for i := range items {
		items[i].TenantID = tenantID
		items[i].SyncedAt = now
		if items[i].ID == uuid.Nil {
			items[i].ID = uuid.New()
		}
		_, err := tx.NamedExecContext(ctx, `
			INSERT INTO tally_stock_items (id, tenant_id, name, parent_group, hsn_code, default_uom, synced_at)
			VALUES (:id, :tenant_id, :name, :parent_group, :hsn_code, :default_uom, :synced_at)
			ON CONFLICT (tenant_id, name) DO UPDATE SET
				parent_group = EXCLUDED.parent_group,
				hsn_code     = EXCLUDED.hsn_code,
				default_uom  = EXCLUDED.default_uom,
				synced_at    = EXCLUDED.synced_at`,
			&items[i])
		if err != nil {
			return fmt.Errorf("upserting stock item %q: %w", items[i].Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing stock item upsert tx: %w", err)
	}
	return nil
}

func (r *tallyMasterRepo) ListStockItems(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyStockItem, error) {
	var items []domain.TallyStockItem
	err := r.db.SelectContext(ctx, &items,
		"SELECT * FROM tally_stock_items WHERE tenant_id = $1 ORDER BY name", tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing stock items: %w", err)
	}
	return items, nil
}

func (r *tallyMasterRepo) ListStockItemsPaginated(ctx context.Context, tenantID uuid.UUID, parentGroup, hsnCode, search string, offset, limit int) ([]domain.TallyStockItem, int, error) {
	where := "WHERE tenant_id = $1"
	args := []interface{}{tenantID}
	n := 2

	if parentGroup != "" {
		where += fmt.Sprintf(" AND parent_group = $%d", n)
		args = append(args, parentGroup)
		n++
	}
	if hsnCode != "" {
		where += fmt.Sprintf(" AND hsn_code = $%d", n)
		args = append(args, hsnCode)
		n++
	}
	if search != "" {
		where += fmt.Sprintf(" AND name ILIKE $%d", n)
		args = append(args, "%"+search+"%")
		n++
	}

	var total int
	err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM tally_stock_items "+where, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting stock items: %w", err)
	}

	query := fmt.Sprintf("SELECT * FROM tally_stock_items %s ORDER BY name ASC LIMIT $%d OFFSET $%d", where, n, n+1)
	args = append(args, limit, offset)

	var items []domain.TallyStockItem
	err = r.db.SelectContext(ctx, &items, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing stock items paginated: %w", err)
	}
	return items, total, nil
}

func (r *tallyMasterRepo) FindStockItemByHSN(ctx context.Context, tenantID uuid.UUID, hsnCode string) (*domain.TallyStockItem, error) {
	var item domain.TallyStockItem
	err := r.db.GetContext(ctx, &item,
		"SELECT * FROM tally_stock_items WHERE tenant_id = $1 AND hsn_code = $2 LIMIT 1", tenantID, hsnCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding stock item by HSN: %w", err)
	}
	return &item, nil
}

// --- Godowns ---

func (r *tallyMasterRepo) UpsertGodowns(ctx context.Context, tenantID uuid.UUID, godowns []domain.TallyGodown) error {
	if len(godowns) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning godown upsert tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	for i := range godowns {
		godowns[i].TenantID = tenantID
		godowns[i].SyncedAt = now
		if godowns[i].ID == uuid.Nil {
			godowns[i].ID = uuid.New()
		}
		_, err := tx.NamedExecContext(ctx, `
			INSERT INTO tally_godowns (id, tenant_id, name, parent, synced_at)
			VALUES (:id, :tenant_id, :name, :parent, :synced_at)
			ON CONFLICT (tenant_id, name) DO UPDATE SET
				parent    = EXCLUDED.parent,
				synced_at = EXCLUDED.synced_at`,
			&godowns[i])
		if err != nil {
			return fmt.Errorf("upserting godown %q: %w", godowns[i].Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing godown upsert tx: %w", err)
	}
	return nil
}

func (r *tallyMasterRepo) ListGodowns(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyGodown, error) {
	var godowns []domain.TallyGodown
	err := r.db.SelectContext(ctx, &godowns,
		"SELECT * FROM tally_godowns WHERE tenant_id = $1 ORDER BY name", tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing godowns: %w", err)
	}
	return godowns, nil
}

func (r *tallyMasterRepo) GetDefaultGodown(ctx context.Context, tenantID uuid.UUID) (*domain.TallyGodown, error) {
	var godown domain.TallyGodown
	err := r.db.GetContext(ctx, &godown,
		"SELECT * FROM tally_godowns WHERE tenant_id = $1 ORDER BY name LIMIT 1", tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting default godown: %w", err)
	}
	return &godown, nil
}

// --- Units ---

func (r *tallyMasterRepo) UpsertUnits(ctx context.Context, tenantID uuid.UUID, units []domain.TallyUnit) error {
	if len(units) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning unit upsert tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	for i := range units {
		units[i].TenantID = tenantID
		units[i].SyncedAt = now
		if units[i].ID == uuid.Nil {
			units[i].ID = uuid.New()
		}
		_, err := tx.NamedExecContext(ctx, `
			INSERT INTO tally_units (id, tenant_id, symbol, formal_name, synced_at)
			VALUES (:id, :tenant_id, :symbol, :formal_name, :synced_at)
			ON CONFLICT (tenant_id, symbol) DO UPDATE SET
				formal_name = EXCLUDED.formal_name,
				synced_at   = EXCLUDED.synced_at`,
			&units[i])
		if err != nil {
			return fmt.Errorf("upserting unit %q: %w", units[i].Symbol, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing unit upsert tx: %w", err)
	}
	return nil
}

func (r *tallyMasterRepo) ListUnits(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyUnit, error) {
	var units []domain.TallyUnit
	err := r.db.SelectContext(ctx, &units,
		"SELECT * FROM tally_units WHERE tenant_id = $1 ORDER BY symbol", tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing units: %w", err)
	}
	return units, nil
}

func (r *tallyMasterRepo) FindUnitBySymbol(ctx context.Context, tenantID uuid.UUID, symbol string) (*domain.TallyUnit, error) {
	var unit domain.TallyUnit
	err := r.db.GetContext(ctx, &unit,
		"SELECT * FROM tally_units WHERE tenant_id = $1 AND symbol = $2 LIMIT 1", tenantID, symbol)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding unit by symbol: %w", err)
	}
	return &unit, nil
}

// --- Cost Centers ---

//nolint:misspell // interface method name uses British spelling
func (r *tallyMasterRepo) UpsertCostCentres(ctx context.Context, tenantID uuid.UUID, centers []domain.TallyCostCentre) error {
	if len(centers) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning cost center upsert tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	for i := range centers {
		centers[i].TenantID = tenantID
		centers[i].SyncedAt = now
		if centers[i].ID == uuid.Nil {
			centers[i].ID = uuid.New()
		}
		_, err := tx.NamedExecContext(ctx, `
			INSERT INTO tally_cost_centres (id, tenant_id, name, parent, synced_at)
			VALUES (:id, :tenant_id, :name, :parent, :synced_at)
			ON CONFLICT (tenant_id, name) DO UPDATE SET
				parent    = EXCLUDED.parent,
				synced_at = EXCLUDED.synced_at`,
			&centers[i])
		if err != nil {
			return fmt.Errorf("upserting cost center %q: %w", centers[i].Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing cost center upsert tx: %w", err)
	}
	return nil
}

//nolint:misspell // interface method name uses British spelling
func (r *tallyMasterRepo) ListCostCentres(ctx context.Context, tenantID uuid.UUID) ([]domain.TallyCostCentre, error) {
	var centers []domain.TallyCostCentre //nolint:misspell // domain type uses British spelling
	err := r.db.SelectContext(ctx, &centers,
		"SELECT * FROM tally_cost_centres WHERE tenant_id = $1 ORDER BY name", tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing cost centers: %w", err)
	}
	return centers, nil
}
