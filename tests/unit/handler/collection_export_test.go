package handler_test

import (
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"satvos/internal/csvexport"
	"satvos/internal/domain"
	"satvos/internal/handler"
	"satvos/internal/validator/invoice"
	"satvos/mocks"
)

func newExportHandler() (*handler.CollectionHandler, *mocks.MockCollectionService, *mocks.MockDocumentService) {
	collSvc := new(mocks.MockCollectionService)
	docSvc := new(mocks.MockDocumentService)
	h := handler.NewCollectionHandler(collSvc, docSvc)
	return h, collSvc, docSvc
}

func TestExportCSV_Success(t *testing.T) {
	h, collSvc, docSvc := newExportHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	collection := &domain.Collection{
		ID:       collectionID,
		TenantID: tenantID,
		Name:     "Q3 Purchase Invoices",
	}

	inv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{InvoiceNumber: "INV-001", InvoiceDate: "2025-01-15"},
		Seller:  invoice.Party{Name: "Seller Corp", GSTIN: "29ABCDE1234F1Z5"},
		Buyer:   invoice.Party{Name: "Buyer Inc"},
		Totals:  invoice.Totals{Total: 1000},
	}
	data, _ := json.Marshal(inv)
	parsedAt := time.Now()

	docs := []domain.Document{
		{
			ID:             uuid.New(),
			Name:           "Invoice 1",
			ParsingStatus:  domain.ParsingStatusCompleted,
			StructuredData: data,
			ParsedAt:       &parsedAt,
			CreatedAt:      time.Now(),
		},
	}

	collSvc.On("GetByID", mock.Anything, tenantID, collectionID, userID, domain.UserRole("member")).
		Return(collection, nil)
	docSvc.On("ListByCollection", mock.Anything, tenantID, collectionID, userID, domain.UserRole("member"), (*uuid.UUID)(nil), 0, 200).
		Return(docs, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/collections/"+collectionID.String()+"/export/csv", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: collectionID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.ExportCSV(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/csv; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "Q3_Purchase_Invoices_")
	assert.Contains(t, w.Header().Get("Content-Disposition"), ".csv")

	// Verify BOM
	body := w.Body.Bytes()
	require.True(t, len(body) >= 3)
	assert.Equal(t, csvexport.BOM, body[:3])

	// Parse CSV (skip BOM)
	r := csv.NewReader(strings.NewReader(string(body[3:])))
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2) // header + 1 data row

	// Header row
	assert.Equal(t, "Document Name", records[0][0])
	assert.Len(t, records[0], 48)

	// Data row
	assert.Equal(t, "Invoice 1", records[1][0])
	assert.Equal(t, "INV-001", records[1][5])
	assert.Equal(t, "Seller Corp", records[1][13])
	assert.Equal(t, "1000.00", records[1][26])

	collSvc.AssertExpectations(t)
	docSvc.AssertExpectations(t)
}

func TestExportCSV_CollectionNotFound(t *testing.T) {
	h, collSvc, _ := newExportHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	collSvc.On("GetByID", mock.Anything, tenantID, collectionID, userID, domain.UserRole("member")).
		Return(nil, domain.ErrCollectionNotFound)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/collections/"+collectionID.String()+"/export/csv", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: collectionID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.ExportCSV(c)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.False(t, resp.Success)
	collSvc.AssertExpectations(t)
}

