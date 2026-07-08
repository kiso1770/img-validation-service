package validation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultFaceMatchThreshold is the minimum cosine similarity (ArcFace embeddings,
	// scaled to [0,1] by the faceid sidecar) to consider two faces the same person.
	DefaultFaceMatchThreshold = 0.40
)

// DetectResult is the outcome of face detection on a single image.
type DetectResult struct {
	FaceCount  int32
	Confidence float64 // highest detection confidence among found faces
}

// MatchResult is the outcome of comparing faces in two images.
type MatchResult struct {
	Matched         bool
	Similarity      float64
	SourceFaceCount int32
	TargetFaceCount int32
}

// FaceDetector detects faces in a single image.
type FaceDetector interface {
	Detect(ctx context.Context, imageData []byte, referenceID string) (DetectResult, error)
}

// FaceMatcher compares faces across two images (e.g. selfie vs. profile photo).
type FaceMatcher interface {
	Match(ctx context.Context, sourceImage, targetImage []byte, referenceID string) (MatchResult, error)
}

// StubFaceDetector reports exactly one face for any non-empty image.
type StubFaceDetector struct{}

func NewStubFaceDetector() *StubFaceDetector { return &StubFaceDetector{} }

func (s *StubFaceDetector) Detect(_ context.Context, imageData []byte, _ string) (DetectResult, error) {
	if len(imageData) == 0 {
		return DetectResult{}, nil
	}
	return DetectResult{FaceCount: 1, Confidence: 1.0}, nil
}

// StubFaceMatcher always reports a match unless reference_id contains "/nomatch/".
type StubFaceMatcher struct{}

func NewStubFaceMatcher() *StubFaceMatcher { return &StubFaceMatcher{} }

func (s *StubFaceMatcher) Match(_ context.Context, _, _ []byte, referenceID string) (MatchResult, error) {
	if strings.Contains(referenceID, "/nomatch/") {
		return MatchResult{Matched: false, Similarity: 0, SourceFaceCount: 1, TargetFaceCount: 1}, nil
	}
	return MatchResult{Matched: true, Similarity: 1.0, SourceFaceCount: 1, TargetFaceCount: 1}, nil
}

// HTTPFaceClient calls the faceid sidecar (InsightFace) for detection and verification.
type HTTPFaceClient struct {
	endpoint       string
	matchThreshold float64
	httpClient     *http.Client
}

func NewHTTPFaceClient(endpoint string, matchThreshold float64) *HTTPFaceClient {
	if matchThreshold <= 0 {
		matchThreshold = DefaultFaceMatchThreshold
	}
	return &HTTPFaceClient{
		endpoint:       strings.TrimRight(endpoint, "/"),
		matchThreshold: matchThreshold,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
	}
}

type faceDetectResponse struct {
	Faces []struct {
		Score float64 `json:"score"`
	} `json:"faces"`
}

func (c *HTTPFaceClient) Detect(ctx context.Context, imageData []byte, referenceID string) (DetectResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/detect", bytes.NewReader(imageData))
	if err != nil {
		return DetectResult{}, fmt.Errorf("face: build detect request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return DetectResult{}, fmt.Errorf("face: call detect: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return DetectResult{}, fmt.Errorf("face: read detect response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return DetectResult{}, fmt.Errorf("face: detect status %d: %s", resp.StatusCode, string(body))
	}

	var parsed faceDetectResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return DetectResult{}, fmt.Errorf("face: decode detect response: %w", err)
	}

	result := DetectResult{FaceCount: int32(len(parsed.Faces))}
	for _, f := range parsed.Faces {
		if f.Score > result.Confidence {
			result.Confidence = f.Score
		}
	}

	slog.Info("face: detect result", "reference_id", referenceID, "face_count", result.FaceCount)
	return result, nil
}

type faceVerifyResponse struct {
	Similarity      float64 `json:"similarity"`
	SourceFaceCount int32   `json:"source_face_count"`
	TargetFaceCount int32   `json:"target_face_count"`
}

func (c *HTTPFaceClient) Match(ctx context.Context, sourceImage, targetImage []byte, referenceID string) (MatchResult, error) {
	body, err := json.Marshal(map[string]string{
		"source_image": base64.StdEncoding.EncodeToString(sourceImage),
		"target_image": base64.StdEncoding.EncodeToString(targetImage),
	})
	if err != nil {
		return MatchResult{}, fmt.Errorf("face: marshal verify request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/verify", bytes.NewReader(body))
	if err != nil {
		return MatchResult{}, fmt.Errorf("face: build verify request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return MatchResult{}, fmt.Errorf("face: call verify: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return MatchResult{}, fmt.Errorf("face: read verify response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return MatchResult{}, fmt.Errorf("face: verify status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed faceVerifyResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return MatchResult{}, fmt.Errorf("face: decode verify response: %w", err)
	}

	result := MatchResult{
		Similarity:      parsed.Similarity,
		SourceFaceCount: parsed.SourceFaceCount,
		TargetFaceCount: parsed.TargetFaceCount,
		Matched:         parsed.Similarity >= c.matchThreshold,
	}

	slog.Info("face: verify result",
		"reference_id", referenceID,
		"similarity", result.Similarity,
		"matched", result.Matched,
		"threshold", c.matchThreshold,
	)
	return result, nil
}
