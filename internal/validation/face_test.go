package validation_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"img-validation-service/internal/validation"
)

func TestStubFaceDetector(t *testing.T) {
	t.Parallel()

	d := validation.NewStubFaceDetector()
	result, err := d.Detect(context.Background(), []byte("data"), "ref")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FaceCount != 1 {
		t.Fatalf("expected 1 face, got %d", result.FaceCount)
	}
}

func TestStubFaceMatcher(t *testing.T) {
	t.Parallel()

	m := validation.NewStubFaceMatcher()

	matched, err := m.Match(context.Background(), []byte("a"), []byte("b"), "ref-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matched.Matched {
		t.Fatal("expected match")
	}

	noMatch, err := m.Match(context.Background(), []byte("a"), []byte("b"), "user/1/nomatch/2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if noMatch.Matched {
		t.Fatal("expected no match for /nomatch/ reference_id")
	}
}

func TestHTTPFaceClient_Detect(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/detect" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"faces": []map[string]float64{{"score": 0.98}},
		})
	}))
	defer srv.Close()

	client := validation.NewHTTPFaceClient(srv.URL, 0.40)
	result, err := client.Detect(context.Background(), []byte("data"), "ref")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FaceCount != 1 {
		t.Fatalf("expected 1 face, got %d", result.FaceCount)
	}
	if result.Confidence != 0.98 {
		t.Fatalf("expected confidence 0.98, got %v", result.Confidence)
	}
}

func TestHTTPFaceClient_Match(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/verify" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, `{"similarity":0.75,"source_face_count":1,"target_face_count":1}`)
	}))
	defer srv.Close()

	client := validation.NewHTTPFaceClient(srv.URL, 0.40)
	result, err := client.Match(context.Background(), []byte("selfie"), []byte("profile"), "ref")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Matched {
		t.Fatal("expected matched=true for similarity above threshold")
	}
	if result.Similarity != 0.75 {
		t.Fatalf("expected similarity 0.75, got %v", result.Similarity)
	}
}

func TestHTTPFaceClient_Match_BelowThreshold(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"similarity":0.10,"source_face_count":1,"target_face_count":1}`)
	}))
	defer srv.Close()

	client := validation.NewHTTPFaceClient(srv.URL, 0.40)
	result, err := client.Match(context.Background(), []byte("selfie"), []byte("profile"), "ref")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Matched {
		t.Fatal("expected matched=false for similarity below threshold")
	}
}

func TestValidator_NoFaceRejection(t *testing.T) {
	t.Parallel()

	noFaceDetector := zeroFaceDetector{}
	v := validation.NewValidatorWithFace(validation.NewStubChecker(), 0.85, 10<<20, noFaceDetector)
	result, err := v.Validate(context.Background(), minimalPNG(), "image/png", "profile_photo", "ref")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected reject for no face")
	}
	if result.RejectionReasons[0] != validation.ReasonNoFace {
		t.Fatalf("expected no_face, got %v", result.RejectionReasons)
	}
}

func TestValidator_SelfieMultipleFacesRejection(t *testing.T) {
	t.Parallel()

	twoFaceDetector := fixedFaceDetector{count: 2}
	v := validation.NewValidatorWithFace(validation.NewStubChecker(), 0.85, 10<<20, twoFaceDetector)
	result, err := v.Validate(context.Background(), minimalPNG(), "image/png", "selfie", "ref")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected reject for multiple faces on selfie")
	}
	if result.RejectionReasons[0] != validation.ReasonMultipleFaces {
		t.Fatalf("expected multiple_faces, got %v", result.RejectionReasons)
	}
}

func TestValidator_ProfilePhotoRejectsMultipleFaces(t *testing.T) {
	t.Parallel()

	// Dating profile photos must show exactly one person, even on non-main
	// positions — group photos with friends aren't allowed.
	twoFaceDetector := fixedFaceDetector{count: 2}
	v := validation.NewValidatorWithFace(validation.NewStubChecker(), 0.85, 10<<20, twoFaceDetector)
	result, err := v.Validate(context.Background(), minimalPNG(), "image/png", "profile_photo", "ref")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected reject for multiple faces on profile_photo")
	}
	if result.RejectionReasons[0] != validation.ReasonMultipleFaces {
		t.Fatalf("expected multiple_faces, got %v", result.RejectionReasons)
	}
}

type zeroFaceDetector struct{}

func (zeroFaceDetector) Detect(context.Context, []byte, string) (validation.DetectResult, error) {
	return validation.DetectResult{FaceCount: 0}, nil
}

type fixedFaceDetector struct{ count int32 }

func (f fixedFaceDetector) Detect(context.Context, []byte, string) (validation.DetectResult, error) {
	return validation.DetectResult{FaceCount: f.count, Confidence: 0.9}, nil
}
