package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/service"
	"satvos/mocks"
)

func setupCollectionService() (
	service.CollectionService,
	*mocks.MockCollectionRepo,
	*mocks.MockCollectionPermissionRepo,
	*mocks.MockCollectionFileRepo,
	*mocks.MockUserRepo,
) {
	collRepo := new(mocks.MockCollectionRepo)
	permRepo := new(mocks.MockCollectionPermissionRepo)
	fileRepo := new(mocks.MockCollectionFileRepo)
	fileSvc := new(mocks.MockFileService)
	userRepo := new(mocks.MockUserRepo)
	userRepo.On("GetByID", mock.Anything, mock.Anything, mock.Anything).Return(&domain.User{}, nil).Maybe()
	svc := service.NewCollectionService(collRepo, permRepo, fileRepo, fileSvc, userRepo, nil)
	return svc, collRepo, permRepo, fileRepo, userRepo
}

func ownerPerm(collectionID, userID uuid.UUID) *domain.CollectionPermissionEntry {
	return &domain.CollectionPermissionEntry{
		ID:           uuid.New(),
		CollectionID: collectionID,
		UserID:       userID,
		Permission:   domain.CollectionPermOwner,
	}
}

func editorPerm(collectionID, userID uuid.UUID) *domain.CollectionPermissionEntry {
	return &domain.CollectionPermissionEntry{
		ID:           uuid.New(),
		CollectionID: collectionID,
		UserID:       userID,
		Permission:   domain.CollectionPermEditor,
	}
}

func viewerPerm(collectionID, userID uuid.UUID) *domain.CollectionPermissionEntry {
	return &domain.CollectionPermissionEntry{
		ID:           uuid.New(),
		CollectionID: collectionID,
		UserID:       userID,
		Permission:   domain.CollectionPermViewer,
	}
}

// --- Create ---

func TestCollectionService_Create_Success(t *testing.T) {
	svc, collRepo, permRepo, _, _ := setupCollectionService()

	tenantID := uuid.New()
	userID := uuid.New()

	collRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Collection")).Return(nil)
	permRepo.On("Upsert", mock.Anything, mock.AnythingOfType("*domain.CollectionPermissionEntry")).Return(nil)

	result, err := svc.Create(context.Background(), &service.CreateCollectionInput{
		TenantID:    tenantID,
		CreatedBy:   userID,
		Role:        domain.RoleMember,
		Name:        "Test Collection",
		Description: "A test collection",
	})

	assert.NoError(t, err)
	assert.Equal(t, "Test Collection", result.Name)
	assert.Equal(t, "A test collection", result.Description)
	assert.Equal(t, tenantID, result.TenantID)
	assert.Equal(t, userID, result.CreatedBy)

	collRepo.AssertExpectations(t)
	permRepo.AssertExpectations(t)
}

