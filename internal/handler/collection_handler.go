package handler

import (
	"fmt"
	"log"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"satvos/internal/csvexport"
	"satvos/internal/domain"
	"satvos/internal/service"
	"satvos/internal/tallyexport"
)

// CollectionHandler handles collection management endpoints.
type CollectionHandler struct {
	collectionService service.CollectionService
	documentService   service.DocumentService
}

// NewCollectionHandler creates a new CollectionHandler.
func NewCollectionHandler(collectionService service.CollectionService, documentService service.DocumentService) *CollectionHandler {
	return &CollectionHandler{collectionService: collectionService, documentService: documentService}
}

// Create handles POST /api/v1/collections
// @Summary Create a collection
// @Description Create a new collection for grouping files
// @Tags collections
// @Accept json
// @Produce json
// @Param request body CreateCollectionRequest true "Collection details"
// @Success 201 {object} Response{data=domain.Collection} "Collection created"
// @Failure 400 {object} ErrorResponseBody "Invalid request"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient role"
// @Security BearerAuth
// @Router /collections [post]
func (h *CollectionHandler) Create(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "name is required")
		return
	}

	collection, err := h.collectionService.Create(c.Request.Context(), &service.CreateCollectionInput{
		TenantID:    tenantID,
		CreatedBy:   userID,
		Role:        role,
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondCreated(c, collection)
}

// List handles GET /api/v1/collections
// @Summary List collections
// @Description List collections the user has access to
// @Tags collections
// @Produce json
// @Param offset query int false "Offset for pagination" default(0)
// @Param limit query int false "Limit for pagination (max 100)" default(20)
// @Success 200 {object} Response{data=[]domain.Collection,meta=PagMeta} "List of collections"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Security BearerAuth
// @Router /collections [get]
func (h *CollectionHandler) List(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	offset, limit := parsePagination(c)

	collections, total, err := h.collectionService.List(c.Request.Context(), tenantID, userID, role, offset, limit)
	if err != nil {
		HandleError(c, err)
		return
	}

	// Enrich collections with the current user's effective permission
	collectionIDs := make([]uuid.UUID, len(collections))
	for i := range collections {
		collectionIDs[i] = collections[i].ID
	}
	permMap, err := h.collectionService.EffectivePermissions(c.Request.Context(), collectionIDs, userID, role)
	if err != nil {
		HandleError(c, err)
		return
	}

	type collectionWithPermission struct {
		domain.Collection     `json:",inline"`
		CurrentUserPermission domain.CollectionPermission `json:"current_user_permission"`
	}
	enriched := make([]collectionWithPermission, len(collections))
	for i := range collections {
		enriched[i] = collectionWithPermission{
			Collection:            collections[i],
			CurrentUserPermission: permMap[collections[i].ID],
		}
	}

	RespondPaginated(c, enriched, PagMeta{Total: total, Offset: offset, Limit: limit})
}

// GetByID handles GET /api/v1/collections/:id
// @Summary Get collection by ID
// @Description Get collection details with paginated files
// @Tags collections
// @Produce json
// @Param id path string true "Collection ID (UUID)"
// @Param offset query int false "Offset for files pagination" default(0)
// @Param limit query int false "Limit for files pagination (max 100)" default(20)
// @Success 200 {object} Response{data=CollectionWithFiles} "Collection with files"
// @Failure 400 {object} ErrorResponseBody "Invalid ID"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Collection not found"
// @Security BearerAuth
// @Router /collections/{id} [get]
func (h *CollectionHandler) GetByID(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	collectionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid collection ID")
		return
	}

	collection, err := h.collectionService.GetByID(c.Request.Context(), tenantID, collectionID, userID, role)
	if err != nil {
		HandleError(c, err)
		return
	}

	// Also fetch files for the collection (first page)
	offset, limit := parsePagination(c)
	files, totalFiles, err := h.collectionService.ListFiles(c.Request.Context(), tenantID, collectionID, userID, role, offset, limit)
	if err != nil {
		HandleError(c, err)
		return
	}

	perm := h.collectionService.EffectivePermission(c.Request.Context(), collectionID, userID, role)

	RespondOK(c, gin.H{
		"collection":              collection,
		"current_user_permission": perm,
		"files":                   files,
		"files_meta":              PagMeta{Total: totalFiles, Offset: offset, Limit: limit},
	})
}

