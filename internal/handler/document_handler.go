package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/middleware"
	"satvos/internal/normalize"
	"satvos/internal/port"
	"satvos/internal/service"
)

// allowedFacetKeys defines the tag keys that can be queried via the TagFacets endpoint.
var allowedFacetKeys = map[string]bool{
	"seller_name":     true,
	"buyer_name":      true,
	"seller_gstin":    true,
	"buyer_gstin":     true,
	"invoice_type":    true,
	"place_of_supply": true,
}

// DocumentHandler handles document parsing endpoints.
type DocumentHandler struct {
	documentService service.DocumentService
	auditRepo       port.DocumentAuditRepository
}

// NewDocumentHandler creates a new DocumentHandler.
func NewDocumentHandler(documentService service.DocumentService, auditRepo port.DocumentAuditRepository) *DocumentHandler {
	return &DocumentHandler{documentService: documentService, auditRepo: auditRepo}
}

// Create handles POST /api/v1/documents
// @Summary Create a document
// @Description Create a document from an uploaded file and trigger AI parsing
// @Tags documents
// @Accept json
// @Produce json
// @Param request body CreateDocumentRequest true "Document creation details"
// @Success 201 {object} Response{data=domain.Document} "Document created, parsing started"
// @Failure 400 {object} ErrorResponseBody "Invalid request"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "File or collection not found"
// @Failure 409 {object} ErrorResponseBody "Document already exists for this file"
// @Security BearerAuth
// @Router /documents [post]
func (h *DocumentHandler) Create(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	var req struct {
		FileID       uuid.UUID         `json:"file_id" binding:"required"`
		CollectionID uuid.UUID         `json:"collection_id" binding:"required"`
		DocumentType string            `json:"document_type" binding:"required"`
		ParseMode    domain.ParseMode  `json:"parse_mode"`
		Name         string            `json:"name"`
		Tags         map[string]string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "file_id, collection_id, and document_type are required")
		return
	}

	// Default to single parse mode
	if req.ParseMode == "" {
		req.ParseMode = domain.ParseModeSingle
	}
	if !domain.ValidParseModes[req.ParseMode] {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "parse_mode must be 'single' or 'dual'")
		return
	}

	doc, err := h.documentService.CreateAndParse(c.Request.Context(), &service.CreateDocumentInput{
		TenantID:     tenantID,
		CollectionID: req.CollectionID,
		FileID:       req.FileID,
		DocumentType: req.DocumentType,
		ParseMode:    req.ParseMode,
		Name:         req.Name,
		Tags:         req.Tags,
		CreatedBy:    userID,
		Role:         role,
	})
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondCreated(c, doc)
}

// GetByID handles GET /api/v1/documents/:id
// @Summary Get document by ID
// @Description Get document details including parsed data and validation status
// @Tags documents
// @Produce json
// @Param id path string true "Document ID (UUID)"
// @Success 200 {object} Response{data=domain.Document} "Document details"
// @Failure 400 {object} ErrorResponseBody "Invalid ID"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Document not found"
// @Security BearerAuth
// @Router /documents/{id} [get]
func (h *DocumentHandler) GetByID(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	docID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid document ID")
		return
	}

	doc, err := h.documentService.GetByID(c.Request.Context(), tenantID, docID, userID, role)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, doc)
}

