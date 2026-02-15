package handler

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/middleware"
	"satvos/internal/service"
)

// FileHandler handles file upload and management endpoints.
type FileHandler struct {
	fileService       service.FileService
	collectionService service.CollectionService
}

// NewFileHandler creates a new FileHandler.
func NewFileHandler(fileService service.FileService, collectionService service.CollectionService) *FileHandler {
	return &FileHandler{fileService: fileService, collectionService: collectionService}
}

// Upload handles POST /api/v1/files/upload
// @Summary Upload a file
// @Description Upload a file (PDF, JPG, PNG, max 50MB) with optional collection association
// @Tags files
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "File to upload (PDF, JPG, or PNG)"
// @Param collection_id formData string false "Collection ID to add file to"
// @Success 201 {object} Response{data=domain.FileMeta} "File uploaded successfully"
// @Failure 400 {object} ErrorResponseBody "Missing file or unsupported type"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 413 {object} ErrorResponseBody "File too large"
// @Failure 500 {object} ErrorResponseBody "Upload failed"
// @Security BearerAuth
// @Router /files/upload [post]
func (h *FileHandler) Upload(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}
	userID, err := middleware.GetUserID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing user context")
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		RespondError(c, http.StatusBadRequest, "MISSING_FILE", "file field is required")
		return
	}
	defer func() { _ = file.Close() }()

	input := service.FileUploadInput{
		TenantID:   tenantID,
		UploadedBy: userID,
		File:       file,
		Header:     header,
	}

	meta, err := h.fileService.Upload(c.Request.Context(), input)
	if err != nil {
		HandleError(c, err)
		return
	}

	role := domain.UserRole(middleware.GetRole(c))

	// Optional: add file to a collection if collection_id is provided
	var warning string
	if collectionIDStr := c.PostForm("collection_id"); collectionIDStr != "" {
		collectionID, parseErr := uuid.Parse(collectionIDStr)
		if parseErr != nil {
			warning = "invalid collection_id format; file uploaded but not added to collection"
		} else {
			addErr := h.collectionService.AddFileToCollection(c.Request.Context(), tenantID, collectionID, meta.ID, userID, role)
			if addErr != nil {
				log.Printf("fileHandler.Upload: failed to add file %s to collection %s: %v",
					meta.ID, collectionID, addErr)
				warning = "file uploaded but failed to add to collection: " + addErr.Error()
			}
		}
	}

	if warning != "" {
		c.JSON(http.StatusCreated, APIResponse{
			Success: true,
			Data: gin.H{
				"file":    meta,
				"warning": warning,
			},
		})
		return
	}

	RespondCreated(c, meta)
}

// List handles GET /api/v1/files
// @Summary List files
// @Description List all files for the tenant with pagination
// @Tags files
// @Produce json
// @Param offset query int false "Offset for pagination" default(0)
// @Param limit query int false "Limit for pagination (max 100)" default(20)
// @Success 200 {object} Response{data=[]domain.FileMeta,meta=PagMeta} "List of files"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Security BearerAuth
// @Router /files [get]
func (h *FileHandler) List(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}
	userID, err := middleware.GetUserID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing user context")
		return
	}
	role := domain.UserRole(middleware.GetRole(c))

	offset, limit := parsePagination(c)

	var files []domain.FileMeta
	var total int
	if role == domain.RoleFree {
		files, total, err = h.fileService.ListByUploader(c.Request.Context(), tenantID, userID, offset, limit)
	} else {
		files, total, err = h.fileService.List(c.Request.Context(), tenantID, offset, limit)
	}
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, files, PagMeta{Total: total, Offset: offset, Limit: limit})
}

// GetByID handles GET /api/v1/files/:id
// @Summary Get file by ID
// @Description Get file metadata and a presigned download URL
// @Tags files
// @Produce json
// @Param id path string true "File ID (UUID)"
// @Success 200 {object} Response{data=FileWithDownloadURL} "File metadata with download URL"
// @Failure 400 {object} ErrorResponseBody "Invalid ID"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 404 {object} ErrorResponseBody "File not found"
// @Security BearerAuth
// @Router /files/{id} [get]
func (h *FileHandler) GetByID(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}
	userID, err := middleware.GetUserID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing user context")
		return
	}
	role := domain.UserRole(middleware.GetRole(c))

	fileID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid file ID")
		return
	}

	meta, err := h.fileService.GetByIDForUser(c.Request.Context(), tenantID, fileID, userID, role)
	if err != nil {
		HandleError(c, err)
		return
	}

	downloadURL, err := h.fileService.GetDownloadURL(c.Request.Context(), tenantID, fileID)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{
		"file":         meta,
		"download_url": downloadURL,
	})
}

// Delete handles DELETE /api/v1/files/:id
// @Summary Delete a file
// @Description Delete a file (admin only)
// @Tags files
// @Produce json
// @Param id path string true "File ID (UUID)"
// @Success 200 {object} Response{data=MessageResponse} "File deleted"
// @Failure 400 {object} ErrorResponseBody "Invalid ID"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Forbidden - admin only"
// @Failure 404 {object} ErrorResponseBody "File not found"
// @Security BearerAuth
// @Router /files/{id} [delete]
func (h *FileHandler) Delete(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	fileID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid file ID")
		return
	}

	if err := h.fileService.Delete(c.Request.Context(), tenantID, fileID); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "file deleted"})
}
