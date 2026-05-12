package alisa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"astroapi/internal/circutebreaker"
	"astroapi/internal/resilience"

	"go.uber.org/zap"
)

//go:generate mockgen -source=client.go -destination=mocks/mock_alisa.go -package=mocks

const (
	defaultTimeout    = 10 * time.Second
	defaultMaxRetries = 3
)

// Generator — единственный наружный интерфейс клиента AlisaAI.
// Используется хендлерами, чтобы позволять мокирование в тестах.
type Generator interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

type Client struct {
	baseURL    string
	apiKey     string
	modelURL   string
	httpClient *http.Client
	maxRetries int
	breaker    *resilience.CircuitBreaker
}

type ClientOptions struct {
	HTTPClient *http.Client
	MaxRetries int
	Logger     *zap.Logger
	Metrics    *circutebreaker.Registry
	Breaker    *resilience.CircuitBreaker
}

type ChatCompletionRequest struct {
	Model             string               `json:"modelUri"`
	CompletionOptions ChatCompletionOption `json:"completionOptions,omitempty"`
	Messages          []ChatMessage        `json:"messages"`
}

type ChatCompletionOption struct {
	Stream      bool   `json:"stream"`
	Temperature string `json:"temperature,omitempty"`
	MaxTokens   string `json:"maxTokens,omitempty"`
}

type ChatMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type ChatCompletionResponse struct {
	Result struct {
		Alternatives []struct {
			Message ChatMessage `json:"message"`
		} `json:"alternatives"`
	} `json:"result"`
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func NewClient(baseURL, apiKey, modelURL string) *Client {
	return NewClientWithOptions(baseURL, apiKey, modelURL, ClientOptions{MaxRetries: defaultMaxRetries})
}

func NewClientWithOptions(baseURL, apiKey, modelURL string, opts ClientOptions) *Client {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}

	maxRetries := opts.MaxRetries
	if maxRetries < 0 {
		maxRetries = defaultMaxRetries
	}

	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	breaker := opts.Breaker
	if breaker == nil {
		breaker = resilience.NewCircuitBreaker("alisa_ai", 5, 30*time.Second, logger, opts.Metrics)
	}

	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		modelURL:   modelURL,
		httpClient: httpClient,
		maxRetries: maxRetries,
		breaker:    breaker,
	}
}

func (c *Client) Generate(ctx context.Context, prompt string) (string, error) {
	requestBody := ChatCompletionRequest{
		Model: c.modelURL,
		CompletionOptions: ChatCompletionOption{
			Stream:      false,
			Temperature: "0.3",
			MaxTokens:   "800",
		},
		Messages: []ChatMessage{{
			Role: "user",
			Text: prompt,
		}},
	}

	responseText, err := c.doRequest(ctx, requestBody)
	if err != nil {
		return "", err
	}

	return responseText, nil
}

func (c *Client) Ping(ctx context.Context) error {
	state := c.breaker.CurrentState()
	if state == resilience.StateClosed {
		return nil
	}
	return fmt.Errorf("circuit breaker state: %d", state)
}

func (c *Client) doRequest(ctx context.Context, requestBody ChatCompletionRequest) (string, error) {
	var responseText string
	var err error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		err = c.breaker.Execute(func() error {
			var sendErr error
			responseText, sendErr = c.send(ctx, requestBody)
			return sendErr
		})
		if err == nil {
			return responseText, nil
		}

		if resilience.IsServiceUnavailable(err) {
			return "", err
		}
		if !isRetryableError(err) || attempt == c.maxRetries {
			return "", err
		}
	}

	return "", err
}

func (c *Client) send(ctx context.Context, requestBody ChatCompletionRequest) (text string, err error) {
	payload, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("marshal AlisaAI request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create AlisaAI request: %w", err)
	}

	request.Header.Set("Authorization", "Api-Key "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("send AlisaAI request: %w", err)
	}
	defer func() {
		errClose := response.Body.Close()
		if errClose != nil {
			err = errors.Join(err, fmt.Errorf("close AlisaAI response body: %w", errClose))
		}
	}()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("read AlisaAI response: %w", err)
	}

	if response.StatusCode >= http.StatusInternalServerError {
		return "", retryableError{statusCode: response.StatusCode, message: string(body)}
	}

	if response.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("AlisaAI request failed: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded ChatCompletionResponse
	if err = json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("decode AlisaAI response: %w", err)
	}

	text = extractResponseText(decoded)
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("AlisaAI response is empty")
	}

	return text, nil
}

func extractResponseText(response ChatCompletionResponse) string {
	text := ""

	if len(response.Result.Alternatives) > 0 {
		text = response.Result.Alternatives[0].Message.Text
	} else if len(response.Choices) > 0 {
		text = response.Choices[0].Message.Text
	} else if response.Error != nil {
		text = response.Error.Message
	}

	return text
}

type retryableError struct {
	statusCode int
	message    string
}

func (e retryableError) Error() string {
	return fmt.Sprintf("AlisaAI temporary error: status=%d body=%s", e.statusCode, strings.TrimSpace(e.message))
}

func isRetryableError(err error) bool {
	var re retryableError
	return errors.As(err, &re)
}

// static assertion на соответствие интерфейсу
var _ Generator = (*Client)(nil)