// List handles GET /api/v1/documents
// @Summary List documents
// @Description List documents with optional collection, assignment, status, and tag filters
// @Tags documents
// @Produce json
// @Param offset query int false "Offset for pagination" default(0)
// @Param limit query int false "Limit for pagination (max 100)" default(20)
// @Param collection_id query string false "Filter by collection ID"
// @Param assigned_to query string false "Filter by assigned user ID"
// @Param seller_name query string false "Filter by seller name (exact match via tags)"
// @Param buyer_name query string false "Filter by buyer name (exact match via tags)"
// @Param parsing_status query string false "Filter by parsing status"
// @Param review_status query string false "Filter by review status"
// @Param validation_status query string false "Filter by validation status"
// @Param reconciliation_status query string false "Filter by reconciliation status"
// @Success 200 {object} Response{data=[]domain.Document,meta=PagMeta} "List of documents"
// @Failure 400 {object} ErrorResponseBody "Invalid filter parameter"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Security BearerAuth
// @Router /documents [get]
func (h *DocumentHandler) List(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	offset, limit := parsePagination(c)

	filters := &port.DocumentListFilters{}

	if v := c.Query("assigned_to"); v != "" {
		parsed, err := uuid.Parse(v)
		if err != nil {
			RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid assigned_to")
			return
		}
		filters.AssignedTo = &parsed
	}
	if v := c.Query("seller_name"); v != "" {
		normalized := normalize.CompanyName(v)
		filters.SellerName = &normalized
	}
	if v := c.Query("buyer_name"); v != "" {
		normalized := normalize.CompanyName(v)
		filters.BuyerName = &normalized
	}
	if v := c.Query("parsing_status"); v != "" {
		ps := domain.ParsingStatus(v)
		if !domain.ValidParsingStatuses[ps] {
			RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid parsing_status value")
			return
		}
		filters.ParsingStatus = &ps
	}
	if v := c.Query("review_status"); v != "" {
		rs := domain.ReviewStatus(v)
		if !domain.ValidReviewStatuses[rs] {
			RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid review_status value")
			return
		}
		filters.ReviewStatus = &rs
	}
	if v := c.Query("validation_status"); v != "" {
		vs := domain.ValidationStatus(v)
		if !domain.ValidValidationStatuses[vs] {
			RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid validation_status value")
			return
		}
		filters.ValidationStatus = &vs
	}
	if v := c.Query("reconciliation_status"); v != "" {
		rs := domain.ReconciliationStatus(v)
		if !domain.ValidReconciliationStatuses[rs] {
			RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid reconciliation_status value")
			return
		}
		filters.ReconciliationStatus = &rs
	}

	collectionIDStr := c.Query("collection_id")
	if collectionIDStr != "" {
		collectionID, err := uuid.Parse(collectionIDStr)
		if err != nil {
			RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid collection_id")
			return
		}
		docs, total, err := h.documentService.ListByCollection(c.Request.Context(), tenantID, collectionID, userID, role, filters, offset, limit)
		if err != nil {
			HandleError(c, err)
			return
		}
		RespondPaginated(c, docs, PagMeta{Total: total, Offset: offset, Limit: limit})
		return
	}

	docs, total, err := h.documentService.ListByTenant(c.Request.Context(), tenantID, userID, role, filters, offset, limit)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, docs, PagMeta{Total: total, Offset: offset, Limit: limit})
}

// TagFacets handles GET /api/v1/documents/tags/facets
// @Summary Get tag facets for filter dropdowns
// @Description Returns distinct tag values with document counts, role-scoped
// @Tags documents
// @Produce json
// @Param keys query string true "Comma-separated tag keys (e.g. seller_name,buyer_name)"
// @Success 200 {object} Response{data=map[string][]port.TagFacet} "Tag facets"
// @Failure 400 {object} ErrorResponseBody "Invalid or missing keys"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Security BearerAuth
// @Router /documents/tags/facets [get]
func (h *DocumentHandler) TagFacets(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	keysStr := c.Query("keys")
	if keysStr == "" {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "keys query parameter is required")
		return
	}

	keys := strings.Split(keysStr, ",")
	for _, k := range keys {
		if !allowedFacetKeys[k] {
			RespondError(c, http.StatusBadRequest, "INVALID_REQUEST",
				fmt.Sprintf("invalid facet key %q", k))
			return
		}
	}

	facets, err := h.documentService.GetTagFacets(
		c.Request.Context(), tenantID, userID, role, keys)
	if err != nil {
		HandleError(c, err)
		return
	}
	RespondOK(c, facets)
}

