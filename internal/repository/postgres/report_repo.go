package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"satvos/internal/domain"
	"satvos/internal/port"
)

type reportRepo struct {
	db *sqlx.DB
}

// NewReportRepo creates a new PostgreSQL-backed ReportRepository.
func NewReportRepo(db *sqlx.DB) port.ReportRepository {
	return &reportRepo{db: db}
}

// buildWhereClause constructs a dynamic WHERE clause for document_summaries queries.
// It returns the clause string (starting with "WHERE") and the positional arguments.
func buildWhereClause(tenantID uuid.UUID, filters *domain.ReportFilters) (clause string, args []interface{}) {
	args = []interface{}{tenantID}
	clause = "WHERE ds.tenant_id = $1"
	argN := 2

	if filters.From != nil {
		clause += fmt.Sprintf(" AND ds.invoice_date >= $%d", argN)
		args = append(args, *filters.From)
		argN++
	}
	if filters.To != nil {
		clause += fmt.Sprintf(" AND ds.invoice_date <= $%d", argN)
		args = append(args, *filters.To)
		argN++
	}
	if filters.CollectionID != nil {
		clause += fmt.Sprintf(" AND ds.collection_id = $%d", argN)
		args = append(args, *filters.CollectionID)
		argN++
	}
	if filters.SellerGSTIN != "" {
		clause += fmt.Sprintf(" AND ds.seller_gstin = $%d", argN)
		args = append(args, filters.SellerGSTIN)
		argN++
	}
	if filters.BuyerGSTIN != "" {
		clause += fmt.Sprintf(" AND ds.buyer_gstin = $%d", argN)
		args = append(args, filters.BuyerGSTIN)
		argN++
	}

	// Viewer and free roles need collection permission scoping
	if filters.UserRole != domain.RoleAdmin && filters.UserRole != domain.RoleManager && filters.UserRole != domain.RoleMember {
		clause += fmt.Sprintf(" AND ds.collection_id IN (SELECT collection_id FROM collection_permissions WHERE user_id = $%d)", argN)
		args = append(args, filters.UserID)
		argN++ //nolint:ineffassign // argN kept incremented for consistency
	}

	return clause, args
}

// dateTruncExpr returns the PostgreSQL date_trunc expression for the given granularity.
func dateTruncExpr(granularity string) string {
	switch granularity {
	case "daily":
		return "date_trunc('day', ds.invoice_date)"
	case "weekly":
		return "date_trunc('week', ds.invoice_date)"
	case "monthly":
		return "date_trunc('month', ds.invoice_date)"
	case "quarterly":
		return "date_trunc('quarter', ds.invoice_date)"
	case "yearly":
		return "date_trunc('year', ds.invoice_date)"
	default:
		return "date_trunc('month', ds.invoice_date)"
	}
}

// formatPeriod formats a time.Time into a human-readable period label based on granularity.
func formatPeriod(t time.Time, granularity string) string {
	switch granularity {
	case "daily":
		return t.Format("2006-01-02")
	case "weekly":
		_, week := t.ISOWeek()
		return fmt.Sprintf("%s-W%02d", t.Format("2006"), week)
	case "monthly":
		return t.Format("2006-01")
	case "quarterly":
		quarter := (int(t.Month())-1)/3 + 1
		return fmt.Sprintf("%s-Q%d", t.Format("2006"), quarter)
	case "yearly":
		return t.Format("2006")
	default:
		return t.Format("2006-01")
	}
}

// periodEnd computes the end of a period given its start and the granularity.
func periodEnd(start time.Time, granularity string) time.Time {
	switch granularity {
	case "daily":
		return start.AddDate(0, 0, 1).Add(-time.Second)
	case "weekly":
		return start.AddDate(0, 0, 7).Add(-time.Second)
	case "monthly":
		return start.AddDate(0, 1, 0).Add(-time.Second)
	case "quarterly":
		return start.AddDate(0, 3, 0).Add(-time.Second)
	case "yearly":
		return start.AddDate(1, 0, 0).Add(-time.Second)
	default:
		return start.AddDate(0, 1, 0).Add(-time.Second)
	}
}

// partySummaryRow is a generic intermediate scan target for seller/buyer summary queries.
type partySummaryRow struct {
	PartyGSTIN          string     `db:"party_gstin"`
	PartyName           string     `db:"party_name"`
	PartyState          string     `db:"party_state"`
	InvoiceCount        int        `db:"invoice_count"`
	TotalAmount         float64    `db:"total_amount"`
	TotalTax            float64    `db:"total_tax"`
	CGST                float64    `db:"cgst"`
	SGST                float64    `db:"sgst"`
	IGST                float64    `db:"igst"`
	AverageInvoiceValue float64    `db:"average_invoice_value"`
	FirstInvoiceDate    *time.Time `db:"first_invoice_date"`
	LastInvoiceDate     *time.Time `db:"last_invoice_date"`
}

