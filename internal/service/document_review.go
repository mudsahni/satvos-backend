package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/validator"
	"satvos/internal/validator/invoice"
)

func (s *documentService) UpdateReview(ctx context.Context, input *UpdateReviewInput) (*domain.Document, error) {
	// Service accounts cannot review documents
	if input.Role == domain.RoleService {
		return nil, domain.ErrServiceAccountReview
	}

	doc, err := s.docRepo.GetByID(ctx, input.TenantID, input.DocumentID)
	if err != nil {
		return nil, err
	}

	// Check editor+ permission on the collection
	if err := s.requireCollectionPerm(ctx, doc.CollectionID, input.ReviewerID, input.Role, domain.CollectionPermEditor); err != nil {
		return nil, err
	}

	if doc.ParsingStatus != domain.ParsingStatusCompleted {
		return nil, domain.ErrDocumentNotParsed
	}

	now := time.Now().UTC()
	doc.ReviewStatus = input.Status
	doc.ReviewedBy = &input.ReviewerID
	doc.ReviewedAt = &now
	doc.ReviewerNotes = input.Notes

	// Auto-set sync_review_status based on review decision
	switch input.Status {
	case domain.ReviewStatusApproved:
		doc.SyncReviewStatus = domain.SyncReviewPending
	case domain.ReviewStatusRejected:
		doc.SyncReviewStatus = domain.SyncReviewNotApplicable
	}

	if err := s.docRepo.UpdateReviewStatus(ctx, doc); err != nil {
		return nil, fmt.Errorf("updating review status: %w", err)
	}

	reviewChanges, _ := json.Marshal(map[string]interface{}{"status": string(input.Status), "notes": input.Notes})
	s.audit(ctx, input.TenantID, input.DocumentID, &input.ReviewerID, domain.AuditDocumentReview, reviewChanges)

	// Update summary statuses after review
	s.updateSummaryStatuses(ctx, doc)

	return doc, nil
}

func (s *documentService) AssignDocument(ctx context.Context, input *AssignDocumentInput) (*domain.Document, error) {
	// Service accounts cannot assign or be assigned documents
	if input.CallerRole == domain.RoleService {
		return nil, domain.ErrServiceAccountReview
	}

	doc, err := s.docRepo.GetByID(ctx, input.TenantID, input.DocumentID)
	if err != nil {
		return nil, err
	}

	// Check editor+ permission on the collection (caller)
	if err := s.requireCollectionPerm(ctx, doc.CollectionID, input.CallerID, input.CallerRole, domain.CollectionPermEditor); err != nil {
		return nil, err
	}

	if doc.ParsingStatus != domain.ParsingStatusCompleted {
		return nil, domain.ErrDocumentNotParsed
	}

	previousAssignee := doc.AssignedTo

	if input.AssigneeID != nil {
		// Verify assignee exists in tenant
		assignee, err := s.userRepo.GetByID(ctx, input.TenantID, *input.AssigneeID)
		if err != nil {
			return nil, fmt.Errorf("assignee not found: %w", err)
		}

		// Verify assignee has editor+ effective permission on collection
		if err := s.requireCollectionPerm(ctx, doc.CollectionID, *input.AssigneeID, assignee.Role, domain.CollectionPermEditor); err != nil {
			return nil, domain.ErrAssigneeCannotReview
		}

		now := time.Now().UTC()
		doc.AssignedTo = input.AssigneeID
		doc.AssignedAt = &now
		doc.AssignedBy = &input.CallerID
	} else {
		// Unassign
		doc.AssignedTo = nil
		doc.AssignedAt = nil
		doc.AssignedBy = nil
	}

	if err := s.docRepo.UpdateAssignment(ctx, doc); err != nil {
		return nil, fmt.Errorf("updating assignment: %w", err)
	}

	// Audit
	var changes json.RawMessage
	if input.AssigneeID != nil {
		changes, _ = json.Marshal(map[string]interface{}{
			"assigned_to": input.AssigneeID.String(), "assigned_by": input.CallerID.String(),
		})
	} else {
		prev := ""
		if previousAssignee != nil {
			prev = previousAssignee.String()
		}
		changes, _ = json.Marshal(map[string]interface{}{
			"assigned_to": nil, "assigned_by": input.CallerID.String(), "previous_assignee": prev,
		})
	}
	s.audit(ctx, input.TenantID, input.DocumentID, &input.CallerID, domain.AuditDocumentAssigned, changes)

	return doc, nil
}