// Retry handles POST /api/v1/documents/:id/retry
// @Summary Retry document parsing
// @Description Re-trigger AI parsing for a failed document
// @Tags documents
// @Produce json
// @Param id path string true "Document ID (UUID)"
// @Success 200 {object} Response{data=domain.Document} "Parsing re-triggered"
// @Failure 400 {object} ErrorResponseBody "Invalid ID"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Document not found"
// @Security BearerAuth
// @Router /documents/{id}/retry [post]
func (h *DocumentHandler) Retry(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	docID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid document ID")
		return
	}

	doc, err := h.documentService.RetryParse(c.Request.Context(), tenantID, docID, userID, role)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, doc)
}

// UpdateReview handles PUT /api/v1/documents/:id/review
// @Summary Review a document
// @Description Approve or reject a parsed document
// @Tags documents
// @Accept json
// @Produce json
// @Param id path string true "Document ID (UUID)"
// @Param request body ReviewDocumentRequest true "Review decision"
// @Success 200 {object} Response{data=domain.Document} "Document reviewed"
// @Failure 400 {object} ErrorResponseBody "Invalid request or document not parsed"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Document not found"
// @Security BearerAuth
// @Router /documents/{id}/review [put]
func (h *DocumentHandler) UpdateReview(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	docID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid document ID")
		return
	}

	var req struct {
		Status domain.ReviewStatus `json:"status" binding:"required"`
		Notes  string              `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "status is required (approved or rejected)")
		return
	}

	if req.Status != domain.ReviewStatusApproved && req.Status != domain.ReviewStatusRejected {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "status must be 'approved' or 'rejected'")
		return
	}

	doc, err := h.documentService.UpdateReview(c.Request.Context(), &service.UpdateReviewInput{
		TenantID:   tenantID,
		DocumentID: docID,
		ReviewerID: userID,
		Role:       role,
		Status:     req.Status,
		Notes:      req.Notes,
	})
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, doc)
}

// AssignDocument handles PUT /api/v1/documents/:id/assign
// @Summary Assign a document for review
// @Description Assign or unassign a document to a user for review. Assignee must have editor+ permission on the collection. Pass null assignee_id to unassign.
// @Tags documents
// @Accept json
// @Produce json
// @Param id path string true "Document ID (UUID)"
// @Param request body object{assignee_id=string} true "Assignee user ID (null to unassign)"
// @Success 200 {object} Response{data=domain.Document} "Document assigned"
// @Failure 400 {object} ErrorResponseBody "Invalid request, document not parsed, or assignee cannot review"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Document or assignee not found"
// @Security BearerAuth
// @Router /documents/{id}/assign [put]
func (h *DocumentHandler) AssignDocument(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	docID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid document ID")
		return
	}

	var req struct {
		AssigneeID *string `json:"assignee_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "request body is required")
		return
	}

	var assigneeID *uuid.UUID
	if req.AssigneeID != nil && *req.AssigneeID != "" {
		parsed, err := uuid.Parse(*req.AssigneeID)
		if err != nil {
			RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid assignee_id format")
			return
		}
		assigneeID = &parsed
	}

	doc, err := h.documentService.AssignDocument(c.Request.Context(), &service.AssignDocumentInput{
		TenantID:   tenantID,
		DocumentID: docID,
		CallerID:   userID,
		CallerRole: role,
		AssigneeID: assigneeID,
	})
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, doc)
}

// ReviewQueue handles GET /api/v1/documents/review-queue
// @Summary Get review queue
// @Description List documents assigned to the current user that are parsed and pending review, ordered by assignment date
// @Tags documents
// @Produce json
// @Param offset query int false "Pagination offset" default(0)
// @Param limit query int false "Pagination limit" default(20)
// @Success 200 {object} Response{data=[]domain.Document,meta=PagMeta} "Review queue"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Security BearerAuth
// @Router /documents/review-queue [get]
func (h *DocumentHandler) ReviewQueue(c *gin.Context) {
	tenantID, userID, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	offset, limit := parsePagination(c)

	docs, total, err := h.documentService.ListReviewQueue(c.Request.Context(), tenantID, userID, offset, limit)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, docs, PagMeta{Total: total, Offset: offset, Limit: limit})
}

