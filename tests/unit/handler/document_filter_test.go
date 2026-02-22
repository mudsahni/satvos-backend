package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"satvos/internal/domain"
	"satvos/internal/handler"
	"satvos/internal/port"
)

// --- List with status filters ---

func TestDocumentHandler_List_WithStatusFilters(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	docs := []domain.Document{
		{ID: uuid.New(), TenantID: tenantID, ParsingStatus: domain.ParsingStatusCompleted},
	}

	mockSvc.On("ListByTenant", mock.Anything, tenantID, userID, domain.UserRole("member"),
		mock.MatchedBy(func(f *port.DocumentListFilters) bool {
			return f.ParsingStatus != nil && *f.ParsingStatus == domain.ParsingStatusCompleted &&
				f.ReviewStatus != nil && *f.ReviewStatus == domain.ReviewStatusPending
		}), 0, 20).Return(docs, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet,
		"/api/v1/documents?parsing_status=completed&review_status=pending", http.NoBody)
	setAuthContext(c, tenantID, userID, "member")

	h.List(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_List_WithTagFilters(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	docs := []domain.Document{
		{ID: uuid.New(), TenantID: tenantID},
	}

	mockSvc.On("ListByTenant", mock.Anything, tenantID, userID, domain.UserRole("admin"),
		mock.MatchedBy(func(f *port.DocumentListFilters) bool {
			return f.SellerName != nil && *f.SellerName == "Acme Corp" &&
				f.BuyerName != nil && *f.BuyerName == "My Company"
		}), 0, 20).Return(docs, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet,
		"/api/v1/documents?seller_name=Acme+Corp&buyer_name=My+Company", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.List(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_List_WithAllFilters(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	assigneeID := uuid.New()
	collectionID := uuid.New()

	docs := []domain.Document{{ID: uuid.New()}}

	mockSvc.On("ListByCollection", mock.Anything, tenantID, collectionID, userID, domain.UserRole("admin"),
		mock.MatchedBy(func(f *port.DocumentListFilters) bool {
			return f.AssignedTo != nil && *f.AssignedTo == assigneeID &&
				f.SellerName != nil && *f.SellerName == "Seller" &&
				f.ParsingStatus != nil && *f.ParsingStatus == domain.ParsingStatusCompleted &&
				f.ReviewStatus != nil && *f.ReviewStatus == domain.ReviewStatusApproved &&
				f.ValidationStatus != nil && *f.ValidationStatus == domain.ValidationStatusValid &&
				f.ReconciliationStatus != nil && *f.ReconciliationStatus == domain.ReconciliationStatusValid
		}), 0, 20).Return(docs, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	url := "/api/v1/documents?collection_id=" + collectionID.String() +
		"&assigned_to=" + assigneeID.String() +
		"&seller_name=Seller" +
		"&parsing_status=completed&review_status=approved" +
		"&validation_status=valid&reconciliation_status=valid"
	c.Request, _ = http.NewRequest(http.MethodGet, url, http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.List(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_List_InvalidParsingStatus(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet,
		"/api/v1/documents?parsing_status=bogus", http.NoBody)
	setAuthContext(c, tenantID, userID, "member")

	h.List(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.False(t, resp.Success)
}

func TestDocumentHandler_List_InvalidReviewStatus(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet,
		"/api/v1/documents?review_status=maybe", http.NoBody)
	setAuthContext(c, tenantID, userID, "member")

	h.List(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_List_InvalidValidationStatus(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet,
		"/api/v1/documents?validation_status=unknown", http.NoBody)
	setAuthContext(c, tenantID, userID, "member")

	h.List(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_List_InvalidReconciliationStatus(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet,
		"/api/v1/documents?reconciliation_status=done", http.NoBody)
	setAuthContext(c, tenantID, userID, "member")

	h.List(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDocumentHandler_List_ValidParsingStatuses(t *testing.T) {
	for _, status := range []string{"pending", "processing", "completed", "failed", "queued"} {
		t.Run(status, func(t *testing.T) {
			h, mockSvc := newDocumentHandler()
			tenantID := uuid.New()
			userID := uuid.New()

			mockSvc.On("ListByTenant", mock.Anything, tenantID, userID, domain.UserRole("member"),
				mock.Anything, 0, 20).Return([]domain.Document{}, 0, nil)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest(http.MethodGet,
				"/api/v1/documents?parsing_status="+status, http.NoBody)
			setAuthContext(c, tenantID, userID, "member")

			h.List(c)

			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

// --- TagFacets handler ---

func TestDocumentHandler_TagFacets_Success(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	facets := map[string][]port.TagFacet{
		"seller_name": {
			{Value: "Acme Corp", Count: 42},
			{Value: "XYZ Industries", Count: 15},
		},
		"buyer_name": {
			{Value: "My Company", Count: 200},
		},
	}

	mockSvc.On("GetTagFacets", mock.Anything, tenantID, userID, domain.UserRole("admin"),
		[]string{"seller_name", "buyer_name"}).Return(facets, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet,
		"/api/v1/documents/tags/facets?keys=seller_name,buyer_name", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.TagFacets(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Data)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_TagFacets_MissingKeys(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet,
		"/api/v1/documents/tags/facets", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.TagFacets(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.False(t, resp.Success)
}

func TestDocumentHandler_TagFacets_InvalidKey(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet,
		"/api/v1/documents/tags/facets?keys=seller_name,secret_field", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.TagFacets(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.False(t, resp.Success)
}

func TestDocumentHandler_TagFacets_AllAllowedKeys(t *testing.T) {
	allowedKeys := []string{"seller_name", "buyer_name", "seller_gstin", "buyer_gstin", "invoice_type", "place_of_supply"}
	for _, key := range allowedKeys {
		t.Run(key, func(t *testing.T) {
			h, mockSvc := newDocumentHandler()
			tenantID := uuid.New()
			userID := uuid.New()

			facets := map[string][]port.TagFacet{key: {}}
			mockSvc.On("GetTagFacets", mock.Anything, tenantID, userID, domain.UserRole("admin"),
				[]string{key}).Return(facets, nil)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest(http.MethodGet,
				"/api/v1/documents/tags/facets?keys="+key, http.NoBody)
			setAuthContext(c, tenantID, userID, "admin")

			h.TagFacets(c)

			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestDocumentHandler_TagFacets_NoAuth(t *testing.T) {
	h, _ := newDocumentHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet,
		"/api/v1/documents/tags/facets?keys=seller_name", http.NoBody)

	h.TagFacets(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDocumentHandler_TagFacets_ServiceError(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	mockSvc.On("GetTagFacets", mock.Anything, tenantID, userID, domain.UserRole("member"),
		[]string{"seller_name"}).Return(nil, domain.ErrForbidden)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet,
		"/api/v1/documents/tags/facets?keys=seller_name", http.NoBody)
	setAuthContext(c, tenantID, userID, "member")

	h.TagFacets(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	mockSvc.AssertExpectations(t)
}

// --- List with no filters (empty struct) ---

func TestDocumentHandler_List_NoFilters(t *testing.T) {
	h, mockSvc := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	mockSvc.On("ListByTenant", mock.Anything, tenantID, userID, domain.UserRole("admin"),
		mock.MatchedBy(func(f *port.DocumentListFilters) bool {
			return f.AssignedTo == nil &&
				f.SellerName == nil &&
				f.BuyerName == nil &&
				f.ParsingStatus == nil &&
				f.ReviewStatus == nil &&
				f.ValidationStatus == nil &&
				f.ReconciliationStatus == nil
		}), 0, 20).Return([]domain.Document{}, 0, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/documents", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.List(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestDocumentHandler_List_InvalidAssignedTo(t *testing.T) {
	h, _ := newDocumentHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet,
		"/api/v1/documents?assigned_to=not-a-uuid", http.NoBody)
	setAuthContext(c, tenantID, userID, "member")

	h.List(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
