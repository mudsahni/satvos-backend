package parser_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"satvos/internal/config"
	"satvos/internal/parser"
	openai "satvos/internal/parser/openai"
	"satvos/internal/port"
)

func newOpenAITestParser(serverURL string) *openai.Parser {
	cfg := &config.ParserProviderConfig{
		Provider:     "openai",
		APIKey:       "test-openai-key",
		DefaultModel: "gpt-4o",
		TimeoutSecs:  30,
	}
	return openai.NewParserWithEndpoint(cfg, serverURL)
}

func openaiSuccessResponse(content string) map[string]interface{} {
	return map[string]interface{}{
		"choices": []map[string]interface{}{
			{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
	}
}

func TestOpenAIParser_Parse_PDF_Success(t *testing.T) {
	llmJSON := `{"data":{"invoice":{"invoice_number":"INV-001","invoice_date":"2024-01-15"}},"confidence_scores":{"invoice":{"invoice_number":0.95,"invoice_date":0.9}}}`
	responseBody := openaiSuccessResponse(llmJSON)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		assert.Equal(t, "Bearer test-openai-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Verify request body
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)
		assert.Equal(t, "gpt-4o", reqBody["model"])
		assert.Equal(t, float64(16384), reqBody["max_completion_tokens"])

		messages := reqBody["messages"].([]interface{})
		assert.Len(t, messages, 1)
		msg := messages[0].(map[string]interface{})
		assert.Equal(t, "user", msg["role"])

		content := msg["content"].([]interface{})
		assert.Len(t, content, 2)

		// First block: file (PDFs use file content type, not image_url)
		fileBlock := content[0].(map[string]interface{})
		assert.Equal(t, "file", fileBlock["type"])
		fileData := fileBlock["file"].(map[string]interface{})
		assert.Equal(t, "document.pdf", fileData["filename"])
		assert.Contains(t, fileData["file_data"], "data:application/pdf;base64,")

		// Second block: text prompt
		textBlock := content[1].(map[string]interface{})
		assert.Equal(t, "text", textBlock["type"])

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(responseBody)
	}))
	defer server.Close()

	p := newOpenAITestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test content"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "gpt-4o", result.ModelUsed)
	assert.NotEmpty(t, result.PromptUsed)

	// Verify structured data
	var data map[string]interface{}
	err = json.Unmarshal(result.StructuredData, &data)
	assert.NoError(t, err)
	inv := data["invoice"].(map[string]interface{})
	assert.Equal(t, "INV-001", inv["invoice_number"])

	// Confidence scores should be empty (no longer populated)
	var scores map[string]interface{}
	err = json.Unmarshal(result.ConfidenceScores, &scores)
	assert.NoError(t, err)
	assert.Empty(t, scores)
}

func TestOpenAIParser_Parse_Image_JPEG_Success(t *testing.T) {
	llmJSON := `{"data":{"invoice":{"invoice_number":"INV-002"}},"confidence_scores":{"invoice":{"invoice_number":0.8}}}`
	responseBody := openaiSuccessResponse(llmJSON)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		if err != nil {
			return
		}

		messages := reqBody["messages"].([]interface{})
		msg := messages[0].(map[string]interface{})
		content := msg["content"].([]interface{})

		imgBlock := content[0].(map[string]interface{})
		assert.Equal(t, "image_url", imgBlock["type"])
		imgURL := imgBlock["image_url"].(map[string]interface{})
		assert.Contains(t, imgURL["url"], "data:image/jpeg;base64,")

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(responseBody)
	}))
	defer server.Close()

	p := newOpenAITestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte{0xFF, 0xD8, 0xFF, 0xE0},
		ContentType:  "image/jpeg",
		DocumentType: "invoice",
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestOpenAIParser_Parse_Image_PNG_Success(t *testing.T) {
	llmJSON := `{"data":{"invoice":{"invoice_number":"INV-003"}},"confidence_scores":{"invoice":{"invoice_number":0.7}}}`
	responseBody := openaiSuccessResponse(llmJSON)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		if err != nil {
			return
		}

		messages := reqBody["messages"].([]interface{})
		msg := messages[0].(map[string]interface{})
		content := msg["content"].([]interface{})

		imgBlock := content[0].(map[string]interface{})
		imgURL := imgBlock["image_url"].(map[string]interface{})
		assert.Contains(t, imgURL["url"], "data:image/png;base64,")

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(responseBody)
	}))
	defer server.Close()

	p := newOpenAITestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte{0x89, 0x50, 0x4E, 0x47},
		ContentType:  "image/png",
		DocumentType: "invoice",
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestOpenAIParser_Parse_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`))
	}))
	defer server.Close()

	p := newOpenAITestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)

	var rlErr *parser.RateLimitError
	require.True(t, errors.As(err, &rlErr))
	assert.Equal(t, "openai", rlErr.Provider)
	assert.Equal(t, 30*1e9, float64(rlErr.RetryAfter)) // 30s in nanoseconds
	assert.Contains(t, rlErr.Err.Error(), "openai API error (status 429)")
}

func TestOpenAIParser_Parse_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"Internal server error","type":"server_error"}}`))
	}))
	defer server.Close()

	p := newOpenAITestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "openai API error (status 500)")

	var rlErr *parser.RateLimitError
	assert.False(t, errors.As(err, &rlErr))
}

