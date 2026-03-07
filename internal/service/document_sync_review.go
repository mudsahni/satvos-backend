package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/port"
)

func (s *documentService) ListSyncReviewQueue(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, filters *port.SyncReviewFilters, offset, limit int) ([]domain.Document, int, error) {
	// Service accounts cannot access sync review queue
	if role == domain.RoleService {
		return nil, 0, domain.ErrInsufficientRole
	}

	// If filtering by collection, verify permission
	if filters != nil && filters.CollectionID != nil {
		if err := s.requireCollectionPerm(ctx, *filters.CollectionID, userID, role, domain.CollectionPermViewer); err != nil {
			return nil, 0, err
		}
	}

	return s.docRepo.ListSyncReviewQueue(ctx, tenantID, filters, offset, limit)
}

func (s *documentService) ApproveSyncBatch(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, docIDs []uuid.UUID) error {
	if role == domain.RoleService {
		return domain.ErrInsufficientRole
	}

	// Verify editor+ permission and pending status on each doc
	for _, docID := range docIDs {
		doc, err := s.docRepo.GetByID(ctx, tenantID, docID)
		if err != nil {
			return err
		}
		if err := s.requireCollectionPerm(ctx, doc.CollectionID, userID, role, domain.CollectionPermEditor); err != nil {
			return err
		}
		if doc.SyncReviewStatus != domain.SyncReviewPending {
			return domain.ErrSyncReviewNotPending
		}
	}

	if err := s.docRepo.BatchUpdateSyncReviewStatus(ctx, tenantID, docIDs, domain.SyncReviewApproved, &userID); err != nil {
		return err
	}

	// Audit each doc
	for _, docID := range docIDs {
		changes, _ := json.Marshal(map[string]interface{}{"approved_by": userID.String()})
		s.audit(ctx, tenantID, docID, &userID, domain.AuditSyncReviewApproved, changes)
	}

	return nil
}

func (s *documentService) RejectSyncBatch(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, docIDs []uuid.UUID, reason string) error {
	if role == domain.RoleService {
		return domain.ErrInsufficientRole
	}

	// Verify editor+ permission and pending status on each doc
	for _, docID := range docIDs {
		doc, err := s.docRepo.GetByID(ctx, tenantID, docID)
		if err != nil {
			return err
		}
		if err := s.requireCollectionPerm(ctx, doc.CollectionID, userID, role, domain.CollectionPermEditor); err != nil {
			return err
		}
		if doc.SyncReviewStatus != domain.SyncReviewPending {
			return domain.ErrSyncReviewNotPending
		}
	}

	if err := s.docRepo.BatchUpdateSyncReviewStatus(ctx, tenantID, docIDs, domain.SyncReviewRejected, nil); err != nil {
		return err
	}

	for _, docID := range docIDs {
		changes, _ := json.Marshal(map[string]interface{}{"rejected_by": userID.String(), "reason": reason})
		s.audit(ctx, tenantID, docID, &userID, domain.AuditSyncReviewRejected, changes)
	}

	return nil
}

func (s *documentService) ApproveCollectionSync(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, collectionID uuid.UUID) error {
	if role == domain.RoleService {
		return domain.ErrInsufficientRole
	}

	if err := s.requireCollectionPerm(ctx, collectionID, userID, role, domain.CollectionPermEditor); err != nil {
		return err
	}

	// List all pending docs in collection
	filters := &port.SyncReviewFilters{
		Status:       domain.SyncReviewPending,
		CollectionID: &collectionID,
	}
	docs, _, err := s.docRepo.ListSyncReviewQueue(ctx, tenantID, filters, 0, 10000)
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return nil
	}

	docIDs := make([]uuid.UUID, len(docs))
	for i := range docs {
		docIDs[i] = docs[i].ID
	}

	if err := s.docRepo.BatchUpdateSyncReviewStatus(ctx, tenantID, docIDs, domain.SyncReviewApproved, &userID); err != nil {
		return err
	}

	for _, docID := range docIDs {
		changes, _ := json.Marshal(map[string]interface{}{
			"approved_by":   userID.String(),
			"collection_id": collectionID.String(),
			"batch":         true,
		})
		s.audit(ctx, tenantID, docID, &userID, domain.AuditSyncReviewApproved, changes)
	}

	return nil
}

func (s *documentService) UpdateVoucherOverrides(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, docID uuid.UUID, overrides *domain.VoucherOverrides) error {
	if role == domain.RoleService {
		return domain.ErrInsufficientRole
	}

	doc, err := s.docRepo.GetByID(ctx, tenantID, docID)
	if err != nil {
		return err
	}

	if err := s.requireCollectionPerm(ctx, doc.CollectionID, userID, role, domain.CollectionPermEditor); err != nil {
		return err
	}

	if doc.ParsingStatus != domain.ParsingStatusCompleted {
		return domain.ErrDocumentNotParsed
	}

	overridesJSON, err := json.Marshal(overrides)
	if err != nil {
		return fmt.Errorf("marshaling overrides: %w", err)
	}

	if err := s.docRepo.UpdateVoucherOverrides(ctx, tenantID, docID, overridesJSON); err != nil {
		return err
	}

	changes, _ := json.Marshal(overrides)
	s.audit(ctx, tenantID, docID, &userID, domain.AuditVoucherOverrideUpdated, changes)

	return nil
}