func TestExportCSV_PermissionDenied(t *testing.T) {
	h, collSvc, _ := newExportHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	collSvc.On("GetByID", mock.Anything, tenantID, collectionID, userID, domain.UserRole("viewer")).
		Return(nil, domain.ErrForbidden)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/collections/"+collectionID.String()+"/export/csv", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: collectionID.String()}}
	setAuthContext(c, tenantID, userID, "viewer")

	h.ExportCSV(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	collSvc.AssertExpectations(t)
}

func TestExportCSV_EmptyCollection(t *testing.T) {
	h, collSvc, docSvc := newExportHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	collection := &domain.Collection{
		ID:       collectionID,
		TenantID: tenantID,
		Name:     "Empty Collection",
	}

	collSvc.On("GetByID", mock.Anything, tenantID, collectionID, userID, domain.UserRole("member")).
		Return(collection, nil)
	docSvc.On("ListByCollection", mock.Anything, tenantID, collectionID, userID, domain.UserRole("member"), (*uuid.UUID)(nil), 0, 200).
		Return([]domain.Document{}, 0, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/collections/"+collectionID.String()+"/export/csv", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: collectionID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.ExportCSV(c)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify BOM + header only
	body := w.Body.Bytes()
	require.True(t, len(body) >= 3)

	r := csv.NewReader(strings.NewReader(string(body[3:])))
	records, err := r.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 1) // header only

	collSvc.AssertExpectations(t)
	docSvc.AssertExpectations(t)
}

func TestExportCSV_InvalidID(t *testing.T) {
	h, _, _ := newExportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/collections/not-a-uuid/export/csv", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "not-a-uuid"}}
	setAuthContext(c, tenantID, userID, "member")

	h.ExportCSV(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Tally Export Tests ---

func TestExportTally_Success(t *testing.T) {
	h, collSvc, docSvc := newExportHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	collection := &domain.Collection{
		ID:       collectionID,
		TenantID: tenantID,
		Name:     "Q4 Invoices",
	}

	inv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{InvoiceNumber: "INV-T01", InvoiceDate: "2025-12-15"},
		Seller:  invoice.Party{Name: "Tally Seller", GSTIN: "07AABCU9603R1ZM"},
		LineItems: []invoice.LineItem{
			{Description: "Widget", Quantity: 2, UnitPrice: 100, TaxableAmount: 200, CGSTRate: 9, CGSTAmount: 18, SGSTRate: 9, SGSTAmount: 18},
		},
		Totals: invoice.Totals{Total: 236},
	}
	data, _ := json.Marshal(inv)

	docs := []domain.Document{
		{
			ID:             uuid.New(),
			TenantID:       tenantID,
			ParsingStatus:  domain.ParsingStatusCompleted,
			StructuredData: data,
			CreatedAt:      time.Now(),
		},
	}

	collSvc.On("GetByID", mock.Anything, tenantID, collectionID, userID, domain.UserRole("member")).
		Return(collection, nil)
	docSvc.On("ListByCollection", mock.Anything, tenantID, collectionID, userID, domain.UserRole("member"), (*uuid.UUID)(nil), 0, 200).
		Return(docs, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/collections/"+collectionID.String()+"/export/tally?company_name=TestCo", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: collectionID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.ExportTally(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/xml; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "Q4_Invoices_")
	assert.Contains(t, w.Header().Get("Content-Disposition"), ".xml")

	body := w.Body.String()
	assert.Contains(t, body, "<SVCURRENTCOMPANY>TestCo</SVCURRENTCOMPANY>")
	assert.Contains(t, body, "Tally Seller")
	assert.Contains(t, body, "INV-T01")

	// Verify well-formed XML
	decoder := xml.NewDecoder(strings.NewReader(body))
	for {
		_, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("XML parse error: %v", err)
		}
	}

	collSvc.AssertExpectations(t)
	docSvc.AssertExpectations(t)
}

func TestExportTally_DefaultCompanyName(t *testing.T) {
	h, collSvc, docSvc := newExportHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	collection := &domain.Collection{
		ID:       collectionID,
		TenantID: tenantID,
		Name:     "My Collection",
	}

	collSvc.On("GetByID", mock.Anything, tenantID, collectionID, userID, domain.UserRole("member")).
		Return(collection, nil)
	docSvc.On("ListByCollection", mock.Anything, tenantID, collectionID, userID, domain.UserRole("member"), (*uuid.UUID)(nil), 0, 200).
		Return([]domain.Document{}, 0, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/collections/"+collectionID.String()+"/export/tally", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: collectionID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.ExportTally(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "<SVCURRENTCOMPANY>My Collection</SVCURRENTCOMPANY>")

	collSvc.AssertExpectations(t)
	docSvc.AssertExpectations(t)
}

func TestExportTally_CollectionNotFound(t *testing.T) {
	h, collSvc, _ := newExportHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	collSvc.On("GetByID", mock.Anything, tenantID, collectionID, userID, domain.UserRole("member")).
		Return(nil, domain.ErrCollectionNotFound)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/collections/"+collectionID.String()+"/export/tally", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: collectionID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.ExportTally(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	collSvc.AssertExpectations(t)
}

func TestExportTally_PermissionDenied(t *testing.T) {
	h, collSvc, _ := newExportHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	collSvc.On("GetByID", mock.Anything, tenantID, collectionID, userID, domain.UserRole("viewer")).
		Return(nil, domain.ErrForbidden)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/collections/"+collectionID.String()+"/export/tally", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: collectionID.String()}}
	setAuthContext(c, tenantID, userID, "viewer")

	h.ExportTally(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	collSvc.AssertExpectations(t)
}

func TestExportTally_EmptyCollection(t *testing.T) {
	h, collSvc, docSvc := newExportHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	collection := &domain.Collection{
		ID:       collectionID,
		TenantID: tenantID,
		Name:     "Empty",
	}

	collSvc.On("GetByID", mock.Anything, tenantID, collectionID, userID, domain.UserRole("member")).
		Return(collection, nil)
	docSvc.On("ListByCollection", mock.Anything, tenantID, collectionID, userID, domain.UserRole("member"), (*uuid.UUID)(nil), 0, 200).
		Return([]domain.Document{}, 0, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/collections/"+collectionID.String()+"/export/tally", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: collectionID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.ExportTally(c)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "<SVCURRENTCOMPANY>Empty</SVCURRENTCOMPANY>")
	assert.NotContains(t, body, "<VOUCHER")

	// Still valid XML
	decoder := xml.NewDecoder(strings.NewReader(body))
	for {
		_, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("XML parse error: %v", err)
		}
	}

	collSvc.AssertExpectations(t)
	docSvc.AssertExpectations(t)
}
