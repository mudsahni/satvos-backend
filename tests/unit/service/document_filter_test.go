package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"satvos/internal/domain"
	"satvos/internal/port"
	"satvos/internal/service"
	"satvos/mocks"
)

// setupDocumentServiceWithSAPerm creates a service with a service account permission repo.
func setupDocumentServiceWithSAPerm() (
	service.DocumentService,
	*mocks.MockDocumentRepo,
	*mocks.MockCollectionPermissionRepo,
	*mocks.MockDocumentTagRepo,
	*mocks.MockServiceAccountPermissionRepo,
) {
	docRepo := new(mocks.MockDocumentRepo)
	fileRepo := new(mocks.MockFileMetaRepo)
	userRepo := new(mocks.MockUserRepo)
	permRepo := new(mocks.MockCollectionPermissionRepo)
	tagRepo := new(mocks.MockDocumentTagRepo)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	saPermRepo := new(mocks.MockServiceAccountPermissionRepo)
	p := new(mocks.MockDocumentParser)
	storage := new(mocks.MockObjectStorage)
	userRepo.On("CheckAndIncrementQuota", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	auditRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.DocumentAuditEntry")).Return(nil).Maybe()
	svc := service.NewDocumentService(&service.DocumentServiceDeps{
		DocRepo:    docRepo,
		FileRepo:   fileRepo,
		UserRepo:   userRepo,
		PermRepo:   permRepo,
		TagRepo:    tagRepo,
		AuditRepo:  auditRepo,
		SAPermRepo: saPermRepo,
		Parser:     p,
		Storage:    storage,
	})
	return svc, docRepo, permRepo, tagRepo, saPermRepo
}

// --- GetTagFacets ---

func TestDocumentService_GetTagFacets_Admin(t *testing.T) {
	svc, _, _, tagRepo, _ := setupDocumentServiceWithSAPerm()

	tenantID := uuid.New()
	userID := uuid.New()
	keys := []string{"seller_name", "buyer_name"}

	expected := map[string][]port.TagFacet{
		"seller_name": {{Value: "Acme", Count: 10}},
		"buyer_name":  {{Value: "Corp", Count: 5}},
	}

	// Admin passes nil collectionIDs (all collections)
	tagRepo.On("FacetsByKeys", mock.Anything, tenantID, keys, []uuid.UUID(nil)).
		Return(expected, nil)

	result, err := svc.GetTagFacets(context.Background(), tenantID, userID, domain.RoleAdmin, keys)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
	tagRepo.AssertExpectations(t)
}

func TestDocumentService_GetTagFacets_Manager(t *testing.T) {
	svc, _, _, tagRepo, _ := setupDocumentServiceWithSAPerm()

	tenantID := uuid.New()
	userID := uuid.New()
	keys := []string{"seller_name"}

	expected := map[string][]port.TagFacet{
		"seller_name": {{Value: "Test", Count: 3}},
	}

	tagRepo.On("FacetsByKeys", mock.Anything, tenantID, keys, []uuid.UUID(nil)).
		Return(expected, nil)

	result, err := svc.GetTagFacets(context.Background(), tenantID, userID, domain.RoleManager, keys)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestDocumentService_GetTagFacets_Member(t *testing.T) {
	svc, _, _, tagRepo, _ := setupDocumentServiceWithSAPerm()

	tenantID := uuid.New()
	userID := uuid.New()
	keys := []string{"buyer_name"}

	expected := map[string][]port.TagFacet{
		"buyer_name": {},
	}

	// Member also passes nil (all collections)
	tagRepo.On("FacetsByKeys", mock.Anything, tenantID, keys, []uuid.UUID(nil)).
		Return(expected, nil)

	result, err := svc.GetTagFacets(context.Background(), tenantID, userID, domain.RoleMember, keys)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestDocumentService_GetTagFacets_Viewer(t *testing.T) {
	svc, _, permRepo, tagRepo, _ := setupDocumentServiceWithSAPerm()

	tenantID := uuid.New()
	userID := uuid.New()
	keys := []string{"seller_name"}

	collIDs := []uuid.UUID{uuid.New(), uuid.New()}
	expected := map[string][]port.TagFacet{
		"seller_name": {{Value: "Scoped Seller", Count: 7}},
	}

	// Viewer gets scoped collection IDs
	permRepo.On("ListCollectionIDs", mock.Anything, tenantID, userID).
		Return(collIDs, nil)
	tagRepo.On("FacetsByKeys", mock.Anything, tenantID, keys, collIDs).
		Return(expected, nil)

	result, err := svc.GetTagFacets(context.Background(), tenantID, userID, domain.RoleViewer, keys)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
	permRepo.AssertExpectations(t)
	tagRepo.AssertExpectations(t)
}

func TestDocumentService_GetTagFacets_ViewerNoPermissions(t *testing.T) {
	svc, _, permRepo, tagRepo, _ := setupDocumentServiceWithSAPerm()

	tenantID := uuid.New()
	userID := uuid.New()
	keys := []string{"seller_name"}

	// Viewer with no permissions gets empty slice
	permRepo.On("ListCollectionIDs", mock.Anything, tenantID, userID).
		Return([]uuid.UUID{}, nil)

	// The empty collectionIDs should be passed through (FacetsByKeys handles it)
	tagRepo.On("FacetsByKeys", mock.Anything, tenantID, keys, []uuid.UUID{}).
		Return(map[string][]port.TagFacet{"seller_name": {}}, nil)

	result, err := svc.GetTagFacets(context.Background(), tenantID, userID, domain.RoleViewer, keys)

	require.NoError(t, err)
	assert.Empty(t, result["seller_name"])
	permRepo.AssertExpectations(t)
}

func TestDocumentService_GetTagFacets_Free(t *testing.T) {
	svc, _, permRepo, tagRepo, _ := setupDocumentServiceWithSAPerm()

	tenantID := uuid.New()
	userID := uuid.New()
	keys := []string{"seller_name"}

	collIDs := []uuid.UUID{uuid.New()}

	permRepo.On("ListCollectionIDs", mock.Anything, tenantID, userID).
		Return(collIDs, nil)
	tagRepo.On("FacetsByKeys", mock.Anything, tenantID, keys, collIDs).
		Return(map[string][]port.TagFacet{"seller_name": {{Value: "X", Count: 1}}}, nil)

	result, err := svc.GetTagFacets(context.Background(), tenantID, userID, domain.RoleFree, keys)

	require.NoError(t, err)
	assert.Len(t, result["seller_name"], 1)
}

func TestDocumentService_GetTagFacets_Service(t *testing.T) {
	svc, _, _, tagRepo, saPermRepo := setupDocumentServiceWithSAPerm()

	tenantID := uuid.New()
	saID := uuid.New()
	keys := []string{"buyer_name"}

	collIDs := []uuid.UUID{uuid.New()}
	expected := map[string][]port.TagFacet{
		"buyer_name": {{Value: "SA Buyer", Count: 3}},
	}

	saPermRepo.On("ListCollectionIDs", mock.Anything, tenantID, saID).
		Return(collIDs, nil)
	tagRepo.On("FacetsByKeys", mock.Anything, tenantID, keys, collIDs).
		Return(expected, nil)

	result, err := svc.GetTagFacets(context.Background(), tenantID, saID, domain.RoleService, keys)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
	saPermRepo.AssertExpectations(t)
}

func TestDocumentService_GetTagFacets_ServiceNoPermissions(t *testing.T) {
	svc, _, _, tagRepo, saPermRepo := setupDocumentServiceWithSAPerm()

	tenantID := uuid.New()
	saID := uuid.New()
	keys := []string{"seller_name"}

	// SA with no collection permissions
	saPermRepo.On("ListCollectionIDs", mock.Anything, tenantID, saID).
		Return([]uuid.UUID{}, nil)
	tagRepo.On("FacetsByKeys", mock.Anything, tenantID, keys, []uuid.UUID{}).
		Return(map[string][]port.TagFacet{"seller_name": {}}, nil)

	result, err := svc.GetTagFacets(context.Background(), tenantID, saID, domain.RoleService, keys)

	require.NoError(t, err)
	assert.Empty(t, result["seller_name"])
}

func TestDocumentService_GetTagFacets_PermRepoError(t *testing.T) {
	svc, _, permRepo, _, _ := setupDocumentServiceWithSAPerm()

	tenantID := uuid.New()
	userID := uuid.New()
	keys := []string{"seller_name"}

	permRepo.On("ListCollectionIDs", mock.Anything, tenantID, userID).
		Return(nil, errors.New("db error"))

	_, err := svc.GetTagFacets(context.Background(), tenantID, userID, domain.RoleViewer, keys)

	assert.Error(t, err)
}

func TestDocumentService_GetTagFacets_TagRepoError(t *testing.T) {
	svc, _, _, tagRepo, _ := setupDocumentServiceWithSAPerm()

	tenantID := uuid.New()
	userID := uuid.New()
	keys := []string{"seller_name"}

	tagRepo.On("FacetsByKeys", mock.Anything, tenantID, keys, []uuid.UUID(nil)).
		Return(nil, errors.New("db error"))

	_, err := svc.GetTagFacets(context.Background(), tenantID, userID, domain.RoleAdmin, keys)

	assert.Error(t, err)
}

// --- ListByTenant role branching ---

func TestDocumentService_ListByTenant_ViewerRole(t *testing.T) {
	svc, docRepo, _, _, _ := setupDocumentServiceWithSAPerm()

	tenantID := uuid.New()
	userID := uuid.New()
	filters := &port.DocumentListFilters{}

	expected := []domain.Document{{ID: uuid.New()}}
	docRepo.On("ListByUserCollections", mock.Anything, tenantID, userID, filters, 0, 20).
		Return(expected, 1, nil)

	docs, total, err := svc.ListByTenant(context.Background(), tenantID, userID, domain.RoleViewer, filters, 0, 20)

	require.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, 1, total)
	docRepo.AssertExpectations(t)
}

func TestDocumentService_ListByTenant_FreeRole(t *testing.T) {
	svc, docRepo, _, _, _ := setupDocumentServiceWithSAPerm()

	tenantID := uuid.New()
	userID := uuid.New()
	filters := &port.DocumentListFilters{}

	docRepo.On("ListByUserCollections", mock.Anything, tenantID, userID, filters, 0, 20).
		Return([]domain.Document{}, 0, nil)

	docs, total, err := svc.ListByTenant(context.Background(), tenantID, userID, domain.RoleFree, filters, 0, 20)

	require.NoError(t, err)
	assert.Empty(t, docs)
	assert.Equal(t, 0, total)
}

func TestDocumentService_ListByTenant_ServiceRole(t *testing.T) {
	svc, docRepo, _, _, _ := setupDocumentServiceWithSAPerm()

	tenantID := uuid.New()
	saID := uuid.New()
	filters := &port.DocumentListFilters{}

	expected := []domain.Document{{ID: uuid.New()}}
	docRepo.On("ListByServiceAccountCollections", mock.Anything, tenantID, saID, filters, 0, 20).
		Return(expected, 1, nil)

	docs, total, err := svc.ListByTenant(context.Background(), tenantID, saID, domain.RoleService, filters, 0, 20)

	require.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, 1, total)
}

func TestDocumentService_ListByTenant_WithFilters(t *testing.T) {
	svc, docRepo, _, _, _ := setupDocumentServiceWithSAPerm()

	tenantID := uuid.New()
	userID := uuid.New()
	completed := domain.ParsingStatusCompleted
	approved := domain.ReviewStatusApproved
	filters := &port.DocumentListFilters{
		ParsingStatus: &completed,
		ReviewStatus:  &approved,
	}

	docRepo.On("ListByTenant", mock.Anything, tenantID, filters, 0, 20).
		Return([]domain.Document{}, 0, nil)

	_, _, err := svc.ListByTenant(context.Background(), tenantID, userID, domain.RoleAdmin, filters, 0, 20)

	require.NoError(t, err)
	docRepo.AssertExpectations(t)
}
