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
	gemini "satvos/internal/parser/gemini"
	"satvos/internal/port"
)

func newGeminiTestParser(serverURL string) *gemini.Parser {
	cfg := &config.ParserProviderConfig{
		Provider:     "gemini",
		APIKey:       "test-gemini-key",
		DefaultModel: "gemini-2.0-flash",
		TimeoutSecs:  30,
	}
	return gemini.NewParserWithEndpoint(cfg, serverURL)
}

func geminiSuccessResponse(text string) map[string]interface{} {
	return map[string]interface{}{
		"candidates": []map[string]interface{}{
			{
				"content": map[string]interface{}{
					"role": "model",
					"parts": []map[string]interface{}{
						{"text": text},
					},
				},
				"finishReason": "STOP",
			},
		},
	}
}

func TestGeminiParser_Parse_PDF_Success(t *testing.T) {
	llmJSON := `{"data":{"invoice":{"invoice_number":"INV-001","invoice_date":"2024-01-15"}},"confidence_scores":{"invoice":{"invoice_number":0.95,"invoice_date":0.9}}}`
	responseBody := geminiSuccessResponse(llmJSON)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		assert.Equal(t, "test-gemini-key", r.Header.Get("x-goog-api-key"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Verify request body
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)

		contents := reqBody["contents"].([]interface{})
		assert.Len(t, contents, 1)
		msg := contents[0].(map[string]interface{})
		assert.Equal(t, "user", msg["role"])

		parts := msg["parts"].([]interface{})
		assert.Len(t, parts, 2)

		// First part: inline_data
		dataPart := parts[0].(map[string]interface{})
		inlineData := dataPart["inline_data"].(map[string]interface{})
		assert.Equal(t, "application/pdf", inlineData["mime_type"])
		assert.NotEmpty(t, inlineData["data"])

		// Second part: text prompt
		textPart := parts[1].(map[string]interface{})
		assert.NotEmpty(t, textPart["text"])

		// Verify generationConfig
		genConfig := reqBody["generationConfig"].(map[string]interface{})
		assert.Equal(t, "application/json", genConfig["responseMimeType"])
		assert.Equal(t, float64(65536), genConfig["maxOutputTokens"])

		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(responseBody)
		if err != nil {
			return
		}
	}))
	defer server.Close()

	p := newGeminiTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test content"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "gemini-2.0-flash", result.ModelUsed)
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

func TestGeminiParser_Parse_JPEG_Success(t *testing.T) {
	llmJSON := `{"data":{"invoice":{"invoice_number":"INV-002"}},"confidence_scores":{"invoice":{"invoice_number":0.8}}}`
	responseBody := geminiSuccessResponse(llmJSON)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		if err != nil {
			return
		}

		contents := reqBody["contents"].([]interface{})
		msg := contents[0].(map[string]interface{})
		parts := msg["parts"].([]interface{})

		dataPart := parts[0].(map[string]interface{})
		inlineData := dataPart["inline_data"].(map[string]interface{})
		assert.Equal(t, "image/jpeg", inlineData["mime_type"])

		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(responseBody)
		if err != nil {
			return
		}
	}))
	defer server.Close()

	p := newGeminiTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte{0xFF, 0xD8, 0xFF, 0xE0},
		ContentType:  "image/jpeg",
		DocumentType: "invoice",
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGeminiParser_Parse_PNG_Success(t *testing.T) {
	llmJSON := `{"data":{"invoice":{"invoice_number":"INV-003"}},"confidence_scores":{"invoice":{"invoice_number":0.7}}}`
	responseBody := geminiSuccessResponse(llmJSON)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		if err != nil {
			return
		}

		contents := reqBody["contents"].([]interface{})
		msg := contents[0].(map[string]interface{})
		parts := msg["parts"].([]interface{})

		dataPart := parts[0].(map[string]interface{})
		inlineData := dataPart["inline_data"].(map[string]interface{})
		assert.Equal(t, "image/png", inlineData["mime_type"])

		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(responseBody)
		if err != nil {
			return
		}
	}))
	defer server.Close()

	p := newGeminiTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte{0x89, 0x50, 0x4E, 0x47},
		ContentType:  "image/png",
		DocumentType: "invoice",
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestGeminiParser_Parse_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, err := w.Write([]byte(`{"error":{"code":429,"message":"Resource has been exhausted","status":"RESOURCE_EXHAUSTED"}}`))
		if err != nil {
			return
		}
	}))
	defer server.Close()

	p := newGeminiTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)

	var rlErr *parser.RateLimitError
	require.True(t, errors.As(err, &rlErr))
	assert.Equal(t, "gemini", rlErr.Provider)
	assert.Equal(t, 30*time.Second, rlErr.RetryAfter)
	assert.Contains(t, rlErr.Err.Error(), "gemini API error (status 429)")
	assert.Contains(t, rlErr.Err.Error(), "RESOURCE_EXHAUSTED")
}

