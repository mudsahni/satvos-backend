package service

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/port"
)

// EffectiveCollectionPerm computes the effective collection permission for a user
// based on their tenant role and any explicit collection-level grant.
// effective = max(implicit_from_role, explicit_collection_perm)
func EffectiveCollectionPerm(ctx context.Context, permRepo port.CollectionPermissionRepository, collectionID, userID uuid.UUID, role domain.UserRole) domain.CollectionPermission {
	implicit := domain.ImplicitCollectionPerm(role)

	explicit := domain.CollectionPermission("")
	perm, err := permRepo.GetByCollectionAndUser(ctx, collectionID, userID)
	if err == nil {
		explicit = perm.Permission
	}

	if domain.CollectionPermLevel(implicit) >= domain.CollectionPermLevel(explicit) {
		return implicit
	}
	return explicit
}

// RequireCollectionPerm checks that the user has at least the minimum permission on a collection.
func RequireCollectionPerm(ctx context.Context, permRepo port.CollectionPermissionRepository, collectionID, userID uuid.UUID, role domain.UserRole, minLevel domain.CollectionPermission) error {
	eff := EffectiveCollectionPerm(ctx, permRepo, collectionID, userID, role)
	if domain.CollectionPermLevel(eff) < domain.CollectionPermLevel(minLevel) {
		return domain.ErrCollectionPermDenied
	}
	return nil
}

// ServiceAccountCollectionPerm looks up the explicit collection permission for a service account.
// Service accounts have no implicit permissions — only explicit grants in service_account_permissions.
func ServiceAccountCollectionPerm(ctx context.Context, saPermRepo port.ServiceAccountPermissionRepository, collectionID, serviceAccountID uuid.UUID) domain.CollectionPermission {
	perm, err := saPermRepo.GetByAccountAndCollection(ctx, serviceAccountID, collectionID)
	if err != nil {
		log.Printf("ServiceAccountCollectionPerm: SA %s has no permission on collection %s: %v", serviceAccountID, collectionID, err)
		return ""
	}
	log.Printf("ServiceAccountCollectionPerm: SA %s has %s on collection %s", serviceAccountID, perm.Permission, collectionID)
	return perm.Permission
}

// RequireServiceAccountCollectionPerm checks that a service account has at least the minimum permission.
func RequireServiceAccountCollectionPerm(ctx context.Context, saPermRepo port.ServiceAccountPermissionRepository, collectionID, serviceAccountID uuid.UUID, minLevel domain.CollectionPermission) error {
	eff := ServiceAccountCollectionPerm(ctx, saPermRepo, collectionID, serviceAccountID)
	if domain.CollectionPermLevel(eff) < domain.CollectionPermLevel(minLevel) {
		return domain.ErrCollectionPermDenied
	}
	return nil
}

// CreatePersonalCollection creates a personal collection for a user and assigns them owner permission.
func CreatePersonalCollection(ctx context.Context, collRepo port.CollectionRepository, permRepo port.CollectionPermissionRepository, tenantID, userID uuid.UUID, userName string) (*domain.Collection, error) {
	collection := &domain.Collection{
		ID:          uuid.New(),
		TenantID:    tenantID,
		Name:        userName + "'s Invoices",
		Description: "Personal invoice collection",
		CreatedBy:   userID,
	}
	if err := collRepo.Create(ctx, collection); err != nil {
		return nil, fmt.Errorf("creating personal collection: %w", err)
	}

	ownerPerm := &domain.CollectionPermissionEntry{
		CollectionID: collection.ID,
		TenantID:     tenantID,
		UserID:       userID,
		Permission:   domain.CollectionPermOwner,
		GrantedBy:    userID,
	}
	if err := permRepo.Upsert(ctx, ownerPerm); err != nil {
		return nil, fmt.Errorf("assigning collection permission: %w", err)
	}

	return collection, nil
}
