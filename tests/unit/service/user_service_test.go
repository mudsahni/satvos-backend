package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/service"
	"satvos/mocks"
)

func TestUserService_Create_Success(t *testing.T) {
	repo := new(mocks.MockUserRepo)
	svc := service.NewUserService(repo)

	tenantID := uuid.New()

	repo.On("Create", mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)

	user, err := svc.Create(context.Background(), tenantID, service.CreateUserInput{
		Email:    "new@test.com",
		Password: "securepassword123",
		FullName: "New User",
		Role:     domain.RoleMember,
	})

	assert.NoError(t, err)
	assert.Equal(t, "new@test.com", user.Email)
	assert.Equal(t, "New User", user.FullName)
	assert.Equal(t, domain.RoleMember, user.Role)
	assert.True(t, user.IsActive)
	assert.NotEmpty(t, user.PasswordHash)
	assert.Equal(t, tenantID, user.TenantID)
	repo.AssertExpectations(t)
}

func TestUserService_Create_NoPassword_InvitationFlow(t *testing.T) {
	repo := new(mocks.MockUserRepo)
	svc := service.NewUserService(repo)

	tenantID := uuid.New()

	repo.On("Create", mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)

	user, err := svc.Create(context.Background(), tenantID, service.CreateUserInput{
		Email:    "invited@test.com",
		FullName: "Invited User",
		Role:     domain.RoleMember,
	})

	assert.NoError(t, err)
	assert.Equal(t, "invited@test.com", user.Email)
	assert.Equal(t, "Invited User", user.FullName)
	assert.Equal(t, domain.RoleMember, user.Role)
	assert.True(t, user.IsActive)
	assert.Empty(t, user.PasswordHash)
	assert.False(t, user.EmailVerified)
	assert.Equal(t, tenantID, user.TenantID)
	repo.AssertExpectations(t)
}

func TestUserService_Create_DuplicateEmail(t *testing.T) {
	repo := new(mocks.MockUserRepo)
	svc := service.NewUserService(repo)

	repo.On("Create", mock.Anything, mock.AnythingOfType("*domain.User")).Return(domain.ErrDuplicateEmail)

	user, err := svc.Create(context.Background(), uuid.New(), service.CreateUserInput{
		Email:    "existing@test.com",
		Password: "password123",
		FullName: "Test User",
		Role:     domain.RoleMember,
	})

	assert.Nil(t, user)
	assert.ErrorIs(t, err, domain.ErrDuplicateEmail)
}

func TestUserService_GetByID_Success(t *testing.T) {
	repo := new(mocks.MockUserRepo)
	svc := service.NewUserService(repo)

	tenantID := uuid.New()
	userID := uuid.New()
	expected := &domain.User{ID: userID, TenantID: tenantID, Email: "user@test.com"}

	repo.On("GetByID", mock.Anything, tenantID, userID).Return(expected, nil)

	user, err := svc.GetByID(context.Background(), tenantID, userID)

	assert.NoError(t, err)
	assert.Equal(t, expected, user)
}

func TestUserService_GetByID_NotFound(t *testing.T) {
	repo := new(mocks.MockUserRepo)
	svc := service.NewUserService(repo)

	tenantID := uuid.New()
	userID := uuid.New()

	repo.On("GetByID", mock.Anything, tenantID, userID).Return(nil, domain.ErrNotFound)

	user, err := svc.GetByID(context.Background(), tenantID, userID)

	assert.Nil(t, user)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestUserService_List_Success(t *testing.T) {
	repo := new(mocks.MockUserRepo)
	svc := service.NewUserService(repo)

	tenantID := uuid.New()
	expected := []domain.User{
		{ID: uuid.New(), TenantID: tenantID, Email: "a@test.com"},
		{ID: uuid.New(), TenantID: tenantID, Email: "b@test.com"},
	}

	repo.On("ListByTenant", mock.Anything, tenantID, 0, 20).Return(expected, 2, nil)

	users, total, err := svc.List(context.Background(), tenantID, 0, 20)

	assert.NoError(t, err)
	assert.Len(t, users, 2)
	assert.Equal(t, 2, total)
}

func TestUserService_Update_Success(t *testing.T) {
	repo := new(mocks.MockUserRepo)
	svc := service.NewUserService(repo)

	tenantID := uuid.New()
	userID := uuid.New()
	existing := &domain.User{
		ID:       userID,
		TenantID: tenantID,
		Email:    "old@test.com",
		FullName: "Old Name",
		Role:     domain.RoleMember,
		IsActive: true,
	}
	newName := "New Name"
	newRole := domain.RoleAdmin

	repo.On("GetByID", mock.Anything, tenantID, userID).Return(existing, nil)
	repo.On("Update", mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)

	user, err := svc.Update(context.Background(), tenantID, userID, service.UpdateUserInput{
		FullName: &newName,
		Role:     &newRole,
	})

	assert.NoError(t, err)
	assert.Equal(t, "New Name", user.FullName)
	assert.Equal(t, domain.RoleAdmin, user.Role)
	repo.AssertExpectations(t)
}

func TestUserService_Delete_Success(t *testing.T) {
	repo := new(mocks.MockUserRepo)
	svc := service.NewUserService(repo)

	tenantID := uuid.New()
	userID := uuid.New()

	repo.On("Delete", mock.Anything, tenantID, userID).Return(nil)

	err := svc.Delete(context.Background(), tenantID, userID)

	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestUserService_Delete_NotFound(t *testing.T) {
	repo := new(mocks.MockUserRepo)
	svc := service.NewUserService(repo)

	tenantID := uuid.New()
	userID := uuid.New()

	repo.On("Delete", mock.Anything, tenantID, userID).Return(domain.ErrNotFound)

	err := svc.Delete(context.Background(), tenantID, userID)

	assert.ErrorIs(t, err, domain.ErrNotFound)
}
