package parser_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"satvos/internal/parser"
	"satvos/internal/port"
	"satvos/internal/validator/invoice"
	"satvos/mocks"
)

func makeParseOutput(inv *invoice.GSTInvoice, model string) *port.ParseOutput {
	data, _ := json.Marshal(inv)
	return &port.ParseOutput{
		StructuredData:   data,
		ConfidenceScores: json.RawMessage("{}"),
		ModelUsed:        model,
		PromptUsed:       "test prompt",
	}
}

func TestMergeParser_BothSucceed_Agreement(t *testing.T) {
	primary := new(mocks.MockDocumentParser)
	secondary := new(mocks.MockDocumentParser)
	mp := parser.NewMergeParser(primary, secondary)

	inv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{InvoiceNumber: "INV-001", InvoiceDate: "15/01/2025"},
		Seller:  invoice.Party{Name: "Seller Corp", GSTIN: "29ABCDE1234F1Z5"},
		Totals:  invoice.Totals{Total: 1000},
	}

	input := port.ParseInput{FileBytes: []byte("test"), ContentType: "application/pdf", DocumentType: "invoice"}

	primary.On("Parse", mock.Anything, input).Return(makeParseOutput(&inv, "claude"), nil)
	secondary.On("Parse", mock.Anything, input).Return(makeParseOutput(&inv, "gemini"), nil)

	result, err := mp.Parse(context.Background(), input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "claude", result.ModelUsed)
	assert.Equal(t, "gemini", result.SecondaryModel)
	assert.NotNil(t, result.FieldProvenance)
	assert.Equal(t, "agree", result.FieldProvenance["invoice.invoice_number"])
	assert.Equal(t, "agree", result.FieldProvenance["seller.gstin"])
	assert.Equal(t, "agree", result.FieldProvenance["totals.total"])
}

func TestMergeParser_BothSucceed_Disagreement(t *testing.T) {
	primary := new(mocks.MockDocumentParser)
	secondary := new(mocks.MockDocumentParser)
	mp := parser.NewMergeParser(primary, secondary)

	pInv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{InvoiceNumber: "INV-001"},
		Seller:  invoice.Party{Name: "Primary Seller"},
		Totals:  invoice.Totals{Total: 1000},
	}
	sInv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{InvoiceNumber: "INV-002"},
		Seller:  invoice.Party{Name: "Secondary Seller"},
		Totals:  invoice.Totals{Total: 2000},
	}

	input := port.ParseInput{FileBytes: []byte("test"), ContentType: "application/pdf", DocumentType: "invoice"}

	primary.On("Parse", mock.Anything, input).Return(makeParseOutput(&pInv, "claude"), nil)
	secondary.On("Parse", mock.Anything, input).Return(makeParseOutput(&sInv, "gemini"), nil)

	result, err := mp.Parse(context.Background(), input)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	// On disagreement, primary value should be kept
	var mergedData invoice.GSTInvoice
	err = json.Unmarshal(result.StructuredData, &mergedData)
	assert.NoError(t, err)
	assert.Equal(t, "INV-001", mergedData.Invoice.InvoiceNumber) // primary kept

	assert.Equal(t, "disagreement", result.FieldProvenance["invoice.invoice_number"])
}

func TestMergeParser_BothSucceed_OneEmpty(t *testing.T) {
	primary := new(mocks.MockDocumentParser)
	secondary := new(mocks.MockDocumentParser)
	mp := parser.NewMergeParser(primary, secondary)

	pInv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{InvoiceNumber: "INV-001"},
		Seller:  invoice.Party{Name: ""}, // primary has empty name
	}
	sInv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{InvoiceNumber: "INV-001"},
		Seller:  invoice.Party{Name: "Secondary Seller"}, // secondary has it
	}

	input := port.ParseInput{FileBytes: []byte("test"), ContentType: "application/pdf", DocumentType: "invoice"}

	primary.On("Parse", mock.Anything, input).Return(makeParseOutput(&pInv, "claude"), nil)
	secondary.On("Parse", mock.Anything, input).Return(makeParseOutput(&sInv, "gemini"), nil)

	result, err := mp.Parse(context.Background(), input)

	assert.NoError(t, err)

	var mergedData invoice.GSTInvoice
	err = json.Unmarshal(result.StructuredData, &mergedData)
	assert.NoError(t, err)
	assert.Equal(t, "Secondary Seller", mergedData.Seller.Name) // filled from secondary
	assert.Equal(t, "secondary", result.FieldProvenance["seller.name"])
}