// partySummaryQuery runs a parameterized party summary query using the specified column prefix ("seller" or "buyer").
func (r *reportRepo) partySummaryQuery(ctx context.Context, tenantID uuid.UUID, filters *domain.ReportFilters, partyPrefix string) ([]partySummaryRow, int, error) {
	whereClause, args := buildWhereClause(tenantID, filters)

	dataQuery := fmt.Sprintf(`SELECT
		%[1]s_gstin AS party_gstin, MAX(%[1]s_name) AS party_name, MAX(%[1]s_state) AS party_state,
		COUNT(*) AS invoice_count, COALESCE(SUM(total_amount), 0) AS total_amount,
		COALESCE(SUM(cgst + sgst + igst), 0) AS total_tax,
		COALESCE(SUM(cgst), 0) AS cgst, COALESCE(SUM(sgst), 0) AS sgst, COALESCE(SUM(igst), 0) AS igst,
		COALESCE(AVG(total_amount), 0) AS average_invoice_value,
		MIN(invoice_date) AS first_invoice_date, MAX(invoice_date) AS last_invoice_date
	FROM document_summaries ds
	%[2]s
	AND %[1]s_gstin IS NOT NULL AND %[1]s_gstin != ''
	GROUP BY %[1]s_gstin
	ORDER BY total_amount DESC
	OFFSET %[3]d LIMIT %[4]d`, partyPrefix, whereClause, filters.Offset, filters.Limit)

	var rows []partySummaryRow
	if err := sqlx.SelectContext(ctx, r.db, &rows, dataQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("reportRepo.%sSummary data: %w", partyPrefix, err)
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(DISTINCT %[1]s_gstin)
	FROM document_summaries ds
	%[2]s
	AND %[1]s_gstin IS NOT NULL AND %[1]s_gstin != ''`, partyPrefix, whereClause)

	var total int
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("reportRepo.%sSummary count: %w", partyPrefix, err)
	}

	return rows, total, nil
}

func (r *reportRepo) SellerSummary(ctx context.Context, tenantID uuid.UUID, filters *domain.ReportFilters) ([]domain.SellerSummaryRow, int, error) {
	partyRows, total, err := r.partySummaryQuery(ctx, tenantID, filters, "seller")
	if err != nil {
		return nil, 0, err
	}
	rows := make([]domain.SellerSummaryRow, len(partyRows))
	for i := range partyRows {
		rows[i] = domain.SellerSummaryRow{
			SellerGSTIN:         partyRows[i].PartyGSTIN,
			SellerName:          partyRows[i].PartyName,
			SellerState:         partyRows[i].PartyState,
			InvoiceCount:        partyRows[i].InvoiceCount,
			TotalAmount:         partyRows[i].TotalAmount,
			TotalTax:            partyRows[i].TotalTax,
			CGST:                partyRows[i].CGST,
			SGST:                partyRows[i].SGST,
			IGST:                partyRows[i].IGST,
			AverageInvoiceValue: partyRows[i].AverageInvoiceValue,
			FirstInvoiceDate:    partyRows[i].FirstInvoiceDate,
			LastInvoiceDate:     partyRows[i].LastInvoiceDate,
		}
	}
	return rows, total, nil
}

func (r *reportRepo) BuyerSummary(ctx context.Context, tenantID uuid.UUID, filters *domain.ReportFilters) ([]domain.BuyerSummaryRow, int, error) {
	partyRows, total, err := r.partySummaryQuery(ctx, tenantID, filters, "buyer")
	if err != nil {
		return nil, 0, err
	}
	rows := make([]domain.BuyerSummaryRow, len(partyRows))
	for i := range partyRows {
		rows[i] = domain.BuyerSummaryRow{
			BuyerGSTIN:          partyRows[i].PartyGSTIN,
			BuyerName:           partyRows[i].PartyName,
			BuyerState:          partyRows[i].PartyState,
			InvoiceCount:        partyRows[i].InvoiceCount,
			TotalAmount:         partyRows[i].TotalAmount,
			TotalTax:            partyRows[i].TotalTax,
			CGST:                partyRows[i].CGST,
			SGST:                partyRows[i].SGST,
			IGST:                partyRows[i].IGST,
			AverageInvoiceValue: partyRows[i].AverageInvoiceValue,
			FirstInvoiceDate:    partyRows[i].FirstInvoiceDate,
			LastInvoiceDate:     partyRows[i].LastInvoiceDate,
		}
	}
	return rows, total, nil
}

func (r *reportRepo) PartyLedger(ctx context.Context, tenantID uuid.UUID, gstin string, filters *domain.ReportFilters) ([]domain.PartyLedgerRow, int, error) {
	whereClause, args := buildWhereClause(tenantID, filters)

	// Add GSTIN filter for the party
	argN := len(args) + 1
	gstinParam := fmt.Sprintf("$%d", argN)
	args = append(args, gstin)
	partyFilter := fmt.Sprintf(" AND (ds.seller_gstin = %s OR ds.buyer_gstin = %s)", gstinParam, gstinParam)

	dataQuery := fmt.Sprintf(`SELECT
		ds.document_id,
		ds.invoice_number,
		ds.invoice_date,
		ds.invoice_type,
		CASE WHEN ds.seller_gstin = %s THEN 'seller' ELSE 'buyer' END AS role,
		CASE WHEN ds.seller_gstin = %s THEN ds.buyer_name ELSE ds.seller_name END AS counterparty_name,
		CASE WHEN ds.seller_gstin = %s THEN ds.buyer_gstin ELSE ds.seller_gstin END AS counterparty_gstin,
		ds.subtotal,
		ds.taxable_amount,
		ds.cgst,
		ds.sgst,
		ds.igst,
		ds.total_amount,
		ds.validation_status,
		ds.review_status
	FROM document_summaries ds
	%s
	%s
	ORDER BY ds.invoice_date ASC
	OFFSET %d LIMIT %d`, gstinParam, gstinParam, gstinParam, whereClause, partyFilter, filters.Offset, filters.Limit)

	var rows []domain.PartyLedgerRow
	if err := sqlx.SelectContext(ctx, r.db, &rows, dataQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("reportRepo.PartyLedger data: %w", err)
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(*)
	FROM document_summaries ds
	%s
	%s`, whereClause, partyFilter)

	var total int
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("reportRepo.PartyLedger count: %w", err)
	}

	return rows, total, nil
}

