package validation_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"img-validation-service/internal/validation"
)

func TestStubLivenessChecker(t *testing.T) {
	t.Parallel()

	c := validation.NewStubLivenessChecker()

	live, err := c.Check(context.Background(), []byte("data"), "ref")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !live.Live {
		t.Fatal("expected live")
	}

	spoof, err := c.Check(context.Background(), []byte("data"), "user/1/spoof/2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spoof.Live {
		t.Fatal("expected spoof rejection")
	}
}

func TestHTTPAntiSpoofChecker(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/classify" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, `{"score": 0.92}`)
	}))
	defer srv.Close()

	c := validation.NewHTTPAntiSpoofChecker(srv.URL, 0.70)
	result, err := c.Check(context.Background(), []byte("data"), "ref")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Live {
		t.Fatal("expected live=true for score above threshold")
	}
	if result.Score != 0.92 {
		t.Fatalf("expected score 0.92, got %v", result.Score)
	}
}

func TestHTTPAntiSpoofChecker_BelowThreshold(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"score": 0.10}`)
	}))
	defer srv.Close()

	c := validation.NewHTTPAntiSpoofChecker(srv.URL, 0.70)
	result, err := c.Check(context.Background(), []byte("data"), "ref")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Live {
		t.Fatal("expected live=false for score below threshold")
	}
}