// Update handles PUT /api/v1/collections/:id
// @Summary Update a collection
// @Description Update collection metadata (requires editor+ permission)
// @Tags collections
// @Accept json
// @Produce json
// @Param id path string true "Collection ID (UUID)"
// @Param request body UpdateCollectionRequest true "Updated collection details"
// @Success 200 {object} Response{data=domain.Collection} "Collection updated"
// @Failure 400 {object} ErrorResponseBody "Invalid request"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Collection not found"
// @Security BearerAuth
// @Router /collections/{id} [put]
func (h *CollectionHandler) Update(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	collectionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid collection ID")
		return
	}

	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "name is required")
		return
	}

	collection, err := h.collectionService.Update(c.Request.Context(), &service.UpdateCollectionInput{
		TenantID:     tenantID,
		CollectionID: collectionID,
		UserID:       userID,
		Role:         role,
		Name:         req.Name,
		Description:  req.Description,
	})
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, collection)
}

// Delete handles DELETE /api/v1/collections/:id
// @Summary Delete a collection
// @Description Delete a collection (requires owner permission or admin role). Files are preserved.
// @Tags collections
// @Produce json
// @Param id path string true "Collection ID (UUID)"
// @Success 200 {object} Response{data=MessageResponse} "Collection deleted"
// @Failure 400 {object} ErrorResponseBody "Invalid ID"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Collection not found"
// @Security BearerAuth
// @Router /collections/{id} [delete]
func (h *CollectionHandler) Delete(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	collectionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid collection ID")
		return
	}

	if err := h.collectionService.Delete(c.Request.Context(), tenantID, collectionID, userID, role); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "collection deleted"})
}

// BatchUploadFiles handles POST /api/v1/collections/:id/files
// @Summary Batch upload files to a collection
// @Description Upload multiple files to a collection. Returns 201 if all succeed, 207 on partial success.
// @Tags collections
// @Accept multipart/form-data
// @Produce json
// @Param id path string true "Collection ID (UUID)"
// @Param files formData file true "Files to upload (multiple)"
// @Success 201 {object} Response{data=[]BatchUploadResult} "All files uploaded successfully"
// @Success 207 {object} Response{data=[]BatchUploadResult} "Partial success"
// @Failure 400 {object} ErrorResponseBody "Invalid request"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Collection not found"
// @Security BearerAuth
// @Router /collections/{id}/files [post]
func (h *CollectionHandler) BatchUploadFiles(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	collectionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid collection ID")
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "multipart form is required")
		return
	}

	fileHeaders := form.File["files"]
	if len(fileHeaders) == 0 {
		RespondError(c, http.StatusBadRequest, "MISSING_FILES", "at least one file is required in 'files' field")
		return
	}

	inputs := make([]service.BatchUploadFileInput, 0, len(fileHeaders))
	openFiles := make([]multipart.File, 0, len(fileHeaders))
	defer func() {
		for _, f := range openFiles {
			_ = f.Close()
		}
	}()
	for _, fh := range fileHeaders {
		f, err := fh.Open()
		if err != nil {
			RespondError(c, http.StatusBadRequest, "FILE_READ_ERROR", "failed to read uploaded file")
			return
		}
		openFiles = append(openFiles, f)
		inputs = append(inputs, service.BatchUploadFileInput{
			File:   f,
			Header: fh,
		})
	}

	results, err := h.collectionService.BatchUploadFiles(c.Request.Context(), tenantID, collectionID, userID, role, inputs)
	if err != nil {
		HandleError(c, err)
		return
	}

	// Check if all succeeded or some failed
	allSuccess := true
	for _, r := range results {
		if !r.Success {
			allSuccess = false
			break
		}
	}

	if allSuccess {
		RespondCreated(c, results)
	} else {
		// 207 Multi-Status for partial success
		c.JSON(http.StatusMultiStatus, APIResponse{Success: true, Data: results})
	}
}

