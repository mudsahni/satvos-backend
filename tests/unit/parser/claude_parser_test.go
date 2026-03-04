package parser_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"satvos/internal/config"
	"satvos/internal/parser"
	claude "satvos/internal/parser/claude"
	"satvos/internal/port"
)

func newTestParser(serverURL string) *claude.Parser {
	cfg := &config.ParserProviderConfig{
		Provider:     "claude",
		APIKey:       "test-api-key",
		DefaultModel: "claude-sonnet-4-20250514",
		TimeoutSecs:  30,
	}
	return claude.NewParserWithEndpoint(cfg, serverURL)
}

func TestClaudeParser_Parse_PDF_Success(t *testing.T) {
	responseBody := map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": `{"data":{"invoice":{"invoice_number":"INV-001","invoice_date":"2024-01-15"}},"confidence_scores":{"invoice":{"invoice_number":0.95,"invoice_date":0.9}}}`,
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Verify request body
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)
		assert.Equal(t, "claude-sonnet-4-20250514", reqBody["model"])
		assert.Equal(t, float64(16384), reqBody["max_tokens"])

		messages := reqBody["messages"].([]interface{})
		assert.Len(t, messages, 1)
		msg := messages[0].(map[string]interface{})
		assert.Equal(t, "user", msg["role"])

		content := msg["content"].([]interface{})
		assert.Len(t, content, 2)

		// First block: document
		docBlock := content[0].(map[string]interface{})
		assert.Equal(t, "document", docBlock["type"])

		// Second block: text prompt
		textBlock := content[1].(map[string]interface{})
		assert.Equal(t, "text", textBlock["type"])

		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(responseBody)
		if err != nil {
			return
		}
	}))
	defer server.Close()

	p := newTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test content"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "claude-sonnet-4-20250514", result.ModelUsed)
	assert.NotEmpty(t, result.PromptUsed)

	// Verify structured data
	var data map[string]interface{}
	err = json.Unmarshal(result.StructuredData, &data)
	assert.NoError(t, err)
	invoice := data["invoice"].(map[string]interface{})
	assert.Equal(t, "INV-001", invoice["invoice_number"])

	// Confidence scores should be empty (no longer populated)
	var scores map[string]interface{}
	err = json.Unmarshal(result.ConfidenceScores, &scores)
	assert.NoError(t, err)
	assert.Empty(t, scores)
}

func TestClaudeParser_Parse_Image_Success(t *testing.T) {
	responseBody := map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": `{"data":{"invoice":{"invoice_number":"INV-002"}},"confidence_scores":{"invoice":{"invoice_number":0.8}}}`,
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		if err != nil {
			return
		}

		messages := reqBody["messages"].([]interface{})
		msg := messages[0].(map[string]interface{})
		content := msg["content"].([]interface{})

		// First block should be image
		imgBlock := content[0].(map[string]interface{})
		assert.Equal(t, "image", imgBlock["type"])
		source := imgBlock["source"].(map[string]interface{})
		assert.Equal(t, "image/jpeg", source["media_type"])

		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(responseBody)
		if err != nil {
			return
		}
	}))
	defer server.Close()

	p := newTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte{0xFF, 0xD8, 0xFF, 0xE0},
		ContentType:  "image/jpeg",
		DocumentType: "invoice",
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestClaudeParser_Parse_PNG_Success(t *testing.T) {
	responseBody := map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": `{"data":{"invoice":{"invoice_number":"INV-003"}},"confidence_scores":{"invoice":{"invoice_number":0.7}}}`,
			},
		},
	}

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
		assert.Equal(t, "image", imgBlock["type"])
		source := imgBlock["source"].(map[string]interface{})
		assert.Equal(t, "image/png", source["media_type"])

		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(responseBody)
		if err != nil {
			return
		}
	}))
	defer server.Close()

	p := newTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte{0x89, 0x50, 0x4E, 0x47},
		ContentType:  "image/png",
		DocumentType: "invoice",
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestClaudeParser_Parse_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, err := w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"rate limited"}}`))
		if err != nil {
			return
		}
	}))
	defer server.Close()

	p := newTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)

	var rlErr *parser.RateLimitError
	require.True(t, errors.As(err, &rlErr))
	assert.Equal(t, "claude", rlErr.Provider)
	assert.Equal(t, 30*time.Second, rlErr.RetryAfter)
	assert.Contains(t, rlErr.Err.Error(), "anthropic API error (status 429)")
}

func TestClaudeParser_Parse_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte(`{"error":{"type":"server_error","message":"internal error"}}`))
		if err != nil {
			return
		}
	}))
	defer server.Close()

	p := newTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "anthropic API error (status 500)")
}

func TestClaudeParser_Parse_EmptyResponse(t *testing.T) {
	responseBody := map[string]interface{}{
		"content": []map[string]interface{}{},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(responseBody)
		if err != nil {
			return
		}
	}))
	defer server.Close()

	p := newTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty response")
}

func TestClaudeParser_Parse_InvalidJSONResponse(t *testing.T) {
	responseBody := map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": "This is not JSON at all, sorry!",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(responseBody)
		if err != nil {
			return
		}
	}))
	defer server.Close()

	p := newTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing LLM JSON output")
}

func TestClaudeParser_Parse_UnsupportedContentType(t *testing.T) {
	p := newTestParser("http://unused")

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("text content"),
		ContentType:  "text/plain",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported content type")
}

func TestClaudeParser_Parse_Truncated(t *testing.T) {
	responseBody := map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": `{"data":{"invoice":{"invoice_number":"INV-001"`,
			},
		},
		"stop_reason": "max_tokens",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(responseBody)
		if err != nil {
			return
		}
	}))
	defer server.Close()

	p := newTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "output truncated")
	assert.Contains(t, err.Error(), "stop_reason: max_tokens")
}

func TestClaudeParser_Parse_ConnectionRefused(t *testing.T) {
	p := newTestParser("http://localhost:1")

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "calling anthropic API")
}