func TestCollectionService_Create_RepoError(t *testing.T) {
	svc, collRepo, _, _, _ := setupCollectionService()

	collRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Collection")).
		Return(errors.New("db error"))

	result, err := svc.Create(context.Background(), &service.CreateCollectionInput{
		TenantID:  uuid.New(),
		CreatedBy: uuid.New(),
		Role:      domain.RoleMember,
		Name:      "Test",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "creating collection")
}

func TestCollectionService_Create_PermissionUpsertError(t *testing.T) {
	svc, collRepo, permRepo, _, _ := setupCollectionService()

	collRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Collection")).Return(nil)
	permRepo.On("Upsert", mock.Anything, mock.AnythingOfType("*domain.CollectionPermissionEntry")).
		Return(errors.New("db error"))

	result, err := svc.Create(context.Background(), &service.CreateCollectionInput{
		TenantID:  uuid.New(),
		CreatedBy: uuid.New(),
		Role:      domain.RoleMember,
		Name:      "Test",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "assigning owner permission")
}

func TestCollectionService_Create_OwnerEmailGrantsPermission(t *testing.T) {
	svc, collRepo, permRepo, _, userRepo := setupCollectionService()

	tenantID := uuid.New()
	creatorID := uuid.New()
	ownerUserID := uuid.New()
	ownerEmail := "owner@example.com"

	collRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Collection")).Return(nil)
	permRepo.On("Upsert", mock.Anything, mock.AnythingOfType("*domain.CollectionPermissionEntry")).Return(nil)
	userRepo.On("GetByEmail", mock.Anything, tenantID, ownerEmail).
		Return(&domain.User{ID: ownerUserID, TenantID: tenantID, Email: ownerEmail}, nil)

	result, err := svc.Create(context.Background(), &service.CreateCollectionInput{
		TenantID:    tenantID,
		CreatedBy:   creatorID,
		Role:        domain.RoleMember,
		Name:        "Test Collection",
		Description: "A test collection",
		OwnerEmail:  ownerEmail,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Verify Upsert was called twice: once for creator, once for owner_email user
	permRepo.AssertNumberOfCalls(t, "Upsert", 2)

	// Verify the second upsert was for the email owner
	secondCall := permRepo.Calls[1]
	perm := secondCall.Arguments.Get(1).(*domain.CollectionPermissionEntry)
	assert.Equal(t, ownerUserID, perm.UserID)
	assert.Equal(t, domain.CollectionPermOwner, perm.Permission)
	assert.Equal(t, creatorID, perm.GrantedBy)
}

func TestCollectionService_Create_OwnerEmailNotFound(t *testing.T) {
	svc, collRepo, permRepo, _, userRepo := setupCollectionService()

	tenantID := uuid.New()
	creatorID := uuid.New()

	collRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Collection")).Return(nil)
	permRepo.On("Upsert", mock.Anything, mock.AnythingOfType("*domain.CollectionPermissionEntry")).Return(nil)
	userRepo.On("GetByEmail", mock.Anything, tenantID, "unknown@example.com").
		Return(nil, domain.ErrNotFound)

	result, err := svc.Create(context.Background(), &service.CreateCollectionInput{
		TenantID:    tenantID,
		CreatedBy:   creatorID,
		Role:        domain.RoleMember,
		Name:        "Test Collection",
		OwnerEmail:  "unknown@example.com",
	})

	// Should succeed — owner_email lookup failure is non-blocking
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Only one upsert (for the creator), not two
	permRepo.AssertNumberOfCalls(t, "Upsert", 1)
}

func TestCollectionService_Create_OwnerEmailSameAsCreator(t *testing.T) {
	svc, collRepo, permRepo, _, userRepo := setupCollectionService()

	tenantID := uuid.New()
	creatorID := uuid.New()
	creatorEmail := "creator@example.com"

	collRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Collection")).Return(nil)
	permRepo.On("Upsert", mock.Anything, mock.AnythingOfType("*domain.CollectionPermissionEntry")).Return(nil)
	userRepo.On("GetByEmail", mock.Anything, tenantID, creatorEmail).
		Return(&domain.User{ID: creatorID, TenantID: tenantID, Email: creatorEmail}, nil)

	result, err := svc.Create(context.Background(), &service.CreateCollectionInput{
		TenantID:    tenantID,
		CreatedBy:   creatorID,
		Role:        domain.RoleMember,
		Name:        "Test Collection",
		OwnerEmail:  creatorEmail,
	})

	// Should succeed — no duplicate upsert when owner_email resolves to creator
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Only one upsert (for the creator); skip because same user
	permRepo.AssertNumberOfCalls(t, "Upsert", 1)
}

func TestCollectionService_Create_OwnerEmailEmpty(t *testing.T) {
	svc, collRepo, permRepo, _, _ := setupCollectionService()

	tenantID := uuid.New()
	creatorID := uuid.New()

	collRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Collection")).Return(nil)
	permRepo.On("Upsert", mock.Anything, mock.AnythingOfType("*domain.CollectionPermissionEntry")).Return(nil)

	result, err := svc.Create(context.Background(), &service.CreateCollectionInput{
		TenantID:    tenantID,
		CreatedBy:   creatorID,
		Role:        domain.RoleMember,
		Name:        "Test Collection",
		OwnerEmail:  "",
	})

	// Empty owner_email is a no-op — existing behavior preserved
	assert.NoError(t, err)
	assert.NotNil(t, result)
	permRepo.AssertNumberOfCalls(t, "Upsert", 1)
}

func TestCollectionService_Create_OwnerEmailPermUpsertFails(t *testing.T) {
	svc, collRepo, permRepo, _, userRepo := setupCollectionService()

	tenantID := uuid.New()
	creatorID := uuid.New()
	ownerUserID := uuid.New()
	ownerEmail := "owner@example.com"

	collRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Collection")).Return(nil)
	// First Upsert (creator) succeeds, second (email owner) fails
	permRepo.On("Upsert", mock.Anything, mock.AnythingOfType("*domain.CollectionPermissionEntry")).
		Return(nil).Once()
	permRepo.On("Upsert", mock.Anything, mock.AnythingOfType("*domain.CollectionPermissionEntry")).
		Return(errors.New("db connection lost")).Once()
	userRepo.On("GetByEmail", mock.Anything, tenantID, ownerEmail).
		Return(&domain.User{ID: ownerUserID, TenantID: tenantID, Email: ownerEmail}, nil)

	result, err := svc.Create(context.Background(), &service.CreateCollectionInput{
		TenantID:    tenantID,
		CreatedBy:   creatorID,
		Role:        domain.RoleMember,
		Name:        "Test Collection",
		OwnerEmail:  ownerEmail,
	})

	// Should still succeed — owner_email perm upsert failure is non-blocking
	assert.NoError(t, err)
	assert.NotNil(t, result)
	permRepo.AssertNumberOfCalls(t, "Upsert", 2)
}

func TestCollectionService_Create_ViewerDenied(t *testing.T) {
	svc, _, _, _, _ := setupCollectionService()

	result, err := svc.Create(context.Background(), &service.CreateCollectionInput{
		TenantID:  uuid.New(),
		CreatedBy: uuid.New(),
		Role:      domain.RoleViewer,
		Name:      "Test",
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrInsufficientRole)
}

// --- GetByID ---

func TestCollectionService_GetByID_Success(t *testing.T) {
	svc, collRepo, permRepo, _, _ := setupCollectionService()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	expected := &domain.Collection{ID: collectionID, TenantID: tenantID, Name: "Test", DocumentCount: 5}

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(viewerPerm(collectionID, userID), nil)
	collRepo.On("GetByID", mock.Anything, tenantID, collectionID).Return(expected, nil)

	result, err := svc.GetByID(context.Background(), tenantID, collectionID, userID, domain.RoleMember)

	assert.NoError(t, err)
	assert.Equal(t, expected, result)
	assert.Equal(t, 5, result.DocumentCount)
}

func TestCollectionService_GetByID_NoPermission(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	userID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(nil, domain.ErrCollectionPermDenied)

	result, err := svc.GetByID(context.Background(), uuid.New(), collectionID, userID, domain.RoleViewer)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrCollectionPermDenied)
}

func TestCollectionService_GetByID_AdminBypassesPermission(t *testing.T) {
	svc, collRepo, permRepo, _, _ := setupCollectionService()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	expected := &domain.Collection{ID: collectionID, TenantID: tenantID, Name: "Admin Access", DocumentCount: 10}

	// Admin has no explicit permission, but GetByCollectionAndUser is still called
	// internally by effectivePermission; it returns an error (no explicit perm).
	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(nil, domain.ErrCollectionPermDenied)
	collRepo.On("GetByID", mock.Anything, tenantID, collectionID).Return(expected, nil)

	result, err := svc.GetByID(context.Background(), tenantID, collectionID, userID, domain.RoleAdmin)

	assert.NoError(t, err)
	assert.Equal(t, expected, result)
	assert.Equal(t, 10, result.DocumentCount)
}

func TestCollectionService_GetByID_ViewerNeedsExplicitPerm(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	userID := uuid.New()

	// Viewer with no explicit permission
	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(nil, domain.ErrCollectionPermDenied)

	result, err := svc.GetByID(context.Background(), uuid.New(), collectionID, userID, domain.RoleViewer)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrCollectionPermDenied)
}

// --- List ---

func TestCollectionService_List_Success(t *testing.T) {
	svc, collRepo, _, _, _ := setupCollectionService()

	tenantID := uuid.New()
	userID := uuid.New()

	expected := []domain.Collection{
		{ID: uuid.New(), TenantID: tenantID, Name: "Collection 1", DocumentCount: 3},
		{ID: uuid.New(), TenantID: tenantID, Name: "Collection 2", DocumentCount: 7},
	}

	collRepo.On("ListByTenant", mock.Anything, tenantID, 0, 20).Return(expected, 2, nil)

	collections, total, err := svc.List(context.Background(), tenantID, userID, domain.RoleMember, 0, 20)

	assert.NoError(t, err)
	assert.Len(t, collections, 2)
	assert.Equal(t, 2, total)
	assert.Equal(t, 3, collections[0].DocumentCount)
	assert.Equal(t, 7, collections[1].DocumentCount)
}

func TestCollectionService_List_AdminSeesAll(t *testing.T) {
	svc, collRepo, _, _, _ := setupCollectionService()

	tenantID := uuid.New()
	userID := uuid.New()

	expected := []domain.Collection{
		{ID: uuid.New(), TenantID: tenantID, Name: "Collection 1", DocumentCount: 2},
		{ID: uuid.New(), TenantID: tenantID, Name: "Collection 2", DocumentCount: 4},
		{ID: uuid.New(), TenantID: tenantID, Name: "Collection 3", DocumentCount: 0},
	}

	collRepo.On("ListByTenant", mock.Anything, tenantID, 0, 20).Return(expected, 3, nil)

	collections, total, err := svc.List(context.Background(), tenantID, userID, domain.RoleAdmin, 0, 20)

	assert.NoError(t, err)
	assert.Len(t, collections, 3)
	assert.Equal(t, 3, total)
	collRepo.AssertExpectations(t)
}

func TestCollectionService_List_ViewerSeesOnlyPermitted(t *testing.T) {
	svc, collRepo, _, _, _ := setupCollectionService()

	tenantID := uuid.New()
	userID := uuid.New()

	expected := []domain.Collection{
		{ID: uuid.New(), TenantID: tenantID, Name: "Permitted Collection", DocumentCount: 1},
	}

	collRepo.On("ListByUser", mock.Anything, tenantID, userID, 0, 20).Return(expected, 1, nil)

	collections, total, err := svc.List(context.Background(), tenantID, userID, domain.RoleViewer, 0, 20)

	assert.NoError(t, err)
	assert.Len(t, collections, 1)
	assert.Equal(t, 1, total)
	collRepo.AssertExpectations(t)
}

// --- Update ---

func TestCollectionService_Update_Success(t *testing.T) {
	svc, collRepo, permRepo, _, _ := setupCollectionService()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	existing := &domain.Collection{ID: collectionID, TenantID: tenantID, Name: "Old Name", DocumentCount: 12}

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(ownerPerm(collectionID, userID), nil)
	collRepo.On("GetByID", mock.Anything, tenantID, collectionID).Return(existing, nil)
	collRepo.On("Update", mock.Anything, mock.AnythingOfType("*domain.Collection")).Return(nil)

	result, err := svc.Update(context.Background(), &service.UpdateCollectionInput{
		TenantID:     tenantID,
		CollectionID: collectionID,
		UserID:       userID,
		Role:         domain.RoleMember,
		Name:         "New Name",
		Description:  "New Desc",
	})

	assert.NoError(t, err)
	assert.Equal(t, "New Name", result.Name)
	assert.Equal(t, "New Desc", result.Description)
	assert.Equal(t, 12, result.DocumentCount)
}

func TestCollectionService_Update_NotOwner(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	userID := uuid.New()

	// Viewer role with no explicit perm -> effective = "" < editor -> denied
	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(nil, domain.ErrCollectionPermDenied)

	result, err := svc.Update(context.Background(), &service.UpdateCollectionInput{
		TenantID:     uuid.New(),
		CollectionID: collectionID,
		UserID:       userID,
		Role:         domain.RoleViewer,
		Name:         "New Name",
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrCollectionPermDenied)
}

func TestCollectionService_Update_ManagerCanEdit(t *testing.T) {
	svc, collRepo, permRepo, _, _ := setupCollectionService()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	existing := &domain.Collection{ID: collectionID, TenantID: tenantID, Name: "Old Name", DocumentCount: 4}

	// Manager has no explicit perm, but implicit editor >= editor required
	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(nil, domain.ErrCollectionPermDenied)
	collRepo.On("GetByID", mock.Anything, tenantID, collectionID).Return(existing, nil)
	collRepo.On("Update", mock.Anything, mock.AnythingOfType("*domain.Collection")).Return(nil)

	result, err := svc.Update(context.Background(), &service.UpdateCollectionInput{
		TenantID:     tenantID,
		CollectionID: collectionID,
		UserID:       userID,
		Role:         domain.RoleManager,
		Name:         "Manager Updated",
		Description:  "Updated by manager",
	})

	assert.NoError(t, err)
	assert.Equal(t, "Manager Updated", result.Name)
	assert.Equal(t, "Updated by manager", result.Description)
	assert.Equal(t, 4, result.DocumentCount)
}

func TestCollectionService_Update_MemberWithoutPermDenied(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	userID := uuid.New()

	// Member has implicit viewer, no explicit perm -> effective = viewer < editor -> denied
	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(nil, domain.ErrCollectionPermDenied)

	result, err := svc.Update(context.Background(), &service.UpdateCollectionInput{
		TenantID:     uuid.New(),
		CollectionID: collectionID,
		UserID:       userID,
		Role:         domain.RoleMember,
		Name:         "Should Fail",
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrCollectionPermDenied)
}

// --- Delete ---

func TestCollectionService_Delete_Success(t *testing.T) {
	svc, collRepo, permRepo, _, _ := setupCollectionService()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(ownerPerm(collectionID, userID), nil)
	collRepo.On("Delete", mock.Anything, tenantID, collectionID).Return(nil)

	err := svc.Delete(context.Background(), tenantID, collectionID, userID, domain.RoleMember)

	assert.NoError(t, err)
	collRepo.AssertExpectations(t)
}

func TestCollectionService_Delete_NotOwner(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	userID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(viewerPerm(collectionID, userID), nil)

	err := svc.Delete(context.Background(), uuid.New(), collectionID, userID, domain.RoleMember)

	assert.ErrorIs(t, err, domain.ErrCollectionPermDenied)
}

func TestCollectionService_Delete_AdminCanDeleteAny(t *testing.T) {
	svc, collRepo, permRepo, _, _ := setupCollectionService()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	// Admin has no explicit perm, but implicit owner >= owner required
	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(nil, domain.ErrCollectionPermDenied)
	collRepo.On("Delete", mock.Anything, tenantID, collectionID).Return(nil)

	err := svc.Delete(context.Background(), tenantID, collectionID, userID, domain.RoleAdmin)

	assert.NoError(t, err)
	collRepo.AssertExpectations(t)
}

func TestCollectionService_Delete_ManagerCannotDelete(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	userID := uuid.New()

	// Manager has implicit editor, no explicit perm -> effective = editor < owner -> denied
	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(nil, domain.ErrCollectionPermDenied)

	err := svc.Delete(context.Background(), uuid.New(), collectionID, userID, domain.RoleManager)

	assert.ErrorIs(t, err, domain.ErrCollectionPermDenied)
}

// --- ListFiles ---

func TestCollectionService_ListFiles_Success(t *testing.T) {
	svc, _, permRepo, fileRepo, _ := setupCollectionService()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	expected := []domain.FileMeta{
		{ID: uuid.New(), TenantID: tenantID, Status: domain.FileStatusUploaded},
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(viewerPerm(collectionID, userID), nil)
	fileRepo.On("ListByCollection", mock.Anything, tenantID, collectionID, 0, 20).
		Return(expected, 1, nil)

	files, total, err := svc.ListFiles(context.Background(), tenantID, collectionID, userID, domain.RoleMember, 0, 20)

	assert.NoError(t, err)
	assert.Len(t, files, 1)
	assert.Equal(t, 1, total)
}

func TestCollectionService_ListFiles_NoPermission(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	userID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(nil, domain.ErrCollectionPermDenied)

	files, total, err := svc.ListFiles(context.Background(), uuid.New(), collectionID, userID, domain.RoleViewer, 0, 20)

	assert.Nil(t, files)
	assert.Equal(t, 0, total)
	assert.ErrorIs(t, err, domain.ErrCollectionPermDenied)
}

// --- RemoveFile ---

func TestCollectionService_RemoveFile_Success(t *testing.T) {
	svc, _, permRepo, fileRepo, _ := setupCollectionService()

	collectionID := uuid.New()
	fileID := uuid.New()
	userID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(editorPerm(collectionID, userID), nil)
	fileRepo.On("Remove", mock.Anything, collectionID, fileID).Return(nil)

	err := svc.RemoveFile(context.Background(), uuid.New(), collectionID, fileID, userID, domain.RoleMember)

	assert.NoError(t, err)
}

func TestCollectionService_RemoveFile_ViewerDenied(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	userID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(viewerPerm(collectionID, userID), nil)

	err := svc.RemoveFile(context.Background(), uuid.New(), collectionID, uuid.New(), userID, domain.RoleMember)

	assert.ErrorIs(t, err, domain.ErrCollectionPermDenied)
}

// --- AddFileToCollection ---

func TestCollectionService_AddFileToCollection_Success(t *testing.T) {
	svc, _, permRepo, fileRepo, _ := setupCollectionService()

	tenantID := uuid.New()
	collectionID := uuid.New()
	fileID := uuid.New()
	userID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(editorPerm(collectionID, userID), nil)
	fileRepo.On("Add", mock.Anything, mock.AnythingOfType("*domain.CollectionFile")).Return(nil)

	err := svc.AddFileToCollection(context.Background(), tenantID, collectionID, fileID, userID, domain.RoleMember)

	assert.NoError(t, err)
}

func TestCollectionService_AddFileToCollection_Duplicate(t *testing.T) {
	svc, _, permRepo, fileRepo, _ := setupCollectionService()

	collectionID := uuid.New()
	userID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(editorPerm(collectionID, userID), nil)
	fileRepo.On("Add", mock.Anything, mock.AnythingOfType("*domain.CollectionFile")).
		Return(domain.ErrDuplicateCollectionFile)

	err := svc.AddFileToCollection(context.Background(), uuid.New(), collectionID, uuid.New(), userID, domain.RoleMember)

	assert.ErrorIs(t, err, domain.ErrDuplicateCollectionFile)
}

// --- SetPermission ---

func TestCollectionService_SetPermission_Success(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	tenantID := uuid.New()
	collectionID := uuid.New()
	ownerID := uuid.New()
	targetID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, ownerID).
		Return(ownerPerm(collectionID, ownerID), nil)
	permRepo.On("Upsert", mock.Anything, mock.AnythingOfType("*domain.CollectionPermissionEntry")).Return(nil)

	err := svc.SetPermission(context.Background(), &service.SetPermissionInput{
		TenantID:     tenantID,
		CollectionID: collectionID,
		GrantedBy:    ownerID,
		CallerRole:   domain.RoleAdmin,
		UserID:       targetID,
		Permission:   domain.CollectionPermEditor,
	})

	assert.NoError(t, err)
	permRepo.AssertExpectations(t)
}

func TestCollectionService_SetPermission_InvalidPermission(t *testing.T) {
	svc, _, _, _, _ := setupCollectionService()

	err := svc.SetPermission(context.Background(), &service.SetPermissionInput{
		TenantID:     uuid.New(),
		CollectionID: uuid.New(),
		GrantedBy:    uuid.New(),
		CallerRole:   domain.RoleAdmin,
		UserID:       uuid.New(),
		Permission:   "superadmin",
	})

	assert.ErrorIs(t, err, domain.ErrInvalidPermission)
}

func TestCollectionService_SetPermission_NotOwner(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	editorID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, editorID).
		Return(editorPerm(collectionID, editorID), nil)

	err := svc.SetPermission(context.Background(), &service.SetPermissionInput{
		TenantID:     uuid.New(),
		CollectionID: collectionID,
		GrantedBy:    editorID,
		CallerRole:   domain.RoleMember,
		UserID:       uuid.New(),
		Permission:   domain.CollectionPermViewer,
	})

	assert.ErrorIs(t, err, domain.ErrCollectionPermDenied)
}

// --- ListPermissions ---

func TestCollectionService_ListPermissions_Success(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	ownerID := uuid.New()

	expected := []domain.CollectionPermissionEntry{
		{ID: uuid.New(), CollectionID: collectionID, UserID: ownerID, Permission: domain.CollectionPermOwner},
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, ownerID).
		Return(ownerPerm(collectionID, ownerID), nil)
	permRepo.On("ListByCollection", mock.Anything, collectionID, 0, 20).
		Return(expected, 1, nil)

	perms, total, err := svc.ListPermissions(context.Background(), uuid.New(), collectionID, ownerID, domain.RoleAdmin, 0, 20)

	assert.NoError(t, err)
	assert.Len(t, perms, 1)
	assert.Equal(t, 1, total)
}

func TestCollectionService_ListPermissions_NotOwner(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	viewerID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, viewerID).
		Return(viewerPerm(collectionID, viewerID), nil)

	perms, total, err := svc.ListPermissions(context.Background(), uuid.New(), collectionID, viewerID, domain.RoleViewer, 0, 20)

	assert.Nil(t, perms)
	assert.Equal(t, 0, total)
	assert.ErrorIs(t, err, domain.ErrCollectionPermDenied)
}

// --- RemovePermission ---

func TestCollectionService_RemovePermission_Success(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	ownerID := uuid.New()
	targetID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, ownerID).
		Return(ownerPerm(collectionID, ownerID), nil)
	permRepo.On("Delete", mock.Anything, collectionID, targetID).Return(nil)

	err := svc.RemovePermission(context.Background(), uuid.New(), collectionID, targetID, ownerID, domain.RoleAdmin)

	assert.NoError(t, err)
	permRepo.AssertExpectations(t)
}

func TestCollectionService_RemovePermission_SelfRemoval(t *testing.T) {
	svc, _, _, _, _ := setupCollectionService()

	userID := uuid.New()

	err := svc.RemovePermission(context.Background(), uuid.New(), uuid.New(), userID, userID, domain.RoleMember)

	assert.ErrorIs(t, err, domain.ErrSelfPermissionRemoval)
}

func TestCollectionService_RemovePermission_NotOwner(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	editorID := uuid.New()
	targetID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, editorID).
		Return(editorPerm(collectionID, editorID), nil)

	err := svc.RemovePermission(context.Background(), uuid.New(), collectionID, targetID, editorID, domain.RoleMember)

	assert.ErrorIs(t, err, domain.ErrCollectionPermDenied)
}

// --- EffectivePermission ---

func TestCollectionService_EffectivePermission_AdminReturnsOwner(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	userID := uuid.New()

	// Admin has no explicit permission
	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(nil, domain.ErrCollectionPermDenied)

	perm := svc.EffectivePermission(context.Background(), collectionID, userID, domain.RoleAdmin)

	assert.Equal(t, domain.CollectionPermOwner, perm)
}

func TestCollectionService_EffectivePermission_ViewerWithExplicitOwner(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	userID := uuid.New()

	// Viewer with explicit owner perm gets owner (no cap)
	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(ownerPerm(collectionID, userID), nil)

	perm := svc.EffectivePermission(context.Background(), collectionID, userID, domain.RoleViewer)

	assert.Equal(t, domain.CollectionPermOwner, perm)
}

func TestCollectionService_EffectivePermission_MemberWithExplicitEditor(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	userID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(editorPerm(collectionID, userID), nil)

	perm := svc.EffectivePermission(context.Background(), collectionID, userID, domain.RoleMember)

	assert.Equal(t, domain.CollectionPermEditor, perm)
}

func TestCollectionService_EffectivePermission_MemberNoExplicit(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	collectionID := uuid.New()
	userID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, userID).
		Return(nil, domain.ErrCollectionPermDenied)

	perm := svc.EffectivePermission(context.Background(), collectionID, userID, domain.RoleMember)

	assert.Equal(t, domain.CollectionPermViewer, perm)
}

