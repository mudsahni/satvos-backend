package port

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"satvos/internal/domain"
)

// SyncReviewFilters carries optional filters for the sync review queue listing.
type SyncReviewFilters struct {
	Status       domain.SyncReviewStatus
	CollectionID *uuid.UUID
}

// DocumentListFilters carries optional server-side filters for document listing.
type DocumentListFilters struct {
	AssignedTo           *uuid.UUID
	SellerName           *string
	BuyerName            *string
	ParsingStatus        *domain.ParsingStatus
	ReviewStatus         *domain.ReviewStatus
	ValidationStatus     *domain.ValidationStatus
	ReconciliationStatus *domain.ReconciliationStatus
}

// TagFacet represents a tag value with its document count.
type TagFacet struct {
	Value string `json:"value" db:"value"`
	Count int    `json:"count" db:"count"`
}

// DocumentRepository defines the contract for document persistence.
type DocumentRepository interface {
	Create(ctx context.Context, doc *domain.Document) error
	GetByID(ctx context.Context, tenantID, docID uuid.UUID) (*domain.Document, error)
	GetByFileID(ctx context.Context, tenantID, fileID uuid.UUID) (*domain.Document, error)
	ListByCollection(ctx context.Context, tenantID, collectionID uuid.UUID, filters *DocumentListFilters, offset, limit int) ([]domain.Document, int, error)
	ListByTenant(ctx context.Context, tenantID uuid.UUID, filters *DocumentListFilters, offset, limit int) ([]domain.Document, int, error)
	ListByUserCollections(ctx context.Context, tenantID, userID uuid.UUID, filters *DocumentListFilters, offset, limit int) ([]domain.Document, int, error)
	ListByServiceAccountCollections(ctx context.Context, tenantID, saID uuid.UUID, filters *DocumentListFilters, offset, limit int) ([]domain.Document, int, error)
	UpdateStructuredData(ctx context.Context, doc *domain.Document) error
	UpdateReviewStatus(ctx context.Context, doc *domain.Document) error
	UpdateAssignment(ctx context.Context, doc *domain.Document) error
	UpdateValidationResults(ctx context.Context, doc *domain.Document) error
	ListReviewQueue(ctx context.Context, tenantID, userID uuid.UUID, offset, limit int) ([]domain.Document, int, error)
	ClaimQueued(ctx context.Context, limit int) ([]domain.Document, error)
	ResetStaleProcessing(ctx context.Context, staleBefore time.Time) (int, error)
	Delete(ctx context.Context, tenantID, docID uuid.UUID) error
	ListSyncReviewQueue(ctx context.Context, tenantID uuid.UUID, filters *SyncReviewFilters, offset, limit int) ([]domain.Document, int, error)
	BatchUpdateSyncReviewStatus(ctx context.Context, tenantID uuid.UUID, docIDs []uuid.UUID, status domain.SyncReviewStatus, approvedBy *uuid.UUID) error
	UpdateVoucherOverrides(ctx context.Context, tenantID, docID uuid.UUID, overrides json.RawMessage) error
}

// DocumentTagRepository defines the contract for document tag persistence.
type DocumentTagRepository interface {
	CreateBatch(ctx context.Context, tags []domain.DocumentTag) error
	ListByDocument(ctx context.Context, documentID uuid.UUID) ([]domain.DocumentTag, error)
	SearchByTag(ctx context.Context, tenantID uuid.UUID, key, value string, offset, limit int) ([]domain.Document, int, error)
	FacetsByKeys(ctx context.Context, tenantID uuid.UUID, keys []string, collectionIDs []uuid.UUID) (map[string][]TagFacet, error)
	DeleteByID(ctx context.Context, documentID, tagID uuid.UUID) error
	DeleteByDocument(ctx context.Context, documentID uuid.UUID) error
	DeleteByDocumentAndSource(ctx context.Context, documentID uuid.UUID, source string) error
}

// DocumentValidationRuleRepository defines the contract for validation rule persistence.
type DocumentValidationRuleRepository interface {
	Create(ctx context.Context, rule *domain.DocumentValidationRule) error
	GetByID(ctx context.Context, tenantID, ruleID uuid.UUID) (*domain.DocumentValidationRule, error)
	ListByDocumentType(ctx context.Context, tenantID uuid.UUID, docType string, collectionID *uuid.UUID) ([]domain.DocumentValidationRule, error)
	ListBuiltinKeys(ctx context.Context, tenantID uuid.UUID, docType string) ([]string, error)
	Update(ctx context.Context, rule *domain.DocumentValidationRule) error
	Delete(ctx context.Context, tenantID, ruleID uuid.UUID) error
}