// RemoveFile handles DELETE /api/v1/collections/:id/files/:fileId
// @Summary Remove a file from a collection
// @Description Remove file association from collection (file itself is not deleted)
// @Tags collections
// @Produce json
// @Param id path string true "Collection ID (UUID)"
// @Param fileId path string true "File ID (UUID)"
// @Success 200 {object} Response{data=MessageResponse} "File removed from collection"
// @Failure 400 {object} ErrorResponseBody "Invalid ID"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Collection or file not found"
// @Security BearerAuth
// @Router /collections/{id}/files/{fileId} [delete]
func (h *CollectionHandler) RemoveFile(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	collectionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid collection ID")
		return
	}

	fileID, err := uuid.Parse(c.Param("fileId"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid file ID")
		return
	}

	if err := h.collectionService.RemoveFile(c.Request.Context(), tenantID, collectionID, fileID, userID, role); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "file removed from collection"})
}

// SetPermission handles POST /api/v1/collections/:id/permissions
// @Summary Set user permission on a collection
// @Description Grant or update a user's permission on a collection (requires owner permission)
// @Tags collections
// @Accept json
// @Produce json
// @Param id path string true "Collection ID (UUID)"
// @Param request body SetPermissionRequest true "Permission details"
// @Success 200 {object} Response{data=MessageResponse} "Permission set"
// @Failure 400 {object} ErrorResponseBody "Invalid request"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Collection not found"
// @Security BearerAuth
// @Router /collections/{id}/permissions [post]
func (h *CollectionHandler) SetPermission(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	collectionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid collection ID")
		return
	}

	var req struct {
		UserID     uuid.UUID                   `json:"user_id" binding:"required"`
		Permission domain.CollectionPermission `json:"permission" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "user_id and permission are required")
		return
	}

	if err := h.collectionService.SetPermission(c.Request.Context(), &service.SetPermissionInput{
		TenantID:     tenantID,
		CollectionID: collectionID,
		GrantedBy:    userID,
		CallerRole:   role,
		UserID:       req.UserID,
		Permission:   req.Permission,
	}); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "permission set"})
}

// ListPermissions handles GET /api/v1/collections/:id/permissions
// @Summary List collection permissions
// @Description List all permission entries for a collection (requires owner permission)
// @Tags collections
// @Produce json
// @Param id path string true "Collection ID (UUID)"
// @Param offset query int false "Offset for pagination" default(0)
// @Param limit query int false "Limit for pagination (max 100)" default(20)
// @Success 200 {object} Response{data=[]domain.CollectionPermissionEntry,meta=PagMeta} "List of permissions"
// @Failure 400 {object} ErrorResponseBody "Invalid ID"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Collection not found"
// @Security BearerAuth
// @Router /collections/{id}/permissions [get]
func (h *CollectionHandler) ListPermissions(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	collectionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid collection ID")
		return
	}

	offset, limit := parsePagination(c)

	perms, total, err := h.collectionService.ListPermissions(c.Request.Context(), tenantID, collectionID, userID, role, offset, limit)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, perms, PagMeta{Total: total, Offset: offset, Limit: limit})
}

// RemovePermission handles DELETE /api/v1/collections/:id/permissions/:userId
// @Summary Remove user permission from a collection
// @Description Remove a user's permission from a collection (requires owner permission, cannot remove self)
// @Tags collections
// @Produce json
// @Param id path string true "Collection ID (UUID)"
// @Param userId path string true "User ID (UUID)"
// @Success 200 {object} Response{data=MessageResponse} "Permission removed"
// @Failure 400 {object} ErrorResponseBody "Invalid ID or self-removal attempt"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Collection not found"
// @Security BearerAuth
// @Router /collections/{id}/permissions/{userId} [delete]
func (h *CollectionHandler) RemovePermission(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	collectionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid collection ID")
		return
	}

	targetUserID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid user ID")
		return
	}

	if err := h.collectionService.RemovePermission(c.Request.Context(), tenantID, collectionID, targetUserID, userID, role); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "permission removed"})
}

// ExportCSV handles GET /api/v1/collections/:id/export/csv
// @Summary Export collection documents as CSV
// @Description Download collection documents as CSV with one row per invoice line item for GST reconciliation
// @Tags collections
// @Produce text/csv
// @Param id path string true "Collection ID (UUID)"
// @Success 200 {file} file "CSV file"
// @Failure 400 {object} ErrorResponseBody "Invalid ID"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Insufficient permission"
// @Failure 404 {object} ErrorResponseBody "Collection not found"
// @Security BearerAuth
// @Router /collections/{id}/export/csv [get]
func (h *CollectionHandler) ExportCSV(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	collectionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid collection ID")
		return
	}

	// Permission check + get collection name
	collection, err := h.collectionService.GetByID(c.Request.Context(), tenantID, collectionID, userID, role)
	if err != nil {
		HandleError(c, err)
		return
	}

	// Set response headers before streaming
	filename := csvexport.BuildFilename(collection.Name)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%q"`, filename))

	// Write UTF-8 BOM for Excel compatibility
	if _, err := c.Writer.Write(csvexport.BOM); err != nil {
		log.Printf("ERROR: csv export BOM write failed: %v", err)
		return
	}

	w := csvexport.NewWriter(c.Writer)
	if err := w.WriteHeader(); err != nil {
		log.Printf("ERROR: csv export header write failed: %v", err)
		return
	}

	// Batch-load documents
	const batchSize = 200
	offset := 0
	for {
		docs, total, err := h.documentService.ListByCollection(
			c.Request.Context(), tenantID, collectionID, userID, role, nil, offset, batchSize,
		)
		if err != nil {
			log.Printf("ERROR: csv export document fetch failed at offset %d: %v", offset, err)
			return
		}

		if err := w.WriteDocuments(docs); err != nil {
			log.Printf("ERROR: csv export write failed at offset %d: %v", offset, err)
			return
		}

		offset += batchSize
		if offset >= total {
			break
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		log.Printf("ERROR: csv export flush failed: %v", err)
	}
}