// --- EffectivePermissions (batch) ---

func TestCollectionService_EffectivePermissions_AdminShortCircuits(t *testing.T) {
	svc, _, _, _, _ := setupCollectionService()

	id1 := uuid.New()
	id2 := uuid.New()
	userID := uuid.New()

	// Admin should not make any DB calls
	result, err := svc.EffectivePermissions(context.Background(), []uuid.UUID{id1, id2}, userID, domain.RoleAdmin)

	assert.NoError(t, err)
	assert.Equal(t, domain.CollectionPermOwner, result[id1])
	assert.Equal(t, domain.CollectionPermOwner, result[id2])
}

func TestCollectionService_EffectivePermissions_ViewerBatchQuery(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	id1 := uuid.New()
	userID := uuid.New()

	// Viewer has no implicit permission; explicit grants determine access
	explicitPerms := map[uuid.UUID]domain.CollectionPermission{}
	permRepo.On("GetByUserForCollections", mock.Anything, userID, []uuid.UUID{id1}).
		Return(explicitPerms, nil)

	result, err := svc.EffectivePermissions(context.Background(), []uuid.UUID{id1}, userID, domain.RoleViewer)

	assert.NoError(t, err)
	// No explicit perm, no implicit → empty permission
	assert.Equal(t, domain.CollectionPermission(""), result[id1])
}

