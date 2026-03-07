package service

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/port"
	"satvos/internal/validator"
)

// CreateDocumentInput is the DTO for creating a document and triggering parsing.
type CreateDocumentInput struct {
	TenantID     uuid.UUID
	CollectionID uuid.UUID
	FileID       uuid.UUID
	DocumentType string
	ParseMode    domain.ParseMode
	Name         string
	Tags         map[string]string
	CreatedBy    uuid.UUID
	Role         domain.UserRole
}

// EditStructuredDataInput is the DTO for manually editing a document's structured data.
type EditStructuredDataInput struct {
	TenantID       uuid.UUID
	DocumentID     uuid.UUID
	UserID         uuid.UUID
	Role           domain.UserRole
	StructuredData json.RawMessage
}

// AssignDocumentInput is the DTO for assigning a document to a reviewer.
type AssignDocumentInput struct {
	TenantID   uuid.UUID
	DocumentID uuid.UUID
	CallerID   uuid.UUID
	CallerRole domain.UserRole
	AssigneeID *uuid.UUID // nil = unassign
}

// UpdateReviewInput is the DTO for updating a document's review status.
type UpdateReviewInput struct {
	TenantID   uuid.UUID
	DocumentID uuid.UUID
	ReviewerID uuid.UUID
	Role       domain.UserRole
	Status     domain.ReviewStatus
	Notes      string
}

// DocumentService defines the document management contract.
type DocumentService interface {
	CreateAndParse(ctx context.Context, input *CreateDocumentInput) (*domain.Document, error)
	GetByID(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) (*domain.Document, error)
	GetByFileID(ctx context.Context, tenantID, fileID, userID uuid.UUID, role domain.UserRole) (*domain.Document, error)
	ListByCollection(ctx context.Context, tenantID, collectionID, userID uuid.UUID, role domain.UserRole, filters *port.DocumentListFilters, offset, limit int) ([]domain.Document, int, error)
	ListByTenant(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, filters *port.DocumentListFilters, offset, limit int) ([]domain.Document, int, error)
	GetTagFacets(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, keys []string) (map[string][]port.TagFacet, error)
	AssignDocument(ctx context.Context, input *AssignDocumentInput) (*domain.Document, error)
	ListReviewQueue(ctx context.Context, tenantID, userID uuid.UUID, offset, limit int) ([]domain.Document, int, error)
	UpdateReview(ctx context.Context, input *UpdateReviewInput) (*domain.Document, error)
	EditStructuredData(ctx context.Context, input *EditStructuredDataInput) (*domain.Document, error)
	RetryParse(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) (*domain.Document, error)
	ValidateDocument(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) error
	GetValidation(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) (*validator.ValidationResponse, error)
	Delete(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) error
	ListTags(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) ([]domain.DocumentTag, error)
	AddTags(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole, tags map[string]string) ([]domain.DocumentTag, error)
	DeleteTag(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole, tagID uuid.UUID) error
	SearchByTag(ctx context.Context, tenantID uuid.UUID, key, value string, offset, limit int) ([]domain.Document, int, error)
	ParseDocument(ctx context.Context, doc *domain.Document, maxAttempts int)
	ListSyncReviewQueue(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, filters *port.SyncReviewFilters, offset, limit int) ([]domain.Document, int, error)
	ApproveSyncBatch(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, docIDs []uuid.UUID) error
	RejectSyncBatch(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, docIDs []uuid.UUID, reason string) error
	ApproveCollectionSync(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, collectionID uuid.UUID) error
	UpdateVoucherOverrides(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, docID uuid.UUID, overrides *domain.VoucherOverrides) error
}

// DocumentServiceDeps groups dependencies for NewDocumentService.
type DocumentServiceDeps struct {
	DocRepo     port.DocumentRepository
	FileRepo    port.FileMetaRepository
	UserRepo    port.UserRepository
	PermRepo    port.CollectionPermissionRepository
	TagRepo     port.DocumentTagRepository
	AuditRepo   port.DocumentAuditRepository
	SummaryRepo port.DocumentSummaryRepository
	SAPermRepo  port.ServiceAccountPermissionRepository // optional — enables service account access
	Parser      port.DocumentParser
	MergeParser port.DocumentParser // optional — enables dual-parse mode
	Storage     port.ObjectStorage
	Validator   *validator.Engine
}

type documentService struct {
	docRepo     port.DocumentRepository
	fileRepo    port.FileMetaRepository
	userRepo    port.UserRepository
	permRepo    port.CollectionPermissionRepository
	tagRepo     port.DocumentTagRepository
	auditRepo   port.DocumentAuditRepository
	summaryRepo port.DocumentSummaryRepository
	saPermRepo  port.ServiceAccountPermissionRepository
	parser      port.DocumentParser
	mergeParser port.DocumentParser // optional merge parser for dual mode
	storage     port.ObjectStorage
	validator   *validator.Engine
}

