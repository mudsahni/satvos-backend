package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/port"
)

// MockDocumentTagRepo is a mock implementation of port.DocumentTagRepository.
type MockDocumentTagRepo struct {
	mock.Mock
}

func (m *MockDocumentTagRepo) CreateBatch(ctx context.Context, tags []domain.DocumentTag) error {
	args := m.Called(ctx, tags)
	return args.Error(0)
}

func (m *MockDocumentTagRepo) ListByDocument(ctx context.Context, documentID uuid.UUID) ([]domain.DocumentTag, error) {
	args := m.Called(ctx, documentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.DocumentTag), args.Error(1)
}

func (m *MockDocumentTagRepo) SearchByTag(ctx context.Context, tenantID uuid.UUID, key, value string, offset, limit int) ([]domain.Document, int, error) {
	args := m.Called(ctx, tenantID, key, value, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Document), args.Int(1), args.Error(2)
}

func (m *MockDocumentTagRepo) FacetsByKeys(ctx context.Context, tenantID uuid.UUID, keys []string, collectionIDs []uuid.UUID) (map[string][]port.TagFacet, error) {
	args := m.Called(ctx, tenantID, keys, collectionIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string][]port.TagFacet), args.Error(1)
}

func (m *MockDocumentTagRepo) DeleteByID(ctx context.Context, documentID, tagID uuid.UUID) error {
	args := m.Called(ctx, documentID, tagID)
	return args.Error(0)
}

func (m *MockDocumentTagRepo) DeleteByDocument(ctx context.Context, documentID uuid.UUID) error {
	args := m.Called(ctx, documentID)
	return args.Error(0)
}

func (m *MockDocumentTagRepo) DeleteByDocumentAndSource(ctx context.Context, documentID uuid.UUID, source string) error {
	args := m.Called(ctx, documentID, source)
	return args.Error(0)
}