// financialSummaryDBRow is an intermediate struct for scanning time-series results.
type financialSummaryDBRow struct {
	PeriodStart   time.Time `db:"period_start"`
	InvoiceCount  int       `db:"invoice_count"`
	Subtotal      float64   `db:"subtotal"`
	TaxableAmount float64   `db:"taxable_amount"`
	CGST          float64   `db:"cgst"`
	SGST          float64   `db:"sgst"`
	IGST          float64   `db:"igst"`
	Cess          float64   `db:"cess"`
	TotalAmount   float64   `db:"total_amount"`
}

func (r *reportRepo) FinancialSummary(ctx context.Context, tenantID uuid.UUID, filters *domain.ReportFilters) ([]domain.FinancialSummaryRow, error) {
	whereClause, args := buildWhereClause(tenantID, filters)
	truncExpr := dateTruncExpr(filters.Granularity)

	query := fmt.Sprintf(`SELECT
		%s AS period_start,
		COUNT(*) AS invoice_count,
		COALESCE(SUM(subtotal), 0) AS subtotal,
		COALESCE(SUM(taxable_amount), 0) AS taxable_amount,
		COALESCE(SUM(cgst), 0) AS cgst,
		COALESCE(SUM(sgst), 0) AS sgst,
		COALESCE(SUM(igst), 0) AS igst,
		COALESCE(SUM(cess), 0) AS cess,
		COALESCE(SUM(total_amount), 0) AS total_amount
	FROM document_summaries ds
	%s
	AND ds.invoice_date IS NOT NULL
	GROUP BY period_start
	ORDER BY period_start ASC`, truncExpr, whereClause)

	var dbRows []financialSummaryDBRow
	if err := sqlx.SelectContext(ctx, r.db, &dbRows, query, args...); err != nil {
		return nil, fmt.Errorf("reportRepo.FinancialSummary: %w", err)
	}

	granularity := filters.Granularity
	if granularity == "" {
		granularity = "monthly"
	}

	rows := make([]domain.FinancialSummaryRow, len(dbRows))
	for i := range dbRows {
		rows[i] = domain.FinancialSummaryRow{
			Period:        formatPeriod(dbRows[i].PeriodStart, granularity),
			PeriodStart:   dbRows[i].PeriodStart,
			PeriodEnd:     periodEnd(dbRows[i].PeriodStart, granularity),
			InvoiceCount:  dbRows[i].InvoiceCount,
			Subtotal:      dbRows[i].Subtotal,
			TaxableAmount: dbRows[i].TaxableAmount,
			CGST:          dbRows[i].CGST,
			SGST:          dbRows[i].SGST,
			IGST:          dbRows[i].IGST,
			Cess:          dbRows[i].Cess,
			TotalAmount:   dbRows[i].TotalAmount,
		}
	}

	return rows, nil
}