func TestGeminiParser_Parse_EmptyResponse(t *testing.T) {
	responseBody := map[string]interface{}{
		"candidates": []map[string]interface{}{},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(responseBody)
		if err != nil {
			return
		}
	}))
	defer server.Close()

	p := newGeminiTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty response from API: no candidates")
}

func TestGeminiParser_Parse_EmptyParts(t *testing.T) {
	responseBody := map[string]interface{}{
		"candidates": []map[string]interface{}{
			{
				"content": map[string]interface{}{
					"role":  "model",
					"parts": []map[string]interface{}{},
				},
				"finishReason": "STOP",
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

	p := newGeminiTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty response from API: no parts")
}

func TestGeminiParser_Parse_InvalidJSON(t *testing.T) {
	responseBody := geminiSuccessResponse("This is not JSON at all, sorry!")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(responseBody)
		if err != nil {
			return
		}
	}))
	defer server.Close()

	p := newGeminiTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing LLM JSON output")
}

func TestGeminiParser_Parse_UnsupportedContentType(t *testing.T) {
	p := newGeminiTestParser("http://unused")

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("text content"),
		ContentType:  "text/plain",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported content type")
}

func TestGeminiParser_Parse_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte(`{"error":{"code":500,"message":"Internal error","status":"INTERNAL"}}`))
		if err != nil {
			return
		}
	}))
	defer server.Close()

	p := newGeminiTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gemini API error (status 500)")
}

func TestGeminiParser_Parse_VerifyRequestFormat(t *testing.T) {
	llmJSON := `{"data":{"invoice":{}},"confidence_scores":{"invoice":{}}}`
	responseBody := geminiSuccessResponse(llmJSON)

	var capturedReq map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify HTTP method
		assert.Equal(t, http.MethodPost, r.Method)

		// Verify auth header
		assert.Equal(t, "test-gemini-key", r.Header.Get("x-goog-api-key"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		err := json.NewDecoder(r.Body).Decode(&capturedReq)
		assert.NoError(t, err)

		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(responseBody)
		if err != nil {
			return
		}
	}))
	defer server.Close()

	p := newGeminiTestParser(server.URL)

	_, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test content"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})
	require.NoError(t, err)

	// Verify top-level structure
	assert.Contains(t, capturedReq, "contents")
	assert.Contains(t, capturedReq, "generationConfig")

	// Verify contents structure
	contents := capturedReq["contents"].([]interface{})
	assert.Len(t, contents, 1)
	msg := contents[0].(map[string]interface{})
	assert.Equal(t, "user", msg["role"])

	parts := msg["parts"].([]interface{})
	assert.Len(t, parts, 2)

	// Verify inline_data part
	dataPart := parts[0].(map[string]interface{})
	assert.Contains(t, dataPart, "inline_data")
	inlineData := dataPart["inline_data"].(map[string]interface{})
	assert.Equal(t, "application/pdf", inlineData["mime_type"])
	assert.NotEmpty(t, inlineData["data"])

	// Verify text part
	textPart := parts[1].(map[string]interface{})
	assert.Contains(t, textPart, "text")
	assert.NotEmpty(t, textPart["text"])

	// Verify generationConfig
	genConfig := capturedReq["generationConfig"].(map[string]interface{})
	assert.Equal(t, "application/json", genConfig["responseMimeType"])
	assert.Equal(t, float64(65536), genConfig["maxOutputTokens"])
}

func TestGeminiParser_Parse_Truncated(t *testing.T) {
	responseBody := map[string]interface{}{
		"candidates": []map[string]interface{}{
			{
				"content": map[string]interface{}{
					"role": "model",
					"parts": []map[string]interface{}{
						{"text": `{"data":{"invoice":{"invoice_number":"INV-001"`},
					},
				},
				"finishReason": "MAX_TOKENS",
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

	p := newGeminiTestParser(server.URL)

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "output truncated")
	assert.Contains(t, err.Error(), "finishReason: MAX_TOKENS")
}

func TestGeminiParser_Parse_ConnectionRefused(t *testing.T) {
	p := newGeminiTestParser("http://localhost:1")

	result, err := p.Parse(context.Background(), port.ParseInput{
		FileBytes:    []byte("%PDF-1.4 test"),
		ContentType:  "application/pdf",
		DocumentType: "invoice",
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "calling gemini API")
}