// EditStructuredData handles PUT /api/v1/documents/:id and PUT /api/v1/documents/:id/structured-data
// @Summary Edit structured data
// @Description Manually edit the parsed structured data of a document, re-run validation and auto-tag extraction
// @Tags documents
// @Accept json
// @Produce json
// @Param id path string true "Document ID (UUID)"
// @Param request body EditStructuredDataRequest true "Structured data (GSTInvoice)"
// @Success 200 {object} Response{data=domain.Document} "Document updated with new structured data"
// @Failure 400 {object} ErrorResponseBody "Invalid request, document not parsed, or invalid structured data"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Document not found"
// @Security BearerAuth
// @Router /documents/{id} [put]
// @Router /documents/{id}/structured-data [put]
func (h *DocumentHandler) EditStructuredData(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	docID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid document ID")
		return
	}

	var req struct {
		StructuredData json.RawMessage `json:"structured_data" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "structured_data is required")
		return
	}

	doc, err := h.documentService.EditStructuredData(c.Request.Context(), &service.EditStructuredDataInput{
		TenantID:       tenantID,
		DocumentID:     docID,
		UserID:         userID,
		Role:           role,
		StructuredData: req.StructuredData,
	})
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, doc)
}

// Validate handles POST /api/v1/documents/:id/validate
// @Summary Re-run validation
// @Description Re-run the validation engine on a parsed document
// @Tags documents
// @Produce json
// @Param id path string true "Document ID (UUID)"
// @Success 200 {object} Response{data=MessageResponse} "Validation completed"
// @Failure 400 {object} ErrorResponseBody "Invalid ID or document not parsed"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 404 {object} ErrorResponseBody "Document not found"
// @Security BearerAuth
// @Router /documents/{id}/validate [post]
func (h *DocumentHandler) Validate(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	docID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid document ID")
		return
	}

	if err := h.documentService.ValidateDocument(c.Request.Context(), tenantID, docID, userID, role); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "validation completed"})
}

// GetValidation handles GET /api/v1/documents/:id/validation
// @Summary Get validation results
// @Description Get detailed validation results including per-rule and per-field status
// @Tags documents
// @Produce json
// @Param id path string true "Document ID (UUID)"
// @Success 200 {object} Response{data=ValidationResponse} "Validation results"
// @Failure 400 {object} ErrorResponseBody "Invalid ID or document not parsed"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 404 {object} ErrorResponseBody "Document not found"
// @Security BearerAuth
// @Router /documents/{id}/validation [get]
func (h *DocumentHandler) GetValidation(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	docID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid document ID")
		return
	}

	result, err := h.documentService.GetValidation(c.Request.Context(), tenantID, docID, userID, role)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, result)
}

// ListTags handles GET /api/v1/documents/:id/tags
// @Summary List document tags
// @Description List all tags (user and auto-generated) for a document
// @Tags documents
// @Produce json
// @Param id path string true "Document ID (UUID)"
// @Success 200 {object} Response{data=[]domain.DocumentTag} "List of tags"
// @Failure 400 {object} ErrorResponseBody "Invalid ID"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Document not found"
// @Security BearerAuth
// @Router /documents/{id}/tags [get]
func (h *DocumentHandler) ListTags(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	docID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid document ID")
		return
	}

	tags, err := h.documentService.ListTags(c.Request.Context(), tenantID, docID, userID, role)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, tags)
}

// AddTags handles POST /api/v1/documents/:id/tags
// @Summary Add tags to a document
// @Description Add user tags to a document (requires editor+ permission)
// @Tags documents
// @Accept json
// @Produce json
// @Param id path string true "Document ID (UUID)"
// @Param request body AddTagsRequest true "Tags to add"
// @Success 201 {object} Response{data=[]domain.DocumentTag} "Tags added"
// @Failure 400 {object} ErrorResponseBody "Invalid request"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Document not found"
// @Security BearerAuth
// @Router /documents/{id}/tags [post]
func (h *DocumentHandler) AddTags(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	docID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid document ID")
		return
	}

	var req struct {
		Tags map[string]string `json:"tags" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "tags map is required")
		return
	}

	tags, err := h.documentService.AddTags(c.Request.Context(), tenantID, docID, userID, role, req.Tags)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondCreated(c, tags)
}

