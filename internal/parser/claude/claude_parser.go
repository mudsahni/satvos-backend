package claude

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
	apiURL     = "https://api.anthropic.com/v1/messages"
	apiVersion = "2023-06-01"
)

// Parser implements port.DocumentParser using the Anthropic Messages API.
type Parser struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// NewParser creates a Claude-based document parser from a provider config.
func NewParser(cfg *config.ParserProviderConfig) *Parser {
	return newParser(cfg, apiURL)
}

// NewParserFromLegacy creates a Claude-based document parser from a legacy ParserConfig.
func NewParserFromLegacy(cfg *config.ParserConfig) *Parser {
	return newParser(cfg.PrimaryConfig(), apiURL)
}

// NewParserWithEndpoint creates a parser pointing at a custom API endpoint (for testing).
func NewParserWithEndpoint(cfg *config.ParserProviderConfig, endpoint string) *Parser {
	return newParser(cfg, endpoint)
}

func newParser(cfg *config.ParserProviderConfig, endpoint string) *Parser {
	model := cfg.DefaultModel
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	timeout := time.Duration(cfg.TimeoutSecs) * time.Second
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	return &Parser{
		apiKey:   cfg.APIKey,
		model:    model,
		endpoint: endpoint,
		client:   &http.Client{Timeout: timeout},
	}
}

func (p *Parser) Parse(ctx context.Context, input port.ParseInput) (*port.ParseOutput, error) {
	prompt := buildPrompt(input.DocumentType)

	contentBlocks, err := buildContentBlocks(input, prompt)
	if err != nil {
		return nil, fmt.Errorf("building content blocks: %w", err)
	}

	reqBody := map[string]interface{}{
		"model":      p.model,
		"max_tokens": 16384,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": contentBlocks,
			},
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
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", apiVersion)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling anthropic API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		baseErr := fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, string(respBody))
		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := parser.ParseRetryAfterHeader(resp.Header.Get("Retry-After"))
			return nil, parser.NewRateLimitError("claude", baseErr, retryAfter)
		}
		return nil, baseErr
	}

	return parseResponse(respBody, p.model, prompt)
}

func buildContentBlocks(input port.ParseInput, prompt string) ([]map[string]interface{}, error) {
	encoded := base64.StdEncoding.EncodeToString(input.FileBytes)
	var blocks []map[string]interface{}

	switch input.ContentType {
	case "application/pdf":
		blocks = append(blocks, map[string]interface{}{
			"type": "document",
			"source": map[string]interface{}{
				"type":       "base64",
				"media_type": "application/pdf",
				"data":       encoded,
			},
		})
	case "image/jpeg", "image/png":
		blocks = append(blocks, map[string]interface{}{
			"type": "image",
			"source": map[string]interface{}{
				"type":       "base64",
				"media_type": input.ContentType,
				"data":       encoded,
			},
		})
	default:
		return nil, fmt.Errorf("unsupported content type for parsing: %s", input.ContentType)
	}

	blocks = append(blocks, map[string]interface{}{
		"type": "text",
		"text": prompt,
	})

	return blocks, nil
}

func buildPrompt(documentType string) string {
	return parser.BuildGSTInvoicePrompt(documentType)
}

// apiResponse models the Anthropic Messages API response.
type apiResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
}

func parseResponse(body []byte, model, prompt string) (*port.ParseOutput, error) {
	var resp apiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshaling response: %w", err)
	}

	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("empty response from API")
	}

	if resp.StopReason == "max_tokens" {
		return nil, fmt.Errorf("output truncated (stop_reason: max_tokens): response exceeded output token limit")
	}

	text := resp.Content[0].Text

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
	// If it has "invoice" or "seller" keys, it's bare format
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
