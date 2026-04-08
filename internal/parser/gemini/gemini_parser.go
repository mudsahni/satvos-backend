package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"satvos/internal/config"
	"satvos/internal/parser"
	"satvos/internal/port"
)

const (
	apiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"
)

// Parser implements port.DocumentParser using Google's Gemini API.
type Parser struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// NewParser creates a Gemini-based document parser.
func NewParser(cfg *config.ParserProviderConfig) *Parser {
	return newParser(cfg, "")
}

// NewParserWithEndpoint creates a parser pointing at a custom API endpoint (for testing).
func NewParserWithEndpoint(cfg *config.ParserProviderConfig, endpoint string) *Parser {
	return newParser(cfg, endpoint)
}

func newParser(cfg *config.ParserProviderConfig, endpoint string) *Parser {
	model := cfg.DefaultModel
	if model == "" {
		model = "gemini-2.0-flash"
	}
	timeout := time.Duration(cfg.TimeoutSecs) * time.Second
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	if endpoint == "" {
		endpoint = fmt.Sprintf("%s/%s:generateContent", apiBaseURL, model)
	}
	return &Parser{
		apiKey:   cfg.APIKey,
		model:    model,
		endpoint: endpoint,
		client:   &http.Client{Timeout: timeout},
	}
}

func (p *Parser) Parse(ctx context.Context, input port.ParseInput) (*port.ParseOutput, error) {
	prompt := parser.BuildGSTInvoicePrompt(input.DocumentType)

	mimeType, err := toGeminiMimeType(input.ContentType)
	if err != nil {
		return nil, err
	}

	encoded := base64.StdEncoding.EncodeToString(input.FileBytes)

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{
						"inline_data": map[string]interface{}{
							"mime_type": mimeType,
							"data":     encoded,
						},
					},
					{
						"text": prompt,
					},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"responseMimeType": "application/json",
			"maxOutputTokens":  65536,
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling gemini API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		baseErr := fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(respBody))
		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := parser.ParseRetryAfterHeader(resp.Header.Get("Retry-After"))
			return nil, parser.NewRateLimitError("gemini", baseErr, retryAfter)
		}
		return nil, baseErr
	}

	return parseResponse(respBody, p.model, prompt)
}

func toGeminiMimeType(contentType string) (string, error) {
	switch contentType {
	case "application/pdf":
		return "application/pdf", nil
	case "image/jpeg":
		return "image/jpeg", nil
	case "image/png":
		return "image/png", nil
	default:
		return "", fmt.Errorf("unsupported content type for parsing: %s", contentType)
	}
}

// geminiResponse models the Gemini API response.
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
}

func parseResponse(body []byte, model, prompt string) (*port.ParseOutput, error) {
	var resp geminiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshaling response: %w", err)
	}

	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("empty response from API: no candidates")
	}

	if resp.Candidates[0].FinishReason == "MAX_TOKENS" {
		return nil, fmt.Errorf("output truncated (finishReason: MAX_TOKENS): response exceeded output token limit")
	}

	if len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response from API: no parts")
	}

	text := resp.Candidates[0].Content.Parts[0].Text

	structuredData, err := extractStructuredData([]byte(text))
	if err != nil {
		return nil, fmt.Errorf("parsing LLM JSON output: %w (raw: %s)", err, truncate(text, 500))
	}

	return &port.ParseOutput{
		StructuredData:   structuredData,
		ConfidenceScores: json.RawMessage("{}"),
		ModelUsed:        model,
		PromptUsed:       prompt,
	}, nil
}

// extractStructuredData tries bare GSTInvoice JSON first, then falls back
// to the old {"data": ..., "confidence_scores": ...} wrapper format.
func extractStructuredData(raw []byte) (json.RawMessage, error) {
	// Try old wrapper format first (has "data" key)
	var wrapper struct {
		Data             json.RawMessage `json:"data"`
		ConfidenceScores json.RawMessage `json:"confidence_scores"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Data) > 0 {
		return wrapper.Data, nil
	}

	// Try bare format (direct GSTInvoice JSON)
	var check map[string]json.RawMessage
	if err := json.Unmarshal(raw, &check); err != nil {
		return nil, err
	}
	if _, ok := check["invoice"]; ok {
		return raw, nil
	}
	if _, ok := check["seller"]; ok {
		return raw, nil
	}

	return nil, fmt.Errorf("unrecognized JSON structure: missing 'data' wrapper and 'invoice' key")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
