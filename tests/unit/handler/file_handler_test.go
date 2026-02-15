package handler_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/handler"
	"satvos/internal/middleware"
	"satvos/mocks"
)

func setAuthContext(c *gin.Context, tenantID, userID uuid.UUID, role string) {
	c.Set(middleware.ContextKeyTenantID, tenantID)
	c.Set(middleware.ContextKeyUserID, userID)
	c.Set(middleware.ContextKeyRole, role)
	c.Set(middleware.ContextKeyEmail, "user@test.com")
}

func TestFileHandler_Upload_Success(t *testing.T) {
	mockFileSvc := new(mocks.MockFileService)
	h := handler.NewFileHandler(mockFileSvc, nil)

	tenantID := uuid.New()
	userID := uuid.New()
	fileID := uuid.New()

	expectedMeta := &domain.FileMeta{
		ID:           fileID,
		TenantID:     tenantID,
		UploadedBy:   userID,
		FileName:     fileID.String() + ".pdf",
		OriginalName: "test.pdf",
		FileType:     domain.FileTypePDF,
		Status:       domain.FileStatusUploaded,
	}

	mockFileSvc.On("Upload", mock.Anything, mock.AnythingOfType("service.FileUploadInput")).
		Return(expectedMeta, nil)

	// Create multipart form body
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.pdf")
	_, _ = part.Write([]byte("%PDF-1.4 test content"))
	_ = writer.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/files/upload", body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	setAuthContext(c, tenantID, userID, "member")

	h.Upload(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockFileSvc.AssertExpectations(t)
}

func TestFileHandler_Upload_NoFile(t *testing.T) {
	mockFileSvc := new(mocks.MockFileService)
	h := handler.NewFileHandler(mockFileSvc, nil)

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/files/upload", http.NoBody)
	setAuthContext(c, tenantID, userID, "member")

	h.Upload(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestFileHandler_Upload_NoAuthContext(t *testing.T) {
	mockFileSvc := new(mocks.MockFileService)
	h := handler.NewFileHandler(mockFileSvc, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/files/upload", http.NoBody)

	h.Upload(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestFileHandler_List_Success(t *testing.T) {
	mockFileSvc := new(mocks.MockFileService)
	h := handler.NewFileHandler(mockFileSvc, nil)

	tenantID := uuid.New()
	userID := uuid.New()

	files := []domain.FileMeta{
		{ID: uuid.New(), TenantID: tenantID, Status: domain.FileStatusUploaded},
	}

	mockFileSvc.On("List", mock.Anything, tenantID, 0, 20).Return(files, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/files?offset=0&limit=20", http.NoBody)
	setAuthContext(c, tenantID, userID, "member")

	h.List(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Meta)
	mockFileSvc.AssertExpectations(t)
}

func TestFileHandler_GetByID_Success(t *testing.T) {
	mockFileSvc := new(mocks.MockFileService)
	h := handler.NewFileHandler(mockFileSvc, nil)

	tenantID := uuid.New()
	userID := uuid.New()
	fileID := uuid.New()

	meta := &domain.FileMeta{
		ID:       fileID,
		TenantID: tenantID,
		S3Bucket: "test-bucket",
		S3Key:    "test-key",
		Status:   domain.FileStatusUploaded,
	}

	mockFileSvc.On("GetByIDForUser", mock.Anything, tenantID, fileID, userID, domain.UserRole("member")).Return(meta, nil)
	mockFileSvc.On("GetDownloadURL", mock.Anything, tenantID, fileID).
		Return("https://presigned.example.com/test", nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/files/"+fileID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: fileID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.GetByID(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockFileSvc.AssertExpectations(t)
}

func TestFileHandler_GetByID_NotFound(t *testing.T) {
	mockFileSvc := new(mocks.MockFileService)
	h := handler.NewFileHandler(mockFileSvc, nil)

	tenantID := uuid.New()
	userID := uuid.New()
	fileID := uuid.New()

	mockFileSvc.On("GetByIDForUser", mock.Anything, tenantID, fileID, userID, domain.UserRole("member")).Return(nil, domain.ErrNotFound)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/files/"+fileID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: fileID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.GetByID(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestFileHandler_GetByID_InvalidID(t *testing.T) {
	mockFileSvc := new(mocks.MockFileService)
	h := handler.NewFileHandler(mockFileSvc, nil)

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/files/not-a-uuid", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "not-a-uuid"}}
	setAuthContext(c, tenantID, userID, "member")

	h.GetByID(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestFileHandler_Delete_Success(t *testing.T) {
	mockFileSvc := new(mocks.MockFileService)
	h := handler.NewFileHandler(mockFileSvc, nil)

	tenantID := uuid.New()
	userID := uuid.New()
	fileID := uuid.New()

	mockFileSvc.On("Delete", mock.Anything, tenantID, fileID).Return(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodDelete, "/api/v1/files/"+fileID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: fileID.String()}}
	setAuthContext(c, tenantID, userID, "admin")

	h.Delete(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockFileSvc.AssertExpectations(t)
}
