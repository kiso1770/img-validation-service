package validation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/image/webp"
)

const (
	PurposeProfilePhoto = "profile_photo"
	DefaultThreshold    = 0.85
	ReasonNSFW          = "nsfw"
	ReasonUnsupported   = "unsupported_format"
	ReasonTooLarge      = "too_large"
)

func init() {
	image.RegisterFormat("webp", "RIFF????WEBPVP8", webp.Decode, webp.DecodeConfig)
}

// Result is the outcome of validating an image.
type Result struct {
	Passed           bool
	NSFWScore        float64
	FormatValid      bool
	DetectedMIMEType string
	Width            int32
	Height           int32
	SizeBytes        int64
	RejectionReasons []string
}

// Validator validates uploaded image bytes.
type Validator interface {
	Validate(ctx context.Context, imageData []byte, contentTypeHint, purpose, referenceID string) (*Result, error)
}

// NSFWChecker scores image bytes for NSFW content.
type NSFWChecker interface {
	Score(ctx context.Context, imageData []byte, referenceID string) (score float64, err error)
}

// StubChecker passes all images unless reference_id contains "/reject/".
type StubChecker struct{}

func NewStubChecker() *StubChecker {
	return &StubChecker{}
}

func (s *StubChecker) Score(_ context.Context, _ []byte, referenceID string) (float64, error) {
	if strings.Contains(referenceID, "/reject/") {
		slog.Info("nsfw: stub rejected", "reference_id", referenceID)
		return 1.0, nil
	}
	return 0.0, nil
}

// HTTPChecker POSTs bytes to an OpenNSFW2 sidecar /classify endpoint.
type HTTPChecker struct {
	endpoint   string
	threshold  float64
	httpClient *http.Client
}

func NewHTTPChecker(endpoint string, threshold float64) *HTTPChecker {
	if threshold <= 0 {
		threshold = DefaultThreshold
	}
	return &HTTPChecker{
		endpoint:   strings.TrimRight(endpoint, "/"),
		threshold:  threshold,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type nsfwResponse struct {
	Score     *float64 `json:"score"`
	NSFWScore *float64 `json:"nsfw_score"`
	Unsafe    *float64 `json:"unsafe"`
}

func (r nsfwResponse) value() (float64, bool) {
	switch {
	case r.Score != nil:
		return *r.Score, true
	case r.NSFWScore != nil:
		return *r.NSFWScore, true
	case r.Unsafe != nil:
		return *r.Unsafe, true
	default:
		return 0, false
	}
}

func (c *HTTPChecker) Score(ctx context.Context, imageData []byte, referenceID string) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+"/classify", bytes.NewReader(imageData))
	if err != nil {
		return 0, fmt.Errorf("nsfw: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("nsfw: call classifier: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return 0, fmt.Errorf("nsfw: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("nsfw: classifier status %d: %s", resp.StatusCode, string(body))
	}

	var parsed nsfwResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, fmt.Errorf("nsfw: decode response: %w", err)
	}
	score, ok := parsed.value()
	if !ok {
		return 0, fmt.Errorf("nsfw: classifier response missing score: %s", string(body))
	}

	slog.Info("nsfw: classifier result",
		"reference_id", referenceID,
		"score", score,
		"threshold", c.threshold,
	)
	return score, nil
}

// NeedsNSFW reports whether NSFW moderation applies to the upload purpose.
func NeedsNSFW(purpose string) bool {
	switch strings.TrimSpace(purpose) {
	case PurposeProfilePhoto, "selfie", "chat_media":
		return true
	default:
		return false
	}
}

type validator struct {
	nsfwChecker       NSFWChecker
	nsfwThreshold     float64
	maxImageSizeBytes int64
}

// NewValidator creates the image validation orchestrator.
func NewValidator(nsfwChecker NSFWChecker, nsfwThreshold float64, maxImageSizeBytes int64) Validator {
	if nsfwChecker == nil {
		nsfwChecker = NewStubChecker()
	}
	if nsfwThreshold <= 0 {
		nsfwThreshold = DefaultThreshold
	}
	if maxImageSizeBytes <= 0 {
		maxImageSizeBytes = 10 * 1024 * 1024
	}
	return &validator{
		nsfwChecker:       nsfwChecker,
		nsfwThreshold:     nsfwThreshold,
		maxImageSizeBytes: maxImageSizeBytes,
	}
}

func (v *validator) Validate(
	ctx context.Context,
	imageData []byte,
	contentTypeHint, purpose, referenceID string,
) (*Result, error) {
	size := int64(len(imageData))
	result := &Result{SizeBytes: size}

	if size == 0 {
		return nil, fmt.Errorf("validation: empty image payload")
	}

	if size > v.maxImageSizeBytes {
		result.Passed = false
		result.FormatValid = false
		result.RejectionReasons = []string{ReasonTooLarge}
		return result, nil
	}

	_, width, height, mimeType, err := inspectFormat(imageData, contentTypeHint)
	if err != nil {
		result.Passed = false
		result.FormatValid = false
		result.RejectionReasons = []string{ReasonUnsupported}
		return result, nil
	}

	result.FormatValid = true
	result.DetectedMIMEType = mimeType
	result.Width = width
	result.Height = height

	if !NeedsNSFW(purpose) {
		result.Passed = true
		return result, nil
	}

	score, err := v.nsfwChecker.Score(ctx, imageData, referenceID)
	if err != nil {
		return nil, err
	}
	result.NSFWScore = score

	if score >= v.nsfwThreshold {
		result.Passed = false
		result.RejectionReasons = []string{ReasonNSFW}
		return result, nil
	}

	result.Passed = true
	return result, nil
}

func inspectFormat(data []byte, hint string) (format string, width, height int32, mime string, err error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", 0, 0, "", err
	}
	switch format {
	case "jpeg", "png", "webp":
		mime = mimeForFormat(format, hint)
		return format, int32(cfg.Width), int32(cfg.Height), mime, nil
	default:
		return format, 0, 0, "", fmt.Errorf("unsupported format %q", format)
	}
}

func mimeForFormat(format, hint string) string {
	switch format {
	case "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	default:
		if strings.TrimSpace(hint) != "" {
			return hint
		}
		return "application/octet-stream"
	}
}

// PingSidecar checks whether the NSFW sidecar health endpoint responds.
func PingSidecar(ctx context.Context, endpoint string) error {
	endpoint = strings.TrimRight(endpoint, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/health/model", nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sidecar health status %d", resp.StatusCode)
	}
	return nil
}
