package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultSummaryPrompt = `You are producing a concise summary of the document below.

Include:
- One-paragraph overview (2-3 sentences)
- 3–7 bullet points highlighting the most important claims
- Any explicit action items or open questions

Output pure markdown. No preamble.

DOCUMENT:
`

// summarize sends the input text to an OpenAI-compatible /chat/completions
// endpoint and returns the summary. Empty URL -> nil (no-op — caller should
// have skipped invoking us).
//
// Supported endpoints:
//   - OpenAI:  https://api.openai.com/v1/chat/completions
//   - Ollama:  http://127.0.0.1:11434/v1/chat/completions
//   - LiteLLM proxy: anywhere OpenAI-compatible
//
// For direct Anthropic API, users should use a LiteLLM / OpenAI-compat
// proxy — Anthropic's raw /v1/messages endpoint has a different shape.
func summarize(ctx context.Context, cfg Config, text string) (string, error) {
	if cfg.SummarizerURL == "" {
		return "", fmt.Errorf("summarizer_url not configured")
	}
	model := cfg.SummarizerModel
	if model == "" {
		model = "gpt-4o-mini"
	}
	prompt := cfg.SummarizerPrompt
	if prompt == "" {
		prompt = defaultSummaryPrompt
	}

	body := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "user", "content": prompt + text},
		},
		"temperature": 0.2,
	}
	buf, _ := json.Marshal(body)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", cfg.SummarizerURL, bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.SummarizerKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.SummarizerKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("summarize %d: %s", resp.StatusCode, string(snippet))
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return "", err
	}
	if len(envelope.Choices) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return strings.TrimSpace(envelope.Choices[0].Message.Content), nil
}
