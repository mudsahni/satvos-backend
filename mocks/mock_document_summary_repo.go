package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
)

type MockDocumentSummaryRepo struct {
	mock.Mock
}

func (m *MockDocumentSummaryRepo) Upsert(ctx context.Context, summary *domain.DocumentSummary) error {
	args := m.Called(ctx, summary)
	return args.Error(0)
}

func (m *MockDocumentSummaryRepo) UpdateStatuses(ctx context.Context, documentID uuid.UUID, statuses domain.SummaryStatusUpdate) error {
	args := m.Called(ctx, documentID, statuses)
	return args.Error(0)
}

func (m *MockDocumentSummaryRepo) ReplaceLineItems(ctx context.Context, documentID, tenantID uuid.UUID, items []domain.DocumentLineItem) error {
	args := m.Called(ctx, documentID, tenantID, items)
	return args.Error(0)
}
