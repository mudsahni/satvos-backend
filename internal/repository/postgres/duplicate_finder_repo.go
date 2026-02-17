package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"satvos/internal/port"
)

type duplicateFinderRepo struct {
	db *sqlx.DB
}

// NewDuplicateFinderRepo creates a new PostgreSQL-backed DuplicateInvoiceFinder.
func NewDuplicateFinderRepo(db *sqlx.DB) port.DuplicateInvoiceFinder {
	return &duplicateFinderRepo{db: db}
}

func (r *duplicateFinderRepo) FindDuplicates(
	ctx context.Context,
	tenantID, excludeDocID uuid.UUID,
	sellerGSTIN, invoiceNumber, financialYear, irn string,
) ([]port.DuplicateMatch, error) {
	// Base WHERE clause uses $1=tenantID, $2=excludeDocID, $3=sellerGSTIN, $4=invoiceNumber.
	// Additional positional args are appended dynamically.
	args := []interface{}{tenantID, excludeDocID, sellerGSTIN, invoiceNumber}
	nextArg := 5

	baseWhere := `d.tenant_id = $1
		  AND d.id != $2
		  AND d.parsing_status = 'completed'
		  AND d.structured_data @> jsonb_build_object(
		      'seller', jsonb_build_object('gstin', $3),
		      'invoice', jsonb_build_object('invoice_number', $4)
		  )`

	var ctes []string
	var unions []string
	var higherTierIDs []string // CTE names whose IDs should be excluded from lower tiers

	// Tier 1: exact_irn — only when irn is non-empty
	if irn != "" {
		irnArg := fmt.Sprintf("$%d", nextArg)
		args = append(args, irn)
		nextArg++

		ctes = append(ctes, fmt.Sprintf(`tier1 AS (
		SELECT d.id, d.name, 'exact_irn'::text AS match_type, d.created_at
		FROM documents d
		WHERE %s
		  AND LOWER(d.structured_data->'invoice'->>'irn') = LOWER(%s)
	)`, baseWhere, irnArg))

		unions = append(unions, `SELECT id, name, match_type, created_at FROM tier1`)
		higherTierIDs = append(higherTierIDs, "tier1")
	}

	// Tier 2: strong — only when financialYear is non-empty
	if financialYear != "" {
		fyArg := fmt.Sprintf("$%d", nextArg)
		args = append(args, financialYear)

		excludeClause := ""
		if len(higherTierIDs) > 0 {
			parts := make([]string, 0, len(higherTierIDs))
			for _, cte := range higherTierIDs {
				parts = append(parts, fmt.Sprintf("SELECT id FROM %s", cte))
			}
			excludeClause = fmt.Sprintf(" AND d.id NOT IN (%s)", strings.Join(parts, " UNION ALL "))
		}

		ctes = append(ctes, fmt.Sprintf(`tier2 AS (
		SELECT d.id, d.name, 'strong'::text AS match_type, d.created_at
		FROM documents d
		JOIN document_summaries ds ON ds.document_id = d.id
		WHERE %s
		  AND CASE
		    WHEN EXTRACT(MONTH FROM ds.invoice_date) >= 4
		    THEN CONCAT(EXTRACT(YEAR FROM ds.invoice_date)::int, '-',
		         LPAD(((EXTRACT(YEAR FROM ds.invoice_date)::int + 1) %% 100)::text, 2, '0'))
		    ELSE CONCAT((EXTRACT(YEAR FROM ds.invoice_date)::int - 1), '-',
		         LPAD((EXTRACT(YEAR FROM ds.invoice_date)::int %% 100)::text, 2, '0'))
		  END = %s%s
	)`, baseWhere, fyArg, excludeClause))

		unions = append(unions, `SELECT id, name, match_type, created_at FROM tier2`)
		higherTierIDs = append(higherTierIDs, "tier2")
	}

	// Tier 3: weak — always included, excludes IDs from higher tiers
	excludeClause := ""
	if len(higherTierIDs) > 0 {
		parts := make([]string, 0, len(higherTierIDs))
		for _, cte := range higherTierIDs {
			parts = append(parts, fmt.Sprintf("SELECT id FROM %s", cte))
		}
		excludeClause = fmt.Sprintf(" AND d.id NOT IN (%s)", strings.Join(parts, " UNION ALL "))
	}

	ctes = append(ctes, fmt.Sprintf(`tier3 AS (
		SELECT d.id, d.name, 'weak'::text AS match_type, d.created_at
		FROM documents d
		WHERE %s%s
	)`, baseWhere, excludeClause))

	unions = append(unions, `SELECT id, name, match_type, created_at FROM tier3`)

	// Build full query
	query := fmt.Sprintf(`WITH %s
	SELECT id, name, match_type, created_at
	FROM (%s) AS all_matches
	ORDER BY CASE match_type
		WHEN 'exact_irn' THEN 1
		WHEN 'strong' THEN 2
		WHEN 'weak' THEN 3
	END, created_at DESC
	LIMIT 10`,
		strings.Join(ctes, ",\n"),
		strings.Join(unions, " UNION ALL "),
	)

	var matches []port.DuplicateMatch
	err := r.db.SelectContext(ctx, &matches, query, args...)
	if err != nil {
		return nil, fmt.Errorf("duplicate_finder_repo.FindDuplicates: %w", err)
	}
	return matches, nil
}
