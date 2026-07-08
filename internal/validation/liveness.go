package validation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// DefaultAntiSpoofThreshold is the minimum live-score to consider a frame genuine.
const DefaultAntiSpoofThreshold = 0.70

// LivenessResult is the outcome of anti-spoof (presentation attack detection) on a frame.
type LivenessResult struct {
	Live  bool
	Score float64
}

// LivenessChecker runs anti-spoof detection on a single image.
type LivenessChecker interface {
	Check(ctx context.Context, imageData []byte, referenceID string) (LivenessResult, error)
}

// StubLivenessChecker always reports live unless reference_id contains "/spoof/".
type StubLivenessChecker struct{}

func NewStubLivenessChecker() *StubLivenessChecker { return &StubLivenessChecker{} }

func (s *StubLivenessChecker) Check(_ context.Context, _ []byte, referenceID string) (LivenessResult, error) {
	if strings.Contains(referenceID, "/spoof/") {
		return LivenessResult{Live: false, Score: 0}, nil
	}
	return LivenessResult{Live: true, Score: 1.0}, nil
}

// HTTPAntiSpoofChecker POSTs raw bytes to the antispoof (MiniFASNet) sidecar.
type HTTPAntiSpoofChecker struct {
	endpoint   string
	threshold  float64
	httpClient *http.Client
}

func NewHTTPAntiSpoofChecker(endpoint string, threshold float64) *HTTPAntiSpoofChecker {
	if threshold <= 0 {
		threshold = DefaultAntiSpoofThreshold
	}
	return &HTTPAntiSpoofChecker{
		endpoint:   strings.TrimRight(endpoint, "/"),
		threshold:  threshold,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type antiSpoofResponse struct {
	Live  *bool    `json:"live"`
	Score *float64 `json:"score"`
}

func (c *HTTPAntiSpoofChecker) Check(ctx context.Context, imageData []byte, referenceID string) (LivenessResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/classify", bytes.NewReader(imageData))
	if err != nil {
		return LivenessResult{}, fmt.Errorf("antispoof: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return LivenessResult{}, fmt.Errorf("antispoof: call classifier: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return LivenessResult{}, fmt.Errorf("antispoof: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return LivenessResult{}, fmt.Errorf("antispoof: classifier status %d: %s", resp.StatusCode, string(body))
	}

	var parsed antiSpoofResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return LivenessResult{}, fmt.Errorf("antispoof: decode response: %w", err)
	}
	if parsed.Score == nil {
		return LivenessResult{}, fmt.Errorf("antispoof: classifier response missing score: %s", string(body))
	}

	score := *parsed.Score
	live := score >= c.threshold
	if parsed.Live != nil {
		live = *parsed.Live
	}

	slog.Info("antispoof: result",
		"reference_id", referenceID,
		"score", score,
		"live", live,
		"threshold", c.threshold,
	)
	return LivenessResult{Live: live, Score: score}, nil
}