// taxSummaryDBRow is an intermediate struct for scanning tax time-series results.
type taxSummaryDBRow struct {
	PeriodStart       time.Time `db:"period_start"`
	IntrastateCount   int       `db:"intrastate_count"`
	IntrastateTaxable float64   `db:"intrastate_taxable"`
	CGST              float64   `db:"cgst"`
	SGST              float64   `db:"sgst"`
	InterstateCount   int       `db:"interstate_count"`
	InterstateTaxable float64   `db:"interstate_taxable"`
	IGST              float64   `db:"igst"`
	Cess              float64   `db:"cess"`
	TotalTax          float64   `db:"total_tax"`
}

func (r *reportRepo) TaxSummary(ctx context.Context, tenantID uuid.UUID, filters *domain.ReportFilters) ([]domain.TaxSummaryRow, error) {
	whereClause, args := buildWhereClause(tenantID, filters)
	truncExpr := dateTruncExpr(filters.Granularity)

	query := fmt.Sprintf(`SELECT
		%s AS period_start,
		COUNT(CASE WHEN igst = 0 THEN 1 END) AS intrastate_count,
		COALESCE(SUM(CASE WHEN igst = 0 THEN taxable_amount ELSE 0 END), 0) AS intrastate_taxable,
		COALESCE(SUM(cgst), 0) AS cgst,
		COALESCE(SUM(sgst), 0) AS sgst,
		COUNT(CASE WHEN igst > 0 THEN 1 END) AS interstate_count,
		COALESCE(SUM(CASE WHEN igst > 0 THEN taxable_amount ELSE 0 END), 0) AS interstate_taxable,
		COALESCE(SUM(igst), 0) AS igst,
		COALESCE(SUM(cess), 0) AS cess,
		COALESCE(SUM(cgst + sgst + igst + cess), 0) AS total_tax
	FROM document_summaries ds
	%s
	AND ds.invoice_date IS NOT NULL
	GROUP BY period_start
	ORDER BY period_start ASC`, truncExpr, whereClause)

	var dbRows []taxSummaryDBRow
	if err := sqlx.SelectContext(ctx, r.db, &dbRows, query, args...); err != nil {
		return nil, fmt.Errorf("reportRepo.TaxSummary: %w", err)
	}

	granularity := filters.Granularity
	if granularity == "" {
		granularity = "monthly"
	}

	rows := make([]domain.TaxSummaryRow, len(dbRows))
	for i := range dbRows {
		rows[i] = domain.TaxSummaryRow{
			Period:            formatPeriod(dbRows[i].PeriodStart, granularity),
			PeriodStart:       dbRows[i].PeriodStart,
			PeriodEnd:         periodEnd(dbRows[i].PeriodStart, granularity),
			IntrastateCount:   dbRows[i].IntrastateCount,
			IntrastateTaxable: dbRows[i].IntrastateTaxable,
			CGST:              dbRows[i].CGST,
			SGST:              dbRows[i].SGST,
			InterstateCount:   dbRows[i].InterstateCount,
			InterstateTaxable: dbRows[i].InterstateTaxable,
			IGST:              dbRows[i].IGST,
			Cess:              dbRows[i].Cess,
			TotalTax:          dbRows[i].TotalTax,
		}
	}

	return rows, nil
}

