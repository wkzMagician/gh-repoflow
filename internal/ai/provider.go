package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Request struct {
	SystemPrompt string
	UserPrompt   string
	JSONSchema   map[string]interface{}
}

type Response struct {
	Content string
}

type Provider interface {
	Generate(ctx context.Context, request Request) (Response, error)
}

type OpenAICompatible struct {
	BaseURL string
	Model   string
	APIKey  string
	Client  *http.Client
}

func (p OpenAICompatible) Generate(ctx context.Context, request Request) (Response, error) {
	if p.BaseURL == "" || p.Model == "" || p.APIKey == "" {
		return Response{}, errors.New("AI provider requires base URL, model, and API key")
	}
	payload := map[string]interface{}{
		"model": p.Model,
		"messages": []map[string]string{
			{"role": "system", "content": request.SystemPrompt},
			{"role": "user", "content": request.UserPrompt},
		},
		"temperature": 0,
	}
	if request.JSONSchema != nil {
		payload["response_format"] = map[string]interface{}{
			"type":        "json_schema",
			"json_schema": request.JSONSchema,
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}
	endpoint := strings.TrimRight(p.BaseURL, "/") + "/chat/completions"
	httpClient := p.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 90 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("create AI request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("call AI provider: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("AI provider returned HTTP %s", resp.Status)
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Response{}, fmt.Errorf("decode AI provider response: %w", err)
	}
	if len(result.Choices) == 0 || strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		return Response{}, errors.New("AI provider returned no message content")
	}
	return Response{Content: result.Choices[0].Message.Content}, nil
}