func TestMergeParser_BothSucceed_GSTINFormatHeuristic(t *testing.T) {
	primary := new(mocks.MockDocumentParser)
	secondary := new(mocks.MockDocumentParser)
	mp := parser.NewMergeParser(primary, secondary)

	pInv := invoice.GSTInvoice{
		Seller: invoice.Party{GSTIN: "invalid-gstin"},
	}
	sInv := invoice.GSTInvoice{
		Seller: invoice.Party{GSTIN: "29ABCDE1234F1Z5"}, // valid GSTIN format
	}

	input := port.ParseInput{FileBytes: []byte("test"), ContentType: "application/pdf", DocumentType: "invoice"}

	primary.On("Parse", mock.Anything, input).Return(makeParseOutput(&pInv, "claude"), nil)
	secondary.On("Parse", mock.Anything, input).Return(makeParseOutput(&sInv, "gemini"), nil)

	result, err := mp.Parse(context.Background(), input)

	assert.NoError(t, err)

	var mergedData invoice.GSTInvoice
	err = json.Unmarshal(result.StructuredData, &mergedData)
	assert.NoError(t, err)
	// Secondary should be preferred because it matches GSTIN format
	assert.Equal(t, "29ABCDE1234F1Z5", mergedData.Seller.GSTIN)
	assert.Equal(t, "secondary_format", result.FieldProvenance["seller.gstin"])
}

func TestMergeParser_PrimaryFails(t *testing.T) {
	primary := new(mocks.MockDocumentParser)
	secondary := new(mocks.MockDocumentParser)
	mp := parser.NewMergeParser(primary, secondary)

	sInv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{InvoiceNumber: "INV-001"},
	}

	input := port.ParseInput{FileBytes: []byte("test"), ContentType: "application/pdf", DocumentType: "invoice"}

	primary.On("Parse", mock.Anything, input).Return(nil, errors.New("primary API error"))
	secondary.On("Parse", mock.Anything, input).Return(makeParseOutput(&sInv, "gemini"), nil)

	result, err := mp.Parse(context.Background(), input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "secondary_only", result.FieldProvenance["_source"])
}

func TestMergeParser_SecondaryFails(t *testing.T) {
	primary := new(mocks.MockDocumentParser)
	secondary := new(mocks.MockDocumentParser)
	mp := parser.NewMergeParser(primary, secondary)

	pInv := invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{InvoiceNumber: "INV-001"},
	}

	input := port.ParseInput{FileBytes: []byte("test"), ContentType: "application/pdf", DocumentType: "invoice"}

	primary.On("Parse", mock.Anything, input).Return(makeParseOutput(&pInv, "claude"), nil)
	secondary.On("Parse", mock.Anything, input).Return(nil, errors.New("secondary API error"))

	result, err := mp.Parse(context.Background(), input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "primary_only", result.FieldProvenance["_source"])
}

func TestMergeParser_BothFail(t *testing.T) {
	primary := new(mocks.MockDocumentParser)
	secondary := new(mocks.MockDocumentParser)
	mp := parser.NewMergeParser(primary, secondary)

	input := port.ParseInput{FileBytes: []byte("test"), ContentType: "application/pdf", DocumentType: "invoice"}

	primary.On("Parse", mock.Anything, input).Return(nil, errors.New("primary error"))
	secondary.On("Parse", mock.Anything, input).Return(nil, errors.New("secondary error"))

	result, err := mp.Parse(context.Background(), input)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "both parsers failed")
}