func (r *reportRepo) HSNSummary(ctx context.Context, tenantID uuid.UUID, filters *domain.ReportFilters) ([]domain.HSNSummaryRow, int, error) {
	args := []interface{}{tenantID}
	argN := 2

	// Build WHERE clause for document_line_items joined with document_summaries
	whereClause := "WHERE li.tenant_id = $1 AND li.hsn_sac_code != ''"

	if filters.From != nil {
		whereClause += fmt.Sprintf(" AND ds.invoice_date >= $%d", argN)
		args = append(args, *filters.From)
		argN++
	}
	if filters.To != nil {
		whereClause += fmt.Sprintf(" AND ds.invoice_date <= $%d", argN)
		args = append(args, *filters.To)
		argN++
	}
	if filters.CollectionID != nil {
		whereClause += fmt.Sprintf(" AND ds.collection_id = $%d", argN)
		args = append(args, *filters.CollectionID)
		argN++
	}
	if filters.SellerGSTIN != "" {
		whereClause += fmt.Sprintf(" AND ds.seller_gstin = $%d", argN)
		args = append(args, filters.SellerGSTIN)
		argN++
	}
	if filters.BuyerGSTIN != "" {
		whereClause += fmt.Sprintf(" AND ds.buyer_gstin = $%d", argN)
		args = append(args, filters.BuyerGSTIN)
		argN++
	}

	// Role scoping for viewer/free
	if filters.UserRole != domain.RoleAdmin && filters.UserRole != domain.RoleManager && filters.UserRole != domain.RoleMember {
		whereClause += fmt.Sprintf(" AND ds.collection_id IN (SELECT collection_id FROM collection_permissions WHERE user_id = $%d)", argN)
		args = append(args, filters.UserID)
		argN++ //nolint:ineffassign // argN kept incremented for consistency
	}

	dataQuery := fmt.Sprintf(`SELECT
		li.hsn_sac_code AS hsn_code,
		MAX(li.description) AS description,
		COUNT(DISTINCT li.document_id) AS invoice_count,
		COUNT(*) AS line_item_count,
		COALESCE(SUM(li.quantity), 0) AS total_quantity,
		COALESCE(SUM(li.taxable_amount), 0) AS taxable_amount,
		COALESCE(SUM(li.cgst_amount), 0) AS cgst,
		COALESCE(SUM(li.sgst_amount), 0) AS sgst,
		COALESCE(SUM(li.igst_amount), 0) AS igst,
		COALESCE(SUM(li.cgst_amount + li.sgst_amount + li.igst_amount), 0) AS total_tax
	FROM document_line_items li
	JOIN document_summaries ds ON ds.document_id = li.document_id
	%s
	GROUP BY li.hsn_sac_code
	ORDER BY taxable_amount DESC
	OFFSET %d LIMIT %d`, whereClause, filters.Offset, filters.Limit)

	var rows []domain.HSNSummaryRow
	if err := sqlx.SelectContext(ctx, r.db, &rows, dataQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("reportRepo.HSNSummary data: %w", err)
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(DISTINCT li.hsn_sac_code)
	FROM document_line_items li
	JOIN document_summaries ds ON ds.document_id = li.document_id
	%s`, whereClause)

	var total int
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("reportRepo.HSNSummary count: %w", err)
	}

	return rows, total, nil
}

func (r *reportRepo) CollectionsOverview(ctx context.Context, tenantID uuid.UUID, filters *domain.ReportFilters) ([]domain.CollectionOverviewRow, error) {
	whereClause, args := buildWhereClause(tenantID, filters)

	query := fmt.Sprintf(`SELECT
		ds.collection_id,
		c.name AS collection_name,
		COUNT(*) AS document_count,
		COALESCE(SUM(ds.total_amount), 0) AS total_amount,
		ROUND(COUNT(CASE WHEN ds.validation_status = 'valid' THEN 1 END)::numeric / NULLIF(COUNT(*), 0) * 100, 1) AS validation_valid_pct,
		ROUND(COUNT(CASE WHEN ds.validation_status = 'warning' THEN 1 END)::numeric / NULLIF(COUNT(*), 0) * 100, 1) AS validation_warning_pct,
		ROUND(COUNT(CASE WHEN ds.validation_status = 'invalid' THEN 1 END)::numeric / NULLIF(COUNT(*), 0) * 100, 1) AS validation_invalid_pct,
		ROUND(COUNT(CASE WHEN ds.review_status = 'approved' THEN 1 END)::numeric / NULLIF(COUNT(*), 0) * 100, 1) AS review_approved_pct,
		ROUND(COUNT(CASE WHEN ds.review_status = 'pending' THEN 1 END)::numeric / NULLIF(COUNT(*), 0) * 100, 1) AS review_pending_pct
	FROM document_summaries ds
	JOIN collections c ON c.id = ds.collection_id
	%s
	GROUP BY ds.collection_id, c.name
	ORDER BY total_amount DESC`, whereClause)

	var rows []domain.CollectionOverviewRow
	if err := sqlx.SelectContext(ctx, r.db, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("reportRepo.CollectionsOverview: %w", err)
	}

	return rows, nil
}
