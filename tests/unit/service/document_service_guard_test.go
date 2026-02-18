package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"satvos/internal/domain"
	"satvos/internal/service"
)

func TestDocumentService_ServiceAccountReviewBlocked(t *testing.T) {
	t.Run("update_review_blocked_for_service_role", func(t *testing.T) {
		svc, _, _, _, _, _, _, _, _ := setupDocumentService()

		_, err := svc.UpdateReview(context.Background(), &service.UpdateReviewInput{
			TenantID:   uuid.New(),
			DocumentID: uuid.New(),
			ReviewerID: uuid.New(),
			Role:       domain.RoleService,
			Status:     domain.ReviewStatusApproved,
		})

		assert.ErrorIs(t, err, domain.ErrServiceAccountReview)
	})

	t.Run("assign_document_blocked_for_service_role", func(t *testing.T) {
		svc, _, _, _, _, _, _, _, _ := setupDocumentService()

		assigneeID := uuid.New()
		_, err := svc.AssignDocument(context.Background(), &service.AssignDocumentInput{
			TenantID:   uuid.New(),
			DocumentID: uuid.New(),
			CallerID:   uuid.New(),
			CallerRole: domain.RoleService,
			AssigneeID: &assigneeID,
		})

		assert.ErrorIs(t, err, domain.ErrServiceAccountReview)
	})
}
