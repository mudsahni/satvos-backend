package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/handler"
	"satvos/internal/service"
	"satvos/internal/validator"
	"satvos/mocks"
)

func newDocumentHandler() (*handler.DocumentHandler, *mocks.MockDocumentService) {
	mockSvc := new(mocks.MockDocumentService)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	h := handler.NewDocumentHandler(mockSvc, auditRepo, nil, nil)
	return h, mockSvc
}

// --- Create ---

func TestDocumentHandler_Create_Success(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	fileID := uuid.New()
	collectionID := uuid.New()
	docID := uuid.New()

	expected := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		CollectionID:  collectionID,
		FileID:        fileID,
		DocumentType:  "invoice",
		ParsingStatus: domain.ParsingStatusPending,
		ReviewStatus:  domain.ReviewStatusPending,
	}

	mockSvc.On("CreateAndParse", mock.Anything, mock.MatchedBy(func(input *service.CreateDocumentInput) bool {
		return input.TenantID == tenantID &&
			input.FileID == fileID &&
			input.CollectionID == collectionID &&
			input.DocumentType == "invoice" &&
			input.CreatedBy == userID &&
			input.Role == domain.UserRole("member")
	})).Return(expected, nil)

	body, _ := json.Marshal(map[string]string{
		"file_id":       fileID.String(),
		"collection_id": collectionID.String(),
		"document_type": "invoice",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "member")

	h.Create(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_Create_MissingFields(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	// Missing document_type
	body, _ := json.Marshal(map[string]string{
		"file_id":       uuid.New().String(),
		"collection_id": uuid.New().String(),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "member")

	h.Create(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_Create_NoAuth(t *testing.T) {
	h, _ := newDocumentHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents", http.NoBody)

	h.Create(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDocumentHandler_Create_DuplicateDocument(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	mockSvc.On("CreateAndParse", mock.Anything, mock.AnythingOfType("*service.CreateDocumentInput")).
		Return(nil, domain.ErrDocumentAlreadyExists)

	body, _ := json.Marshal(map[string]string{
		"file_id":       uuid.New().String(),
		"collection_id": uuid.New().String(),
		"document_type": "invoice",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "member")

	h.Create(c)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestDocumentHandler_Create_FileNotFound(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	mockSvc.On("CreateAndParse", mock.Anything, mock.AnythingOfType("*service.CreateDocumentInput")).
		Return(nil, domain.ErrNotFound)

	body, _ := json.Marshal(map[string]string{
		"file_id":       uuid.New().String(),
		"collection_id": uuid.New().String(),
		"document_type": "invoice",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "member")

	h.Create(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- Create with parse_mode ---

func TestDocumentHandler_Create_WithParseMode(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	fileID := uuid.New()
	collectionID := uuid.New()
	docID := uuid.New()

	expected := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParseMode:     domain.ParseModeDual,
		ParsingStatus: domain.ParsingStatusPending,
	}

	mockSvc.On("CreateAndParse", mock.Anything, mock.MatchedBy(func(input *service.CreateDocumentInput) bool {
		return input.ParseMode == domain.ParseModeDual
	})).Return(expected, nil)

	body, _ := json.Marshal(map[string]string{
		"file_id":       fileID.String(),
		"collection_id": collectionID.String(),
		"document_type": "invoice",
		"parse_mode":    "dual",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "member")

	h.Create(c)

	assert.Equal(t, http.StatusCreated, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_Create_DefaultParseMode(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	fileID := uuid.New()
	collectionID := uuid.New()
	docID := uuid.New()

	expected := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParseMode:     domain.ParseModeSingle,
		ParsingStatus: domain.ParsingStatusPending,
	}

	mockSvc.On("CreateAndParse", mock.Anything, mock.MatchedBy(func(input *service.CreateDocumentInput) bool {
		return input.ParseMode == domain.ParseModeSingle
	})).Return(expected, nil)

	body, _ := json.Marshal(map[string]string{
		"file_id":       fileID.String(),
		"collection_id": collectionID.String(),
		"document_type": "invoice",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "member")

	h.Create(c)

	assert.Equal(t, http.StatusCreated, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_Create_InvalidParseMode(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	body, _ := json.Marshal(map[string]string{
		"file_id":       uuid.New().String(),
		"collection_id": uuid.New().String(),
		"document_type": "invoice",
		"parse_mode":    "triple",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "member")

	h.Create(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp.Success)
}

// --- GetByID ---

func TestDocumentHandler_GetByID_Success(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	expected := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusCompleted,
	}

	mockSvc.On("GetByID", mock.Anything, tenantID, docID, userID, domain.UserRole("member")).Return(expected, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/"+docID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.GetByID(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_GetByID_NotFound(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("GetByID", mock.Anything, tenantID, docID, userID, domain.UserRole("member")).Return(nil, domain.ErrDocumentNotFound)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/"+docID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.GetByID(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDocumentHandler_GetByID_InvalidID(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/not-a-uuid", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "not-a-uuid"}}
	setAuthContext(c, tenantID, userID, "member")

	h.GetByID(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- List ---

func TestDocumentHandler_List_ByTenant(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	docs := []domain.Document{
		{ID: uuid.New(), TenantID: tenantID, ParsingStatus: domain.ParsingStatusCompleted},
	}

	mockSvc.On("ListByTenant", mock.Anything, tenantID, userID, domain.UserRole("member"), mock.Anything, 0, 20).Return(docs, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents?offset=0&limit=20", http.NoBody)
	setAuthContext(c, tenantID, userID, "member")

	h.List(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Meta)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_List_ByCollection(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	docs := []domain.Document{
		{ID: uuid.New(), TenantID: tenantID, CollectionID: collectionID},
	}

	mockSvc.On("ListByCollection", mock.Anything, tenantID, collectionID, userID, domain.UserRole("member"), mock.Anything, 0, 20).
		Return(docs, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet,
		"/api/v1/documents?collection_id="+collectionID.String()+"&offset=0&limit=20", http.NoBody)
	setAuthContext(c, tenantID, userID, "member")

	h.List(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Meta)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_List_InvalidCollectionID(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet,
		"/api/v1/documents?collection_id=not-a-uuid", http.NoBody)
	setAuthContext(c, tenantID, userID, "member")

	h.List(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_List_NoAuth(t *testing.T) {
	h, _ := newDocumentHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents", http.NoBody)

	h.List(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- Retry ---

func TestDocumentHandler_Retry_Success(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	expected := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusPending,
	}

	mockSvc.On("RetryParse", mock.Anything, tenantID, docID, userID, domain.UserRole("member")).Return(expected, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents/"+docID.String()+"/retry", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.Retry(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_Retry_NotFound(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("RetryParse", mock.Anything, tenantID, docID, userID, domain.UserRole("member")).Return(nil, domain.ErrDocumentNotFound)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents/"+docID.String()+"/retry", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.Retry(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDocumentHandler_Retry_InvalidID(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents/bad-id/retry", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "bad-id"}}
	setAuthContext(c, tenantID, userID, "member")

	h.Retry(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- UpdateReview ---

func TestDocumentHandler_UpdateReview_Approved(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	expected := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ReviewStatus:  domain.ReviewStatusApproved,
		ReviewerNotes: "Verified",
	}

	mockSvc.On("UpdateReview", mock.Anything, mock.MatchedBy(func(input *service.UpdateReviewInput) bool {
		return input.TenantID == tenantID &&
			input.DocumentID == docID &&
			input.ReviewerID == userID &&
			input.Status == domain.ReviewStatusApproved &&
			input.Notes == "Verified"
	})).Return(expected, nil)

	body, _ := json.Marshal(map[string]string{
		"status": "approved",
		"notes":  "Verified",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/review", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.UpdateReview(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_UpdateReview_Rejected(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	expected := &domain.Document{
		ID:           docID,
		ReviewStatus: domain.ReviewStatusRejected,
	}

	mockSvc.On("UpdateReview", mock.Anything, mock.AnythingOfType("*service.UpdateReviewInput")).
		Return(expected, nil)

	body, _ := json.Marshal(map[string]string{
		"status": "rejected",
		"notes":  "Wrong amounts",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/review", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.UpdateReview(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDocumentHandler_UpdateReview_InvalidStatus(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	body, _ := json.Marshal(map[string]string{
		"status": "maybe",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/review", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.UpdateReview(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_UpdateReview_MissingStatus(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	body, _ := json.Marshal(map[string]string{
		"notes": "No status provided",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/review", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.UpdateReview(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_UpdateReview_NotParsed(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("UpdateReview", mock.Anything, mock.AnythingOfType("*service.UpdateReviewInput")).
		Return(nil, domain.ErrDocumentNotParsed)

	body, _ := json.Marshal(map[string]string{
		"status": "approved",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/review", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.UpdateReview(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_UpdateReview_InvalidDocID(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/bad-id/review", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "bad-id"}}
	setAuthContext(c, tenantID, userID, "member")

	h.UpdateReview(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_UpdateReview_NoAuth(t *testing.T) {
	h, _ := newDocumentHandler()

	docID := uuid.New()

	body, _ := json.Marshal(map[string]string{"status": "approved"})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/review", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}

	h.UpdateReview(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- Delete ---

func TestDocumentHandler_Delete_Success(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("Delete", mock.Anything, tenantID, docID, userID, domain.UserRole("admin")).Return(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodDelete, "/api/v1/documents/"+docID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "admin")

	h.Delete(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_Delete_NotFound(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("Delete", mock.Anything, tenantID, docID, userID, domain.UserRole("admin")).Return(domain.ErrDocumentNotFound)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodDelete, "/api/v1/documents/"+docID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "admin")

	h.Delete(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDocumentHandler_Delete_InvalidID(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodDelete, "/api/v1/documents/bad-id", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "bad-id"}}
	setAuthContext(c, tenantID, userID, "admin")

	h.Delete(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_Delete_NoAuth(t *testing.T) {
	h, _ := newDocumentHandler()

	docID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodDelete, "/api/v1/documents/"+docID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}

	h.Delete(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- Create with name and tags ---

func TestDocumentHandler_Create_WithNameAndTags(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	fileID := uuid.New()
	collectionID := uuid.New()
	docID := uuid.New()

	expected := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		CollectionID:  collectionID,
		FileID:        fileID,
		Name:          "My Invoice",
		DocumentType:  "invoice",
		ParsingStatus: domain.ParsingStatusPending,
	}

	mockSvc.On("CreateAndParse", mock.Anything, mock.MatchedBy(func(input *service.CreateDocumentInput) bool {
		return input.Name == "My Invoice" &&
			input.Tags["vendor"] == "Acme" &&
			input.Tags["year"] == "2025"
	})).Return(expected, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"file_id":       fileID.String(),
		"collection_id": collectionID.String(),
		"document_type": "invoice",
		"name":          "My Invoice",
		"tags":          map[string]string{"vendor": "Acme", "year": "2025"},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "member")

	h.Create(c)

	assert.Equal(t, http.StatusCreated, w.Code)
	mockSvc.AssertExpectations(t)
}

// --- ListTags ---

func TestDocumentHandler_ListTags_Success(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	tags := []domain.DocumentTag{
		{ID: uuid.New(), DocumentID: docID, Key: "vendor", Value: "Acme", Source: "user"},
		{ID: uuid.New(), DocumentID: docID, Key: "seller_name", Value: "Acme Corp", Source: "auto"},
	}

	mockSvc.On("ListTags", mock.Anything, tenantID, docID, userID, domain.UserRole("member")).Return(tags, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/"+docID.String()+"/tags", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.ListTags(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_ListTags_InvalidID(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/bad-id/tags", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "bad-id"}}
	setAuthContext(c, tenantID, userID, "member")

	h.ListTags(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- AddTags ---

func TestDocumentHandler_AddTags_Success(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	resultTags := []domain.DocumentTag{
		{ID: uuid.New(), DocumentID: docID, Key: "vendor", Value: "Acme", Source: "user"},
	}

	mockSvc.On("AddTags", mock.Anything, tenantID, docID, userID, domain.UserRole("member"),
		map[string]string{"vendor": "Acme"}).Return(resultTags, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"tags": map[string]string{"vendor": "Acme"},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents/"+docID.String()+"/tags", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.AddTags(c)

	assert.Equal(t, http.StatusCreated, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_AddTags_MissingBody(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	body, _ := json.Marshal(map[string]interface{}{})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents/"+docID.String()+"/tags", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.AddTags(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- DeleteTag ---

func TestDocumentHandler_DeleteTag_Success(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()
	tagID := uuid.New()

	mockSvc.On("DeleteTag", mock.Anything, tenantID, docID, userID, domain.UserRole("member"), tagID).Return(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodDelete, "/api/v1/documents/"+docID.String()+"/tags/"+tagID.String(), http.NoBody)
	c.Params = gin.Params{
		{Key: "id", Value: docID.String()},
		{Key: "tagId", Value: tagID.String()},
	}
	setAuthContext(c, tenantID, userID, "member")

	h.DeleteTag(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_DeleteTag_InvalidTagID(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodDelete, "/api/v1/documents/"+docID.String()+"/tags/bad-id", http.NoBody)
	c.Params = gin.Params{
		{Key: "id", Value: docID.String()},
		{Key: "tagId", Value: "bad-id"},
	}
	setAuthContext(c, tenantID, userID, "member")

	h.DeleteTag(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- SearchByTag ---

func TestDocumentHandler_SearchByTag_Success(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	docs := []domain.Document{
		{ID: uuid.New(), TenantID: tenantID},
	}

	mockSvc.On("SearchByTag", mock.Anything, tenantID, "vendor", "Acme", 0, 20).
		Return(docs, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet,
		"/api/v1/documents/search/tags?key=vendor&value=Acme&offset=0&limit=20", http.NoBody)
	setAuthContext(c, tenantID, userID, "member")

	h.SearchByTag(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Meta)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_SearchByTag_MissingParams(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/search/tags?key=vendor", http.NoBody)
	setAuthContext(c, tenantID, userID, "member")

	h.SearchByTag(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- EditStructuredData ---

func TestDocumentHandler_EditStructuredData_Success(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	expected := &domain.Document{
		ID:               docID,
		TenantID:         tenantID,
		ParsingStatus:    domain.ParsingStatusCompleted,
		ReviewStatus:     domain.ReviewStatusPending,
		ValidationStatus: domain.ValidationStatusValid,
		StructuredData:   json.RawMessage(`{"invoice":{"invoice_number":"INV-001"}}`),
	}

	mockSvc.On("EditStructuredData", mock.Anything, mock.MatchedBy(func(input *service.EditStructuredDataInput) bool {
		return input.TenantID == tenantID &&
			input.DocumentID == docID &&
			input.UserID == userID &&
			input.Role == domain.UserRole("member")
	})).Return(expected, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"structured_data": map[string]interface{}{
			"invoice": map[string]string{"invoice_number": "INV-001"},
		},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/structured-data", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.EditStructuredData(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_EditStructuredData_MissingBody(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	body, _ := json.Marshal(map[string]interface{}{})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/structured-data", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.EditStructuredData(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_EditStructuredData_InvalidDocID(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/bad-id/structured-data", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "bad-id"}}
	setAuthContext(c, tenantID, userID, "member")

	h.EditStructuredData(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_EditStructuredData_NoAuth(t *testing.T) {
	h, _ := newDocumentHandler()

	docID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/structured-data", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}

	h.EditStructuredData(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDocumentHandler_EditStructuredData_NotParsed(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("EditStructuredData", mock.Anything, mock.AnythingOfType("*service.EditStructuredDataInput")).
		Return(nil, domain.ErrDocumentNotParsed)

	body, _ := json.Marshal(map[string]interface{}{
		"structured_data": map[string]interface{}{"invoice": map[string]string{}},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/structured-data", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.EditStructuredData(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_EditStructuredData_NotFound(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("EditStructuredData", mock.Anything, mock.AnythingOfType("*service.EditStructuredDataInput")).
		Return(nil, domain.ErrDocumentNotFound)

	body, _ := json.Marshal(map[string]interface{}{
		"structured_data": map[string]interface{}{"invoice": map[string]string{}},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/structured-data", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.EditStructuredData(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDocumentHandler_EditStructuredData_PermissionDenied(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("EditStructuredData", mock.Anything, mock.AnythingOfType("*service.EditStructuredDataInput")).
		Return(nil, domain.ErrCollectionPermDenied)

	body, _ := json.Marshal(map[string]interface{}{
		"structured_data": map[string]interface{}{"invoice": map[string]string{}},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/structured-data", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "viewer")

	h.EditStructuredData(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- EditStructuredData via PUT /documents/:id (shorthand route) ---

func TestDocumentHandler_EditStructuredData_ShorthandRoute_Success(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	expected := &domain.Document{
		ID:               docID,
		TenantID:         tenantID,
		ParsingStatus:    domain.ParsingStatusCompleted,
		ReviewStatus:     domain.ReviewStatusPending,
		ValidationStatus: domain.ValidationStatusValid,
		StructuredData:   json.RawMessage(`{"invoice":{"invoice_number":"INV-002"}}`),
	}

	mockSvc.On("EditStructuredData", mock.Anything, mock.MatchedBy(func(input *service.EditStructuredDataInput) bool {
		return input.TenantID == tenantID &&
			input.DocumentID == docID &&
			input.UserID == userID &&
			input.Role == domain.UserRole("member")
	})).Return(expected, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"structured_data": map[string]interface{}{
			"invoice": map[string]string{"invoice_number": "INV-002"},
		},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String(), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.EditStructuredData(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_EditStructuredData_ShorthandRoute_MissingBody(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	body, _ := json.Marshal(map[string]interface{}{})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String(), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.EditStructuredData(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_EditStructuredData_ShorthandRoute_NoAuth(t *testing.T) {
	h, _ := newDocumentHandler()

	docID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}

	h.EditStructuredData(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDocumentHandler_EditStructuredData_InvalidStructuredData(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("EditStructuredData", mock.Anything, mock.AnythingOfType("*service.EditStructuredDataInput")).
		Return(nil, domain.ErrInvalidStructuredData)

	body, _ := json.Marshal(map[string]interface{}{
		"structured_data": map[string]interface{}{"invoice": map[string]string{}},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/structured-data", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.EditStructuredData(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Validate ---

func TestDocumentHandler_Validate_Success(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("ValidateDocument", mock.Anything, tenantID, docID, userID, domain.UserRole("member")).
		Return(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents/"+docID.String()+"/validate", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.Validate(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_Validate_InvalidID(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents/not-a-uuid/validate", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "not-a-uuid"}}
	setAuthContext(c, tenantID, userID, "member")

	h.Validate(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_Validate_NoAuth(t *testing.T) {
	h, _ := newDocumentHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents/"+uuid.New().String()+"/validate", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}

	h.Validate(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDocumentHandler_Validate_NotFound(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("ValidateDocument", mock.Anything, tenantID, docID, userID, domain.UserRole("member")).
		Return(domain.ErrDocumentNotFound)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents/"+docID.String()+"/validate", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.Validate(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_Validate_NotParsed(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("ValidateDocument", mock.Anything, tenantID, docID, userID, domain.UserRole("member")).
		Return(domain.ErrDocumentNotParsed)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents/"+docID.String()+"/validate", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.Validate(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_Validate_PermDenied(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("ValidateDocument", mock.Anything, tenantID, docID, userID, domain.UserRole("viewer")).
		Return(domain.ErrCollectionPermDenied)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/documents/"+docID.String()+"/validate", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "viewer")

	h.Validate(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	mockSvc.AssertExpectations(t)
}

// --- GetValidation ---

func TestDocumentHandler_GetValidation_Success(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	expected := &validator.ValidationResponse{
		DocumentID:           docID,
		ValidationStatus:     domain.ValidationStatusValid,
		Summary:              validator.ValidationSummary{Total: 3, Passed: 3},
		ReconciliationStatus: domain.ReconciliationStatusValid,
	}

	mockSvc.On("GetValidation", mock.Anything, tenantID, docID, userID, domain.UserRole("member")).
		Return(expected, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/"+docID.String()+"/validation", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.GetValidation(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Data)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_GetValidation_InvalidID(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/not-a-uuid/validation", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "not-a-uuid"}}
	setAuthContext(c, tenantID, userID, "member")

	h.GetValidation(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_GetValidation_NoAuth(t *testing.T) {
	h, _ := newDocumentHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/"+uuid.New().String()+"/validation", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}

	h.GetValidation(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDocumentHandler_GetValidation_NotFound(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("GetValidation", mock.Anything, tenantID, docID, userID, domain.UserRole("member")).
		Return(nil, domain.ErrDocumentNotFound)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/"+docID.String()+"/validation", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.GetValidation(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_GetValidation_PermDenied(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("GetValidation", mock.Anything, tenantID, docID, userID, domain.UserRole("viewer")).
		Return(nil, domain.ErrCollectionPermDenied)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/"+docID.String()+"/validation", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "viewer")

	h.GetValidation(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	mockSvc.AssertExpectations(t)
}

// --- ListAudit ---

func TestDocumentHandler_ListAudit_Success(t *testing.T) {
	mockSvc := new(mocks.MockDocumentService)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	h := handler.NewDocumentHandler(mockSvc, auditRepo, nil, nil)

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	entries := []domain.DocumentAuditEntry{
		{ID: uuid.New(), TenantID: tenantID, DocumentID: docID, Action: "document.created"},
	}

	auditRepo.On("ListByDocument", mock.Anything, tenantID, docID, 0, 20).
		Return(entries, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/"+docID.String()+"/audit?offset=0&limit=20", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.ListAudit(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Meta)
	auditRepo.AssertExpectations(t)
}

func TestDocumentHandler_ListAudit_InvalidID(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/bad-id/audit", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "bad-id"}}
	setAuthContext(c, tenantID, userID, "member")

	h.ListAudit(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- AssignDocument ---

func TestDocumentHandler_AssignDocument_Success(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()
	assigneeID := uuid.New()

	expected := &domain.Document{
		ID:         docID,
		TenantID:   tenantID,
		AssignedTo: &assigneeID,
	}

	mockSvc.On("AssignDocument", mock.Anything, mock.MatchedBy(func(input *service.AssignDocumentInput) bool {
		return input.TenantID == tenantID &&
			input.DocumentID == docID &&
			input.CallerID == userID &&
			input.CallerRole == domain.UserRole("admin") &&
			input.AssigneeID != nil && *input.AssigneeID == assigneeID
	})).Return(expected, nil)

	body, _ := json.Marshal(map[string]string{
		"assignee_id": assigneeID.String(),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/assign", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "admin")

	h.AssignDocument(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_AssignDocument_Unassign(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	expected := &domain.Document{
		ID:         docID,
		TenantID:   tenantID,
		AssignedTo: nil,
	}

	mockSvc.On("AssignDocument", mock.Anything, mock.MatchedBy(func(input *service.AssignDocumentInput) bool {
		return input.AssigneeID == nil
	})).Return(expected, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"assignee_id": nil,
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/assign", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "admin")

	h.AssignDocument(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_AssignDocument_InvalidID(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	body, _ := json.Marshal(map[string]string{"assignee_id": uuid.New().String()})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/bad-id/assign", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "bad-id"}}
	setAuthContext(c, tenantID, userID, "admin")

	h.AssignDocument(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_AssignDocument_NoAuth(t *testing.T) {
	h, _ := newDocumentHandler()

	docID := uuid.New()
	body, _ := json.Marshal(map[string]string{"assignee_id": uuid.New().String()})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/assign", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}

	h.AssignDocument(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDocumentHandler_AssignDocument_NotParsed(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("AssignDocument", mock.Anything, mock.AnythingOfType("*service.AssignDocumentInput")).
		Return(nil, domain.ErrDocumentNotParsed)

	body, _ := json.Marshal(map[string]string{"assignee_id": uuid.New().String()})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/assign", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "admin")

	h.AssignDocument(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_AssignDocument_AssigneeCannotReview(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("AssignDocument", mock.Anything, mock.AnythingOfType("*service.AssignDocumentInput")).
		Return(nil, domain.ErrAssigneeCannotReview)

	body, _ := json.Marshal(map[string]string{"assignee_id": uuid.New().String()})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/documents/"+docID.String()+"/assign", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "admin")

	h.AssignDocument(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- ReviewQueue ---

func TestDocumentHandler_ReviewQueue_Success(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	docs := []domain.Document{
		{ID: uuid.New(), TenantID: tenantID, ParsingStatus: domain.ParsingStatusCompleted, AssignedTo: &userID},
	}

	mockSvc.On("ListReviewQueue", mock.Anything, tenantID, userID, 0, 20).Return(docs, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/review-queue?offset=0&limit=20", http.NoBody)
	setAuthContext(c, tenantID, userID, "member")

	h.ReviewQueue(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Meta)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_ReviewQueue_Empty(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	mockSvc.On("ListReviewQueue", mock.Anything, tenantID, userID, 0, 20).Return([]domain.Document{}, 0, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/review-queue?offset=0&limit=20", http.NoBody)
	setAuthContext(c, tenantID, userID, "member")

	h.ReviewQueue(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_ReviewQueue_NoAuth(t *testing.T) {
	h, _ := newDocumentHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/review-queue", http.NoBody)

	h.ReviewQueue(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- ListSyncEvents ---

func TestDocumentHandler_ListSyncEvents_Success(t *testing.T) {
	mockSvc := new(mocks.MockDocumentService)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	syncRepo := new(mocks.MockSyncRepo)
	h := handler.NewDocumentHandler(mockSvc, auditRepo, syncRepo, nil)

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	events := []domain.SyncEvent{
		{ID: uuid.New(), TenantID: tenantID, DocumentID: &docID, Direction: "outbound", Status: "success"},
	}
	syncRepo.On("ListSyncEventsByDocument", mock.Anything, tenantID, docID, 0, 20).Return(events, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/"+docID.String()+"/sync-events", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "admin")

	h.ListSyncEvents(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Meta)
	assert.Equal(t, 1, resp.Meta.Total)
	syncRepo.AssertExpectations(t)
}

func TestDocumentHandler_ListSyncEvents_InvalidID(t *testing.T) {
	mockSvc := new(mocks.MockDocumentService)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	syncRepo := new(mocks.MockSyncRepo)
	h := handler.NewDocumentHandler(mockSvc, auditRepo, syncRepo, nil)

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/bad-id/sync-events", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "bad-id"}}
	setAuthContext(c, tenantID, userID, "admin")

	h.ListSyncEvents(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_ListSyncEvents_NilRepo(t *testing.T) {
	mockSvc := new(mocks.MockDocumentService)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	h := handler.NewDocumentHandler(mockSvc, auditRepo, nil, nil)

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/"+docID.String()+"/sync-events", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "admin")

	h.ListSyncEvents(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- VoucherPreview ---

func TestDocumentHandler_VoucherPreview_Success(t *testing.T) {
	mockSvc := new(mocks.MockDocumentService)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	voucherBuilder := new(mocks.MockVoucherBuilder)
	h := handler.NewDocumentHandler(mockSvc, auditRepo, nil, voucherBuilder)

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	doc := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusCompleted,
	}
	mockSvc.On("GetByID", mock.Anything, tenantID, docID, userID, domain.UserRole("admin")).Return(doc, nil)

	voucherDef := &domain.VoucherDef{
		DocumentID:      docID,
		VoucherType:     "Purchase",
		MatchConfidence: map[string]string{"party_ledger": "high"},
	}
	voucherBuilder.On("Build", mock.Anything, tenantID, doc).Return(voucherDef, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/"+docID.String()+"/voucher-preview", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "admin")

	h.VoucherPreview(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
	voucherBuilder.AssertExpectations(t)
}

func TestDocumentHandler_VoucherPreview_DocumentNotParsed(t *testing.T) {
	mockSvc := new(mocks.MockDocumentService)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	voucherBuilder := new(mocks.MockVoucherBuilder)
	h := handler.NewDocumentHandler(mockSvc, auditRepo, nil, voucherBuilder)

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	doc := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusPending,
	}
	mockSvc.On("GetByID", mock.Anything, tenantID, docID, userID, domain.UserRole("admin")).Return(doc, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/"+docID.String()+"/voucher-preview", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "admin")

	h.VoucherPreview(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_VoucherPreview_NotFound(t *testing.T) {
	mockSvc := new(mocks.MockDocumentService)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	voucherBuilder := new(mocks.MockVoucherBuilder)
	h := handler.NewDocumentHandler(mockSvc, auditRepo, nil, voucherBuilder)

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("GetByID", mock.Anything, tenantID, docID, userID, domain.UserRole("admin")).Return(nil, domain.ErrDocumentNotFound)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/"+docID.String()+"/voucher-preview", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "admin")

	h.VoucherPreview(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDocumentHandler_VoucherPreview_NilBuilder(t *testing.T) {
	mockSvc := new(mocks.MockDocumentService)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	h := handler.NewDocumentHandler(mockSvc, auditRepo, nil, nil)

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/"+docID.String()+"/voucher-preview", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "admin")

	h.VoucherPreview(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDocumentHandler_VoucherPreview_PermissionDenied(t *testing.T) {
	mockSvc := new(mocks.MockDocumentService)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	voucherBuilder := new(mocks.MockVoucherBuilder)
	h := handler.NewDocumentHandler(mockSvc, auditRepo, nil, voucherBuilder)

	tenantID := uuid.New()
	userID := uuid.New()
	docID := uuid.New()

	mockSvc.On("GetByID", mock.Anything, tenantID, docID, userID, domain.UserRole("viewer")).Return(nil, domain.ErrCollectionPermDenied)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents/"+docID.String()+"/voucher-preview", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: docID.String()}}
	setAuthContext(c, tenantID, userID, "viewer")

	h.VoucherPreview(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}