func TestCollectionService_EffectivePermissions_MemberBatchQuery(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	id1 := uuid.New()
	id2 := uuid.New()
	id3 := uuid.New()
	userID := uuid.New()
	collIDs := []uuid.UUID{id1, id2, id3}

	// id1 has explicit editor, id2 has explicit owner, id3 has no explicit perm
	explicitPerms := map[uuid.UUID]domain.CollectionPermission{
		id1: domain.CollectionPermEditor,
		id2: domain.CollectionPermOwner,
	}
	permRepo.On("GetByUserForCollections", mock.Anything, userID, collIDs).
		Return(explicitPerms, nil)

	result, err := svc.EffectivePermissions(context.Background(), collIDs, userID, domain.RoleMember)

	assert.NoError(t, err)
	// Member implicit = viewer; explicit editor > viewer
	assert.Equal(t, domain.CollectionPermEditor, result[id1])
	// Member implicit = viewer; explicit owner > viewer
	assert.Equal(t, domain.CollectionPermOwner, result[id2])
	// Member implicit = viewer; no explicit -> viewer
	assert.Equal(t, domain.CollectionPermViewer, result[id3])
	permRepo.AssertExpectations(t)
}

func TestCollectionService_EffectivePermissions_ManagerBatchQuery(t *testing.T) {
	svc, _, permRepo, _, _ := setupCollectionService()

	id1 := uuid.New()
	id2 := uuid.New()
	userID := uuid.New()
	collIDs := []uuid.UUID{id1, id2}

	// id1 has explicit owner, id2 has no explicit perm
	explicitPerms := map[uuid.UUID]domain.CollectionPermission{
		id1: domain.CollectionPermOwner,
	}
	permRepo.On("GetByUserForCollections", mock.Anything, userID, collIDs).
		Return(explicitPerms, nil)

	result, err := svc.EffectivePermissions(context.Background(), collIDs, userID, domain.RoleManager)

	assert.NoError(t, err)
	// Manager implicit = editor; explicit owner > editor
	assert.Equal(t, domain.CollectionPermOwner, result[id1])
	// Manager implicit = editor; no explicit -> editor
	assert.Equal(t, domain.CollectionPermEditor, result[id2])
	permRepo.AssertExpectations(t)
}