func TestMergeParser_LineItems_SecondaryHasMore(t *testing.T) {
	primary := new(mocks.MockDocumentParser)
	secondary := new(mocks.MockDocumentParser)
	mp := parser.NewMergeParser(primary, secondary)

	pInv := invoice.GSTInvoice{
		LineItems: []invoice.LineItem{
			{Description: "Item 1", Total: 100},
		},
	}
	sInv := invoice.GSTInvoice{
		LineItems: []invoice.LineItem{
			{Description: "Item 1", Total: 100},
			{Description: "Item 2", Total: 200},
		},
	}

	input := port.ParseInput{FileBytes: []byte("test"), ContentType: "application/pdf", DocumentType: "invoice"}

	primary.On("Parse", mock.Anything, input).Return(makeParseOutput(&pInv, "claude"), nil)
	secondary.On("Parse", mock.Anything, input).Return(makeParseOutput(&sInv, "gemini"), nil)

	result, err := mp.Parse(context.Background(), input)

	assert.NoError(t, err)

	var mergedData invoice.GSTInvoice
	err = json.Unmarshal(result.StructuredData, &mergedData)
	assert.NoError(t, err)
	assert.Len(t, mergedData.LineItems, 2)
	assert.Equal(t, "secondary", result.FieldProvenance["line_items"])
}

func TestMergeParser_LineItems_PrimaryHasMoreOrEqual(t *testing.T) {
	primary := new(mocks.MockDocumentParser)
	secondary := new(mocks.MockDocumentParser)
	mp := parser.NewMergeParser(primary, secondary)

	pInv := invoice.GSTInvoice{
		LineItems: []invoice.LineItem{
			{Description: "Item 1", Total: 100},
			{Description: "Item 2", Total: 200},
		},
	}
	sInv := invoice.GSTInvoice{
		LineItems: []invoice.LineItem{
			{Description: "Item 1", Total: 100},
		},
	}

	input := port.ParseInput{FileBytes: []byte("test"), ContentType: "application/pdf", DocumentType: "invoice"}

	primary.On("Parse", mock.Anything, input).Return(makeParseOutput(&pInv, "claude"), nil)
	secondary.On("Parse", mock.Anything, input).Return(makeParseOutput(&sInv, "gemini"), nil)

	result, err := mp.Parse(context.Background(), input)

	assert.NoError(t, err)

	var mergedData invoice.GSTInvoice
	err = json.Unmarshal(result.StructuredData, &mergedData)
	assert.NoError(t, err)
	assert.Len(t, mergedData.LineItems, 2)
	assert.Equal(t, "primary", result.FieldProvenance["line_items"])
}

func TestMergeParser_PANFormatTiebreak(t *testing.T) {
	primary := new(mocks.MockDocumentParser)
	secondary := new(mocks.MockDocumentParser)
	mp := parser.NewMergeParser(primary, secondary)

	pInv := invoice.GSTInvoice{Seller: invoice.Party{PAN: "INVALID123"}}
	sInv := invoice.GSTInvoice{Seller: invoice.Party{PAN: "ABCDE1234F"}} // valid PAN

	input := port.ParseInput{FileBytes: []byte("test"), ContentType: "application/pdf", DocumentType: "invoice"}

	primary.On("Parse", mock.Anything, input).Return(makeParseOutput(&pInv, "claude"), nil)
	secondary.On("Parse", mock.Anything, input).Return(makeParseOutput(&sInv, "gemini"), nil)

	result, err := mp.Parse(context.Background(), input)

	assert.NoError(t, err)

	var mergedData invoice.GSTInvoice
	_ = json.Unmarshal(result.StructuredData, &mergedData)
	assert.Equal(t, "ABCDE1234F", mergedData.Seller.PAN)
	assert.Equal(t, "secondary_format", result.FieldProvenance["seller.pan"])
}

func TestMergeParser_DateFormatTiebreak(t *testing.T) {
	primary := new(mocks.MockDocumentParser)
	secondary := new(mocks.MockDocumentParser)
	mp := parser.NewMergeParser(primary, secondary)

	pInv := invoice.GSTInvoice{Invoice: invoice.InvoiceHeader{InvoiceDate: "not-a-date"}}
	sInv := invoice.GSTInvoice{Invoice: invoice.InvoiceHeader{InvoiceDate: "15-01-2025"}} // valid date

	input := port.ParseInput{FileBytes: []byte("test"), ContentType: "application/pdf", DocumentType: "invoice"}

	primary.On("Parse", mock.Anything, input).Return(makeParseOutput(&pInv, "claude"), nil)
	secondary.On("Parse", mock.Anything, input).Return(makeParseOutput(&sInv, "gemini"), nil)

	result, err := mp.Parse(context.Background(), input)

	assert.NoError(t, err)

	var mergedData invoice.GSTInvoice
	_ = json.Unmarshal(result.StructuredData, &mergedData)
	assert.Equal(t, "15-01-2025", mergedData.Invoice.InvoiceDate)
	assert.Equal(t, "secondary_format", result.FieldProvenance["invoice.invoice_date"])
}

