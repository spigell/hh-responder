package gemini

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"google.golang.org/genai"
)

const (
	defaultModel      = "gemini-2.5-pro"
	defaultMaxRetries = 3
	retryInitialDelay = 100 * time.Millisecond
	retryMaxDelay     = 3 * time.Second
	maxQuotaDelay     = 180 * time.Second
)

var sleep = time.Sleep

type modelGenerator interface {
	GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
}

// Generator wraps the Google GenAI client to provide simple prompt-based interactions.
type Generator struct {
	models     modelGenerator
	model      string
	maxRetries int
	logger     *zap.Logger
}

// NewGenerator creates a new Generator configured for the Gemini API backend.
func NewGenerator(ctx context.Context, apiKey, model string, maxRetries int, logger *zap.Logger) (*Generator, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errors.New("gemini api key is required")
	}

	cfg := &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	}

	client, err := genai.NewClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create genai client: %w", err)
	}

	if model = strings.TrimSpace(model); model == "" {
		model = defaultModel
	}

	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &Generator{
		models:     client.Models,
		model:      model,
		maxRetries: maxRetries,
		logger:     logger,
	}, nil
}

// GenerateContent sends the prompt to Gemini and returns the first textual response.
func (g *Generator) GenerateContent(ctx context.Context, prompt string) (string, error) {
	if g == nil || g.models == nil {
		return "", errors.New("gemini generator is not initialized")
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("prompt must not be empty")
	}

	return g.generateWithModel(ctx, g.model, prompt)
}

func (g *Generator) MaxRetries() int {
	if g == nil {
		return 0
	}
	if g.maxRetries <= 0 {
		return defaultMaxRetries
	}
	return g.maxRetries
}

func (g *Generator) generateWithModel(ctx context.Context, model, prompt string) (string, error) {
	attempts := g.maxRetries
	if attempts <= 0 {
		attempts = defaultMaxRetries
	}

	delay := retryInitialDelay

	contents := genai.Text(prompt)

	for attempt := 1; attempt <= attempts; attempt++ {
		resp, err := g.models.GenerateContent(ctx, model, contents, nil)
		if err == nil {
			output := extractText(resp)
			if output == "" {
				return "", errors.New("gemini api returned empty response")
			}
			return output, nil
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}

		decision := classifyRetry(err)
		if !decision.retry || attempt == attempts {
			return "", fmt.Errorf("generate content: %w", err)
		}

		wait := delay
		if decision.delay > 0 {
			wait = decision.delay
		}

		g.logger.Debug("gemini request retry in details",
			zap.String("model", model),
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", attempts),
			zap.String("delay", wait.String()),
			zap.Bool("quota_retry", decision.quota),
			zap.Error(err),
		)

		g.logger.Info("gemini request retry occured",
			zap.String("model", model),
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", attempts),
			zap.String("delay", wait.String()),
			zap.Bool("quota_retry", decision.quota),
		)

		if err := waitFor(ctx, wait); err != nil {
			return "", err
		}

		if decision.delay == 0 {
			delay = minDuration(delay*2, retryMaxDelay)
		}
	}

	return "", errors.New("gemini generate content failed")
}

func extractText(resp *genai.GenerateContentResponse) string {
	if resp == nil {
		return ""
	}
	var builder strings.Builder
	for _, candidate := range resp.Candidates {
		if candidate == nil || candidate.Content == nil {
			continue
		}
		for _, part := range candidate.Content.Parts {
			if part == nil {
				continue
			}
			text := strings.TrimSpace(part.Text)
			if text == "" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(text)
		}
	}

	return strings.TrimSpace(builder.String())
}

func waitFor(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		sleep(d)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

type retryDecision struct {
	retry bool
	delay time.Duration
	quota bool
}


func classifyRetry(err error) retryDecision {
	switch {
	case err == nil:
		// no retry. no error
		return retryDecision{}

	case errors.Is(err, context.Canceled),
		errors.Is(err, context.DeadlineExceeded):
		// no retry
		return retryDecision{}

	case func() bool {
		var netErr net.Error
		return errors.As(err, &netErr) && netErr.Timeout()
	}():
		// retry on timeout
		return retryDecision{retry: true}

	case func() bool {
		var apiErr genai.APIError
		return errors.As(err, &apiErr)
	}():
		var apiErr genai.APIError
		_ = errors.As(err, &apiErr) // safe second extract

		// check for quota-based retry
		if delay, ok := retryDelayFromAPIError(apiErr); ok {
			return retryDecision{retry: true, delay: delay, quota: true}
		}

		// retry on 408, 5xx, or UNAVAILABLE
		if apiErr.Code == http.StatusRequestTimeout ||
			(apiErr.Code >= 500 && apiErr.Code < 600) ||
			strings.EqualFold(apiErr.Status, "UNAVAILABLE") {
			return retryDecision{retry: true}
		}

	default:
		// no retry
		return retryDecision{}
	}

	return retryDecision{}
}


var retryAfterRegex = regexp.MustCompile(`(?i)(?:-)?[^\d]*(\d+(?:\.\d+)?)\s*(seconds|secs|s|ms|milliseconds|minutes|mins|m)?`)

func retryDelayFromAPIError(err genai.APIError) (time.Duration, bool) {
	if !isQuotaAPIError(err) {
		return 0, false
	}

	var delay time.Duration
	var retrieved bool

	for _, detail := range err.Details {
		if d, ok := parseDelayFromMap(detail); ok {
			retrieved = ok
			delay = d
		}
	}

	if !retrieved {
		return 0, false
	}

	if delay >= maxQuotaDelay {
		return 0, false
	}

	// Add 1 second to ensure
	return delay + time.Second*1, true
}

func isQuotaAPIError(err genai.APIError) bool {
	if err.Code == http.StatusTooManyRequests {
		return true
	}
	if strings.EqualFold(err.Status, "RESOURCE_EXHAUSTED") {
		return true
	}
	message := strings.ToLower(err.Message)
	if strings.Contains(message, "quota") || strings.Contains(message, "exhaust") {
		return true
	}
	for _, detail := range err.Details {
		for key, value := range detail {
			if strings.EqualFold(key, "reason") {
				if str, ok := value.(string); ok && strings.Contains(strings.ToLower(str), "quota") {
					return true
				}
			}
		}
	}
	return false
}

func parseDelayFromMap(values map[string]any) (time.Duration, bool) {
	var match any
	for key, value := range values {
		if key == "retryDelay" {
			match = value
			break
		}
	}

	switch v := match.(type) {
	case string:
		return parseDelayFromString(v)
	case float64:
		return secondsToDuration(v), true
	case int:
		return secondsToDuration(float64(v)), true
	case map[string]any:
		return parseDelayFromMap(v)
	case nil:
		return 0, false
	default:
		return 0, false
	}
}

func parseDelayFromString(val string) (time.Duration, bool) {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0, false
	}

	matches := retryAfterRegex.FindStringSubmatch(val); 
       if len(matches) <= 2 {
               return 0, false
       }

       num, _ := strconv.ParseFloat(matches[1], 64)
       unit := strings.ToLower(matches[2])


	switch unit {
	case "", "s", "sec", "secs", "second", "seconds":
		return secondsToDuration(num), true
	case "ms", "millisecond", "milliseconds":
		return time.Duration(num * float64(time.Millisecond)), true
	case "m", "min", "mins", "minute", "minutes":
		return time.Duration(num * float64(time.Minute)), true
	default:
		return 0, false
	}
}

func secondsToDuration(v float64) time.Duration {
	return time.Duration(v * float64(time.Second))
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
