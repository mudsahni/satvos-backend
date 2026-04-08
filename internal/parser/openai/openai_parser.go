package openai

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
	apiURL = "https://api.openai.com/v1/chat/completions"
)

// Parser implements port.DocumentParser using the OpenAI Chat Completions API.
type Parser struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// NewParser creates an OpenAI-based document parser from a provider config.
func NewParser(cfg *config.ParserProviderConfig) *Parser {
	return newParser(cfg, apiURL)
}

// NewParserWithEndpoint creates a parser pointing at a custom API endpoint (for testing).
func NewParserWithEndpoint(cfg *config.ParserProviderConfig, endpoint string) *Parser {
	return newParser(cfg, endpoint)
}

func newParser(cfg *config.ParserProviderConfig, endpoint string) *Parser {
	model := cfg.DefaultModel
	if model == "" {
		model = "gpt-4o"
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
	prompt := parser.BuildGSTInvoicePrompt(input.DocumentType)

	contentBlocks, err := buildContentBlocks(input, prompt)
	if err != nil {
		return nil, fmt.Errorf("building content blocks: %w", err)
	}

	reqBody := map[string]interface{}{
		"model":      p.model,
		"max_completion_tokens": 16384,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": contentBlocks,
			},
		},
		"response_format": map[string]interface{}{
			"type": "json_object",
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
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling openai API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		baseErr := fmt.Errorf("openai API error (status %d): %s", resp.StatusCode, string(respBody))
		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := parser.ParseRetryAfterHeader(resp.Header.Get("Retry-After"))
			return nil, parser.NewRateLimitError("openai", baseErr, retryAfter)
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
		dataURI := fmt.Sprintf("data:%s;base64,%s", input.ContentType, encoded)
		blocks = append(blocks, map[string]interface{}{
			"type": "file",
			"file": map[string]interface{}{
				"filename":  "document.pdf",
				"file_data": dataURI,
			},
		})
	case "image/jpeg", "image/png":
		dataURI := fmt.Sprintf("data:%s;base64,%s", input.ContentType, encoded)
		blocks = append(blocks, map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]interface{}{
				"url": dataURI,
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

// apiResponse models the OpenAI Chat Completions API response.
type apiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func parseResponse(body []byte, model, prompt string) (*port.ParseOutput, error) {
	var resp apiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshaling response: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from API: no choices")
	}

	if resp.Choices[0].FinishReason == "length" {
		return nil, fmt.Errorf("output truncated (finish_reason: length): response exceeded output token limit")
	}

	text := resp.Choices[0].Message.Content

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
