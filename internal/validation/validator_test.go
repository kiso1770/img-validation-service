package validation_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"img-validation-service/internal/validation"
)

func minimalPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func TestValidator_PassSafePNG(t *testing.T) {
	t.Parallel()

	v := validation.NewValidator(validation.NewStubChecker(), 0.85, 10<<20)
	result, err := v.Validate(context.Background(), minimalPNG(), "image/png", "profile_photo", "ref-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected pass, reasons=%v", result.RejectionReasons)
	}
	if !result.FormatValid {
		t.Fatal("expected format valid")
	}
}

func TestValidator_RejectTooLarge(t *testing.T) {
	t.Parallel()

	v := validation.NewValidator(validation.NewStubChecker(), 0.85, 16)
	result, err := v.Validate(context.Background(), minimalPNG(), "image/png", "profile_photo", "ref-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected reject for size")
	}
	if len(result.RejectionReasons) != 1 || result.RejectionReasons[0] != validation.ReasonTooLarge {
		t.Fatalf("expected too_large, got %v", result.RejectionReasons)
	}
}

func TestValidator_RejectUnsupportedFormat(t *testing.T) {
	t.Parallel()

	v := validation.NewValidator(validation.NewStubChecker(), 0.85, 10<<20)
	result, err := v.Validate(context.Background(), []byte("not-an-image"), "text/plain", "profile_photo", "ref-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected reject for format")
	}
	if result.RejectionReasons[0] != validation.ReasonUnsupported {
		t.Fatalf("expected unsupported_format, got %v", result.RejectionReasons)
	}
}

func TestValidator_StubRejectMarker(t *testing.T) {
	t.Parallel()

	v := validation.NewValidator(validation.NewStubChecker(), 0.85, 10<<20)
	result, err := v.Validate(context.Background(), minimalPNG(), "image/png", "profile_photo", "profile_photo/u/reject/uuid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected nsfw reject")
	}
	if result.RejectionReasons[0] != validation.ReasonNSFW {
		t.Fatalf("expected nsfw reason, got %v", result.RejectionReasons)
	}
}

func TestHTTPChecker_PassAndReject(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/classify" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, `{"score": 0.10}`)
	}))
	defer srv.Close()

	checker := validation.NewHTTPChecker(srv.URL, 0.85)
	v := validation.NewValidator(checker, 0.85, 10<<20)
	result, err := v.Validate(context.Background(), minimalPNG(), "image/png", "profile_photo", "ref")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Fatal("expected pass")
	}

	srvReject := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"nsfw_score": 0.92}`)
	}))
	defer srvReject.Close()

	checkerReject := validation.NewHTTPChecker(srvReject.URL, 0.85)
	vReject := validation.NewValidator(checkerReject, 0.85, 10<<20)
	result, err = vReject.Validate(context.Background(), minimalPNG(), "image/png", "profile_photo", "ref")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected reject")
	}
}

func TestNeedsNSFW(t *testing.T) {
	t.Parallel()

	if !validation.NeedsNSFW("profile_photo") {
		t.Fatal("profile_photo should need nsfw")
	}
	if validation.NeedsNSFW("unknown") {
		t.Fatal("unknown purpose should skip nsfw")
	}
}

func TestMinimalPNGFromBase64(t *testing.T) {
	t.Parallel()

	raw, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	v := validation.NewValidator(validation.NewStubChecker(), 0.85, 10<<20)
	result, err := v.Validate(context.Background(), raw, "image/png", "profile_photo", "ref")
	if err != nil || !result.Passed {
		t.Fatalf("expected pass for base64 png: err=%v result=%+v", err, result)
	}
}