func TestMergeParser_StateCodeTiebreak(t *testing.T) {
	primary := new(mocks.MockDocumentParser)
	secondary := new(mocks.MockDocumentParser)
	mp := parser.NewMergeParser(primary, secondary)

	pInv := invoice.GSTInvoice{Seller: invoice.Party{StateCode: "99"}} // invalid
	sInv := invoice.GSTInvoice{Seller: invoice.Party{StateCode: "29"}} // valid Karnataka

	input := port.ParseInput{FileBytes: []byte("test"), ContentType: "application/pdf", DocumentType: "invoice"}

	primary.On("Parse", mock.Anything, input).Return(makeParseOutput(&pInv, "claude"), nil)
	secondary.On("Parse", mock.Anything, input).Return(makeParseOutput(&sInv, "gemini"), nil)

	result, err := mp.Parse(context.Background(), input)

	assert.NoError(t, err)

	var mergedData invoice.GSTInvoice
	_ = json.Unmarshal(result.StructuredData, &mergedData)
	assert.Equal(t, "29", mergedData.Seller.StateCode)
	assert.Equal(t, "secondary_format", result.FieldProvenance["seller.state_code"])
}

func TestMergeParser_CurrencyTiebreak(t *testing.T) {
	primary := new(mocks.MockDocumentParser)
	secondary := new(mocks.MockDocumentParser)
	mp := parser.NewMergeParser(primary, secondary)

	pInv := invoice.GSTInvoice{Invoice: invoice.InvoiceHeader{Currency: "Rupees"}}
	sInv := invoice.GSTInvoice{Invoice: invoice.InvoiceHeader{Currency: "INR"}}

	input := port.ParseInput{FileBytes: []byte("test"), ContentType: "application/pdf", DocumentType: "invoice"}

	primary.On("Parse", mock.Anything, input).Return(makeParseOutput(&pInv, "claude"), nil)
	secondary.On("Parse", mock.Anything, input).Return(makeParseOutput(&sInv, "gemini"), nil)

	result, err := mp.Parse(context.Background(), input)

	assert.NoError(t, err)

	var mergedData invoice.GSTInvoice
	_ = json.Unmarshal(result.StructuredData, &mergedData)
	assert.Equal(t, "INR", mergedData.Invoice.Currency)
	assert.Equal(t, "secondary_format", result.FieldProvenance["invoice.currency"])
}

func TestMergeParser_MathConsistencyTiebreak(t *testing.T) {
	primary := new(mocks.MockDocumentParser)
	secondary := new(mocks.MockDocumentParser)
	mp := parser.NewMergeParser(primary, secondary)

	// Line items have CGST = 45 + 45 = 90
	lineItems := []invoice.LineItem{
		{CGSTAmount: 45, SGSTAmount: 45, Total: 590},
		{CGSTAmount: 45, SGSTAmount: 45, Total: 590},
	}
	pInv := invoice.GSTInvoice{
		LineItems: lineItems,
		Totals:    invoice.Totals{CGST: 100}, // wrong
	}
	sInv := invoice.GSTInvoice{
		LineItems: lineItems,
		Totals:    invoice.Totals{CGST: 90}, // matches sum
	}

	input := port.ParseInput{FileBytes: []byte("test"), ContentType: "application/pdf", DocumentType: "invoice"}

	primary.On("Parse", mock.Anything, input).Return(makeParseOutput(&pInv, "claude"), nil)
	secondary.On("Parse", mock.Anything, input).Return(makeParseOutput(&sInv, "gemini"), nil)

	result, err := mp.Parse(context.Background(), input)

	assert.NoError(t, err)

	var mergedData invoice.GSTInvoice
	_ = json.Unmarshal(result.StructuredData, &mergedData)
	assert.Equal(t, float64(90), mergedData.Totals.CGST)
	assert.Equal(t, "secondary_format", result.FieldProvenance["totals.cgst"])
}
