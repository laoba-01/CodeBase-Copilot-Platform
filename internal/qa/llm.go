package qa

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type LLMConfig struct {
	Provider string // claude, deepseek
	APIKey   string
	Model    string
}

type LLMClient struct {
	cfg  LLMConfig
	http *http.Client
}

func NewLLMClient(cfg LLMConfig) *LLMClient {
	return &LLMClient{cfg: cfg, http: &http.Client{}}
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// StreamChat calls the LLM API and sends each text delta to the callback.
func (c *LLMClient) StreamChat(ctx context.Context, messages []ChatMessage, onDelta func(text string)) error {
	var endpoint string
	switch c.cfg.Provider {
	case "claude":
		endpoint = "https://api.anthropic.com/v1/messages"
	case "deepseek":
		endpoint = "https://api.deepseek.com/v1/chat/completions"
	default:
		return fmt.Errorf("unknown LLM provider: %s", c.cfg.Provider)
	}

	reqBody := map[string]any{
		"model":      c.cfg.Model,
		"messages":   messages,
		"stream":     true,
		"max_tokens": 4096,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	if c.cfg.Provider == "claude" {
		req.Header.Set("x-api-key", c.cfg.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("llm error %d: %s", resp.StatusCode, string(body))
	}

	return c.parseSSEStream(resp.Body, onDelta)
}

func (c *LLMClient) parseSSEStream(r io.Reader, onDelta func(text string)) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line == "data: [DONE]" {
			continue
		}

		if len(line) > 6 && line[:6] == "data: " {
			data := line[6:]
			var event map[string]any
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch c.cfg.Provider {
			case "claude":
				if delta, ok := extractClaudeDelta(event); ok && delta != "" {
					onDelta(delta)
				}
			case "deepseek":
				if choices, ok := event["choices"].([]any); ok && len(choices) > 0 {
					choice := choices[0].(map[string]any)
					if delta, ok := choice["delta"].(map[string]any); ok {
						if content, ok := delta["content"].(string); ok {
							onDelta(content)
						}
					}
				}
			}
		}
	}
	return scanner.Err()
}

func extractClaudeDelta(event map[string]any) (string, bool) {
	eventType, _ := event["type"].(string)
	if eventType == "content_block_delta" {
		if delta, ok := event["delta"].(map[string]any); ok {
			if text, ok := delta["text"].(string); ok {
				return text, true
			}
		}
	}
	return "", false
}