// --- Service Account Collection Creation ---

func setupCollectionServiceWithSA() (
	service.CollectionService,
	*mocks.MockCollectionRepo,
	*mocks.MockServiceAccountPermissionRepo,
) {
	collRepo := new(mocks.MockCollectionRepo)
	permRepo := new(mocks.MockCollectionPermissionRepo)
	fileRepo := new(mocks.MockCollectionFileRepo)
	fileSvc := new(mocks.MockFileService)
	userRepo := new(mocks.MockUserRepo)
	saPermRepo := new(mocks.MockServiceAccountPermissionRepo)
	svc := service.NewCollectionService(collRepo, permRepo, fileRepo, fileSvc, userRepo, saPermRepo)
	return svc, collRepo, saPermRepo
}

func TestCollectionService_Create_ServiceAccountSuccess(t *testing.T) {
	svc, collRepo, saPermRepo := setupCollectionServiceWithSA()

	tenantID := uuid.New()
	saID := uuid.New()

	collRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Collection")).Return(nil)
	saPermRepo.On("Upsert", mock.Anything, mock.AnythingOfType("*domain.ServiceAccountPermission")).Return(nil)

	result, err := svc.Create(context.Background(), &service.CreateCollectionInput{
		TenantID:  tenantID,
		CreatedBy: saID,
		Role:      domain.RoleService,
		Name:      "SA Collection",
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "SA Collection", result.Name)

	// Verify SA perm was upserted, not regular perm
	saPermRepo.AssertCalled(t, "Upsert", mock.Anything, mock.AnythingOfType("*domain.ServiceAccountPermission"))
	collRepo.AssertExpectations(t)
}

func TestCollectionService_Create_ServiceAccountWithoutSAPermRepoDenied(t *testing.T) {
	// Use the regular setup (nil saPermRepo)
	svc, _, _, _, _ := setupCollectionService()

	result, err := svc.Create(context.Background(), &service.CreateCollectionInput{
		TenantID:  uuid.New(),
		CreatedBy: uuid.New(),
		Role:      domain.RoleService,
		Name:      "Should Fail",
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrInsufficientRole)
}

func TestCollectionService_Create_ServiceAccountDenied_StillBlocksViewer(t *testing.T) {
	svc, _, _ := setupCollectionServiceWithSA()

	result, err := svc.Create(context.Background(), &service.CreateCollectionInput{
		TenantID:  uuid.New(),
		CreatedBy: uuid.New(),
		Role:      domain.RoleViewer,
		Name:      "Should Fail",
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrInsufficientRole)
}

func TestCollectionService_BatchUploadFiles_SAPermCheck(t *testing.T) {
	svc, _, saPermRepo := setupCollectionServiceWithSA()

	collectionID := uuid.New()
	saID := uuid.New()

	// SA has editor permission
	saPermRepo.On("GetByAccountAndCollection", mock.Anything, saID, collectionID).
		Return(&domain.ServiceAccountPermission{Permission: domain.CollectionPermEditor}, nil)

	// No files to upload — just verify permission path works
	results, err := svc.BatchUploadFiles(context.Background(), uuid.New(), collectionID, saID, domain.RoleService, nil)

	assert.NoError(t, err)
	assert.Empty(t, results)
	saPermRepo.AssertCalled(t, "GetByAccountAndCollection", mock.Anything, saID, collectionID)
}

func TestCollectionService_GetByID_SAPermCheck(t *testing.T) {
	svc, collRepo, saPermRepo := setupCollectionServiceWithSA()

	tenantID := uuid.New()
	collectionID := uuid.New()
	saID := uuid.New()
	expected := &domain.Collection{ID: collectionID, TenantID: tenantID, Name: "Test"}

	saPermRepo.On("GetByAccountAndCollection", mock.Anything, saID, collectionID).
		Return(&domain.ServiceAccountPermission{Permission: domain.CollectionPermViewer}, nil)
	collRepo.On("GetByID", mock.Anything, tenantID, collectionID).Return(expected, nil)

	result, err := svc.GetByID(context.Background(), tenantID, collectionID, saID, domain.RoleService)

	assert.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestCollectionService_GetByID_SANoPerm(t *testing.T) {
	svc, _, saPermRepo := setupCollectionServiceWithSA()

	collectionID := uuid.New()
	saID := uuid.New()

	saPermRepo.On("GetByAccountAndCollection", mock.Anything, saID, collectionID).
		Return(nil, domain.ErrCollectionPermDenied)

	result, err := svc.GetByID(context.Background(), uuid.New(), collectionID, saID, domain.RoleService)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrCollectionPermDenied)
}

func TestCollectionService_List_ServiceAccount(t *testing.T) {
	svc, collRepo, _ := setupCollectionServiceWithSA()

	tenantID := uuid.New()
	saID := uuid.New()
	expected := []domain.Collection{{ID: uuid.New(), Name: "SA Collection"}}

	collRepo.On("ListByServiceAccount", mock.Anything, tenantID, saID, 0, 20).
		Return(expected, 1, nil)

	collections, total, err := svc.List(context.Background(), tenantID, saID, domain.RoleService, 0, 20)

	assert.NoError(t, err)
	assert.Len(t, collections, 1)
	assert.Equal(t, 1, total)
	collRepo.AssertCalled(t, "ListByServiceAccount", mock.Anything, tenantID, saID, 0, 20)
}

func TestCollectionService_EffectivePermissions_ServiceAccount(t *testing.T) {
	svc, _, saPermRepo := setupCollectionServiceWithSA()

	saID := uuid.New()
	coll1 := uuid.New()
	coll2 := uuid.New()
	collIDs := []uuid.UUID{coll1, coll2}

	saPermRepo.On("GetByAccountForCollections", mock.Anything, saID, collIDs).
		Return(map[uuid.UUID]domain.CollectionPermission{
			coll1: domain.CollectionPermOwner,
			coll2: domain.CollectionPermEditor,
		}, nil)

	result, err := svc.EffectivePermissions(context.Background(), collIDs, saID, domain.RoleService)

	assert.NoError(t, err)
	assert.Equal(t, domain.CollectionPermOwner, result[coll1])
	assert.Equal(t, domain.CollectionPermEditor, result[coll2])
}

func TestCollectionService_SetPermission_SABlocked(t *testing.T) {
	svc, _, _ := setupCollectionServiceWithSA()

	err := svc.SetPermission(context.Background(), &service.SetPermissionInput{
		TenantID:     uuid.New(),
		CollectionID: uuid.New(),
		UserID:       uuid.New(),
		Permission:   domain.CollectionPermEditor,
		GrantedBy:    uuid.New(),
		CallerRole:   domain.RoleService,
	})

	assert.ErrorIs(t, err, domain.ErrInsufficientRole)
}

func TestCollectionService_ListPermissions_SABlocked(t *testing.T) {
	svc, _, _ := setupCollectionServiceWithSA()

	_, _, err := svc.ListPermissions(context.Background(), uuid.New(), uuid.New(), uuid.New(), domain.RoleService, 0, 20)

	assert.ErrorIs(t, err, domain.ErrInsufficientRole)
}

func TestCollectionService_RemovePermission_SABlocked(t *testing.T) {
	svc, _, _ := setupCollectionServiceWithSA()

	err := svc.RemovePermission(context.Background(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), domain.RoleService)

	assert.ErrorIs(t, err, domain.ErrInsufficientRole)
}