func (s *documentService) EditStructuredData(ctx context.Context, input *EditStructuredDataInput) (*domain.Document, error) {
	doc, err := s.docRepo.GetByID(ctx, input.TenantID, input.DocumentID)
	if err != nil {
		return nil, err
	}

	// Check editor+ permission on the collection
	if err := s.requireCollectionPerm(ctx, doc.CollectionID, input.UserID, input.Role, domain.CollectionPermEditor); err != nil {
		return nil, err
	}

	if doc.ParsingStatus != domain.ParsingStatusCompleted {
		return nil, domain.ErrDocumentNotParsed
	}

	// Validate that the structured data can be unmarshalled into GSTInvoice
	var inv invoice.GSTInvoice
	if err := json.Unmarshal(input.StructuredData, &inv); err != nil {
		return nil, domain.ErrInvalidStructuredData
	}

	// Update document fields
	doc.StructuredData = input.StructuredData
	doc.ConfidenceScores = json.RawMessage("{}")
	doc.FieldProvenance = json.RawMessage(`{"_source":"manual_edit"}`)

	// Reset validation and reconciliation status
	doc.ValidationStatus = domain.ValidationStatusPending
	doc.ValidationResults = json.RawMessage("[]")
	doc.ReconciliationStatus = domain.ReconciliationStatusPending

	// Persist structured data changes
	if err := s.docRepo.UpdateStructuredData(ctx, doc); err != nil {
		return nil, fmt.Errorf("updating structured data: %w", err)
	}

	s.audit(ctx, input.TenantID, input.DocumentID, &input.UserID, domain.AuditDocumentEditStructured, json.RawMessage(`{"provenance":"manual_edit"}`))

	// Reset review status
	doc.ReviewStatus = domain.ReviewStatusPending
	doc.ReviewedBy = nil
	doc.ReviewedAt = nil
	doc.ReviewerNotes = ""

	if err := s.docRepo.UpdateReviewStatus(ctx, doc); err != nil {
		return nil, fmt.Errorf("resetting review status: %w", err)
	}

	// Re-extract auto-tags from the already-parsed invoice (reuse inv from above)
	if s.tagRepo != nil {
		s.extractAndSaveAutoTags(ctx, doc.ID, doc.TenantID, &inv)
	}

	// Run validation synchronously
	if s.validator != nil {
		if err := s.validator.ValidateDocument(ctx, input.TenantID, input.DocumentID); err != nil {
			log.Printf("documentService.EditStructuredData: validation failed for %s: %v", input.DocumentID, err)
		}
	}

	// Re-fetch to get updated validation results
	updated, err := s.docRepo.GetByID(ctx, input.TenantID, input.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("re-fetching document after edit: %w", err)
	}

	// Upsert document summary for reporting (reuse inv from above)
	s.upsertSummary(ctx, updated, &inv)

	if s.validator != nil {
		s.auditValidationCompleted(ctx, input.TenantID, input.DocumentID, &input.UserID, "edit")
		// Update summary statuses after validation
		s.updateSummaryStatuses(ctx, updated)
	}

	return updated, nil
}

func (s *documentService) ValidateDocument(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) error {
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
	if s.validator == nil {
		return fmt.Errorf("validation engine not configured")
	}
	s.audit(ctx, tenantID, docID, &userID, domain.AuditDocumentValidate, nil)
	if err := s.validator.ValidateDocument(ctx, tenantID, docID); err != nil {
		return err
	}
	s.auditValidationCompleted(ctx, tenantID, docID, &userID, "manual")
	return nil
}

func (s *documentService) GetValidation(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) (*validator.ValidationResponse, error) {
	doc, err := s.docRepo.GetByID(ctx, tenantID, docID)
	if err != nil {
		return nil, err
	}
	if err := s.requireCollectionPerm(ctx, doc.CollectionID, userID, role, domain.CollectionPermViewer); err != nil {
		return nil, err
	}
	if s.validator == nil {
		return nil, fmt.Errorf("validation engine not configured")
	}
	return s.validator.GetValidation(ctx, tenantID, docID)
}
