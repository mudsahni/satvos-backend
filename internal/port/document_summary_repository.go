package port

import (
	"context"

	"github.com/google/uuid"

	"satvos/internal/domain"
)

// DocumentSummaryRepository manages the materialized document_summaries table.
type DocumentSummaryRepository interface {
	Upsert(ctx context.Context, summary *domain.DocumentSummary) error
	UpdateStatuses(ctx context.Context, documentID uuid.UUID, statuses domain.SummaryStatusUpdate) error
	ReplaceLineItems(ctx context.Context, documentID, tenantID uuid.UUID, items []domain.DocumentLineItem) error
}