// ExportTally handles GET /api/v1/collections/:id/export/tally
func (h *CollectionHandler) ExportTally(c *gin.Context) {
	tenantID, userID, role, ok := extractAuthContext(c)
	if !ok {
		return
	}

	collectionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid collection ID")
		return
	}

	collection, err := h.collectionService.GetByID(c.Request.Context(), tenantID, collectionID, userID, role)
	if err != nil {
		HandleError(c, err)
		return
	}

	companyName := c.Query("company_name")
	if companyName == "" {
		companyName = collection.Name
	}

	filename := tallyexport.BuildFilename(collection.Name)
	c.Header("Content-Type", "application/xml; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))

	opts := &tallyexport.TallyExportOptions{CompanyName: companyName}
	tw := tallyexport.NewWriter(c.Writer, opts)

	const batchSize = 200
	offset := 0
	for {
		docs, total, err := h.documentService.ListByCollection(
			c.Request.Context(), tenantID, collectionID, userID, role, nil, offset, batchSize,
		)
		if err != nil {
			log.Printf("ERROR: tally export document fetch failed at offset %d: %v", offset, err)
			return
		}

		tw.WriteDocuments(docs)

		offset += batchSize
		if offset >= total {
			break
		}
	}

	if err := tw.Flush(); err != nil {
		log.Printf("ERROR: tally export flush failed: %v", err)
	}
}