func TestOpenAIParser_Parse_EmptyResponse(t *testing.T) {
	responseBody := map[string]interface{}{
		"choices": []map[string]interface{}{},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(responseBody)
	}))
	defer server.Close()

	p := newOpenAITestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no choices")
}

func TestOpenAIParser_Parse_InvalidJSON(t *testing.T) {
	responseBody := openaiSuccessResponse("This is not JSON at all, sorry!")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(responseBody)
	}))
	defer server.Close()

	p := newOpenAITestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing LLM JSON output")
}

func TestOpenAIParser_Parse_UnsupportedContentType(t *testing.T) {
	p := newOpenAITestParser("http://unused")

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("text content"),
		ContentType:  "text/plain",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported content type")
}

func TestOpenAIParser_Parse_VerifyRequestFormat(t *testing.T) {
	llmJSON := `{"data":{"invoice":{}},"confidence_scores":{"invoice":{}}}`
	responseBody := openaiSuccessResponse(llmJSON)

	var capturedReq map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "Bearer test-openai-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		err := json.NewDecoder(r.Body).Decode(&capturedReq)
		assert.NoError(t, err)

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(responseBody)
	}))
	defer server.Close()

	p := newOpenAITestParser(server.URL)

	_, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test content"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})
	require.NoError(t, err)

	// Verify top-level structure
	assert.Equal(t, "gpt-4o", capturedReq["model"])
	assert.Equal(t, float64(16384), capturedReq["max_completion_tokens"])
	assert.Contains(t, capturedReq, "messages")
	assert.Contains(t, capturedReq, "response_format")

	// Verify response_format
	respFmt := capturedReq["response_format"].(map[string]interface{})
	assert.Equal(t, "json_object", respFmt["type"])

	// Verify messages structure
	messages := capturedReq["messages"].([]interface{})
	assert.Len(t, messages, 1)
	msg := messages[0].(map[string]interface{})
	assert.Equal(t, "user", msg["role"])

	content := msg["content"].([]interface{})
	assert.Len(t, content, 2)

	// Verify file part (PDFs use file content type, not image_url)
	fileBlock := content[0].(map[string]interface{})
	assert.Equal(t, "file", fileBlock["type"])
	assert.Contains(t, fileBlock, "file")
	fileData := fileBlock["file"].(map[string]interface{})
	assert.Contains(t, fileData["file_data"], "data:application/pdf;base64,")

	// Verify text part
	textBlock := content[1].(map[string]interface{})
	assert.Equal(t, "text", textBlock["type"])
	assert.NotEmpty(t, textBlock["text"])
}

func TestOpenAIParser_Parse_Truncated(t *testing.T) {
	responseBody := map[string]interface{}{
		"choices": []map[string]interface{}{
			{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": `{"data":{"invoice":{"invoice_number":"INV-001"`,
				},
				"finish_reason": "length",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(responseBody)
	}))
	defer server.Close()

	p := newOpenAITestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "output truncated")
	assert.Contains(t, err.Error(), "finish_reason: length")
}