// NewDocumentService creates a new DocumentService implementation.
func NewDocumentService(deps *DocumentServiceDeps) DocumentService {
	return &documentService{
		docRepo:     deps.DocRepo,
		fileRepo:    deps.FileRepo,
		userRepo:    deps.UserRepo,
		permRepo:    deps.PermRepo,
		tagRepo:     deps.TagRepo,
		auditRepo:   deps.AuditRepo,
		summaryRepo: deps.SummaryRepo,
		saPermRepo:  deps.SAPermRepo,
		parser:      deps.Parser,
		mergeParser: deps.MergeParser,
		storage:     deps.Storage,
		validator:   deps.Validator,
	}
}

// requireCollectionPerm delegates to the shared RequireCollectionPerm helper.
// For service accounts, it checks the service_account_permissions table instead.
func (s *documentService) requireCollectionPerm(ctx context.Context, collectionID, userID uuid.UUID, role domain.UserRole, minLevel domain.CollectionPermission) error {
	if role == domain.RoleService {
		if s.saPermRepo == nil {
			return domain.ErrCollectionPermDenied
		}
		return RequireServiceAccountCollectionPerm(ctx, s.saPermRepo, collectionID, userID, minLevel)
	}
	return RequireCollectionPerm(ctx, s.permRepo, collectionID, userID, role, minLevel)
}

func (s *documentService) GetByID(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) (*domain.Document, error) {
	doc, err := s.docRepo.GetByID(ctx, tenantID, docID)
	if err != nil {
		return nil, err
	}
	if err := s.requireCollectionPerm(ctx, doc.CollectionID, userID, role, domain.CollectionPermViewer); err != nil {
		return nil, err
	}
	return doc, nil
}

func (s *documentService) GetByFileID(ctx context.Context, tenantID, fileID, userID uuid.UUID, role domain.UserRole) (*domain.Document, error) {
	doc, err := s.docRepo.GetByFileID(ctx, tenantID, fileID)
	if err != nil {
		return nil, err
	}
	if err := s.requireCollectionPerm(ctx, doc.CollectionID, userID, role, domain.CollectionPermViewer); err != nil {
		return nil, err
	}
	return doc, nil
}

func (s *documentService) ListByCollection(ctx context.Context, tenantID, collectionID, userID uuid.UUID, role domain.UserRole, filters *port.DocumentListFilters, offset, limit int) ([]domain.Document, int, error) {
	if err := s.requireCollectionPerm(ctx, collectionID, userID, role, domain.CollectionPermViewer); err != nil {
		return nil, 0, err
	}
	return s.docRepo.ListByCollection(ctx, tenantID, collectionID, filters, offset, limit)
}

func (s *documentService) ListByTenant(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, filters *port.DocumentListFilters, offset, limit int) ([]domain.Document, int, error) {
	// Admin, manager, and member see all documents
	if role == domain.RoleAdmin || role == domain.RoleManager || role == domain.RoleMember {
		return s.docRepo.ListByTenant(ctx, tenantID, filters, offset, limit)
	}
	// Service accounts see only documents in collections they have explicit permission for
	if role == domain.RoleService {
		return s.docRepo.ListByServiceAccountCollections(ctx, tenantID, userID, filters, offset, limit)
	}
	// Viewer/free sees only documents in collections they have access to
	return s.docRepo.ListByUserCollections(ctx, tenantID, userID, filters, offset, limit)
}

// accessibleCollectionIDs returns the collection IDs a user can access, or nil for unrestricted access.
func (s *documentService) accessibleCollectionIDs(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole) ([]uuid.UUID, error) {
	if role == domain.RoleAdmin || role == domain.RoleManager || role == domain.RoleMember {
		return nil, nil // nil = all collections
	}
	if role == domain.RoleService {
		if s.saPermRepo == nil {
			return []uuid.UUID{}, nil
		}
		return s.saPermRepo.ListCollectionIDs(ctx, tenantID, userID)
	}
	return s.permRepo.ListCollectionIDs(ctx, tenantID, userID)
}

func (s *documentService) GetTagFacets(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, keys []string) (map[string][]port.TagFacet, error) {
	collectionIDs, err := s.accessibleCollectionIDs(ctx, tenantID, userID, role)
	if err != nil {
		return nil, err
	}
	return s.tagRepo.FacetsByKeys(ctx, tenantID, keys, collectionIDs)
}

func (s *documentService) ListReviewQueue(ctx context.Context, tenantID, userID uuid.UUID, offset, limit int) ([]domain.Document, int, error) {
	return s.docRepo.ListReviewQueue(ctx, tenantID, userID, offset, limit)
}

func (s *documentService) Delete(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) error {
	s.audit(ctx, tenantID, docID, &userID, domain.AuditDocumentDeleted, nil)
	return s.docRepo.Delete(ctx, tenantID, docID)
}