// DeleteTag handles DELETE /api/v1/documents/:id/tags/:tagId
// @Summary Delete a tag
// @Description Delete a user tag from a document (requires editor+ permission)
// @Tags documents
// @Produce json
// @Param id path string true "Document ID (UUID)"
// @Param tagId path string true "Tag ID (UUID)"
// @Success 200 {object} Response{data=MessageResponse} "Tag deleted"
// @Failure 400 {object} ErrorResponseBody "Invalid ID"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Document or tag not found"
// @Security BearerAuth
// @Router /documents/{id}/tags/{tagId} [delete]
func (h *DocumentHandler) DeleteTag(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	docID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid document ID")
		return
	}

	tagID, err := uuid.Parse(c.Param("tagId"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid tag ID")
		return
	}

	if err := h.documentService.DeleteTag(c.Request.Context(), tenantID, docID, userID, role, tagID); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "tag deleted"})
}

// SearchByTag handles GET /api/v1/documents/search/tags
// @Summary Search documents by tag
// @Description Search for documents with a specific tag key-value pair
// @Tags documents
// @Produce json
// @Param key query string true "Tag key"
// @Param value query string true "Tag value"
// @Param offset query int false "Offset for pagination" default(0)
// @Param limit query int false "Limit for pagination (max 100)" default(20)
// @Success 200 {object} Response{data=[]domain.Document,meta=PagMeta} "Matching documents"
// @Failure 400 {object} ErrorResponseBody "Missing key or value"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Security BearerAuth
// @Router /documents/search/tags [get]
func (h *DocumentHandler) SearchByTag(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	key := c.Query("key")
	value := c.Query("value")
	if key == "" || value == "" {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "key and value query parameters are required")
		return
	}

	if key == "seller_name" || key == "buyer_name" {
		value = normalize.CompanyName(value)
	}

	offset, limit := parsePagination(c)

	docs, total, err := h.documentService.SearchByTag(c.Request.Context(), tenantID, key, value, offset, limit)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, docs, PagMeta{Total: total, Offset: offset, Limit: limit})
}

// Delete handles DELETE /api/v1/documents/:id
// @Summary Delete a document
// @Description Delete a document (admin only)
// @Tags documents
// @Produce json
// @Param id path string true "Document ID (UUID)"
// @Success 200 {object} Response{data=MessageResponse} "Document deleted"
// @Failure 400 {object} ErrorResponseBody "Invalid ID"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Forbidden - admin only"
// @Failure 404 {object} ErrorResponseBody "Document not found"
// @Security BearerAuth
// @Router /documents/{id} [delete]
func (h *DocumentHandler) Delete(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	docID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid document ID")
		return
	}

	if err := h.documentService.Delete(c.Request.Context(), tenantID, docID, userID, role); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "document deleted"})
}

// ListAudit handles GET /api/v1/documents/:id/audit
func (h *DocumentHandler) ListAudit(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	docID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid document ID")
		return
	}

	offset, limit := parsePagination(c)

	entries, total, err := h.auditRepo.ListByDocument(c.Request.Context(), tenantID, docID, offset, limit)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, entries, PagMeta{Total: total, Offset: offset, Limit: limit})
}
