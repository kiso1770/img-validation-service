package grpcserver_test

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	grpcserver "img-validation-service/internal/grpc"
	imgvalidationv1 "img-validation-service/internal/grpc/pb/imgvalidation/v1"
	"img-validation-service/internal/validation"
)

const bufSize = 1024 * 1024

// failIfCalledFaceMatcher fails the test if the sidecar is ever invoked; it is used to
// prove that oversized MatchFaces payloads are rejected before reaching the matcher.
type failIfCalledFaceMatcher struct {
	t *testing.T
}

func (f *failIfCalledFaceMatcher) Match(_ context.Context, _, _ []byte, _ string) (validation.MatchResult, error) {
	f.t.Fatal("face matcher should not be called for an oversized payload")
	return validation.MatchResult{}, nil
}

// failIfCalledLivenessChecker fails the test if the sidecar is ever invoked; it is used to
// prove that oversized CheckLiveness payloads are rejected before reaching the checker.
type failIfCalledLivenessChecker struct {
	t *testing.T
}

func (f *failIfCalledLivenessChecker) Check(_ context.Context, _ []byte, _ string) (validation.LivenessResult, error) {
	f.t.Fatal("liveness checker should not be called for an oversized payload")
	return validation.LivenessResult{}, nil
}

// startTestServerWithLimit wires a server whose face matcher and liveness checker fail the
// test if invoked, with a small max image size so oversized-payload tests don't need to
// allocate large buffers.
func startTestServerWithLimit(t *testing.T, maxImageSizeBytes int64) (*grpc.ClientConn, func()) {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	s := grpc.NewServer()
	v := validation.NewValidator(validation.NewStubChecker(), 0.85, 10<<20)
	srv := grpcserver.NewServerWithFaceAndLimit(
		v,
		&failIfCalledFaceMatcher{t: t},
		&failIfCalledLivenessChecker{t: t},
		maxImageSizeBytes,
	)
	imgvalidationv1.RegisterImageValidationServiceServer(s, srv)

	go func() {
		if err := s.Serve(lis); err != nil {
			t.Logf("server exited: %v", err)
		}
	}()

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	cleanup := func() {
		conn.Close()
		s.Stop()
	}
	return conn, cleanup
}

func startTestServer(t *testing.T) (*grpc.ClientConn, func()) {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	s := grpc.NewServer()
	v := validation.NewValidator(validation.NewStubChecker(), 0.85, 10<<20)
	imgvalidationv1.RegisterImageValidationServiceServer(s, grpcserver.NewServer(v))

	go func() {
		if err := s.Serve(lis); err != nil {
			t.Logf("server exited: %v", err)
		}
	}()

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	cleanup := func() {
		conn.Close()
		s.Stop()
	}
	return conn, cleanup
}

func TestValidateImageGRPC(t *testing.T) {
	t.Parallel()

	conn, cleanup := startTestServer(t)
	defer cleanup()

	client := imgvalidationv1.NewImageValidationServiceClient(conn)
	resp, err := client.ValidateImage(context.Background(), &imgvalidationv1.ValidateImageRequest{
		ImageData:       []byte("not-an-image"),
		ContentTypeHint: "image/png",
		Purpose:         "profile_photo",
		ReferenceId:     "test-ref",
	})
	if err != nil {
		t.Fatalf("rpc error: %v", err)
	}
	if resp.GetPassed() {
		t.Fatal("expected business reject for invalid format")
	}
	if resp.GetRejectionReasons()[0] != validation.ReasonUnsupported {
		t.Fatalf("expected unsupported_format, got %v", resp.GetRejectionReasons())
	}
}

func TestValidateImageGRPC_EmptyPayload(t *testing.T) {
	t.Parallel()

	conn, cleanup := startTestServer(t)
	defer cleanup()

	client := imgvalidationv1.NewImageValidationServiceClient(conn)
	_, err := client.ValidateImage(context.Background(), &imgvalidationv1.ValidateImageRequest{})
	if err == nil {
		t.Fatal("expected invalid argument error")
	}
}

func TestMatchFacesGRPC(t *testing.T) {
	t.Parallel()

	conn, cleanup := startTestServer(t)
	defer cleanup()

	client := imgvalidationv1.NewImageValidationServiceClient(conn)
	resp, err := client.MatchFaces(context.Background(), &imgvalidationv1.MatchFacesRequest{
		SourceImage: []byte("selfie"),
		TargetImage: []byte("profile"),
		ReferenceId: "test-ref",
	})
	if err != nil {
		t.Fatalf("rpc error: %v", err)
	}
	if !resp.GetMatched() {
		t.Fatal("expected stub matcher to report matched=true")
	}
}

func TestMatchFacesGRPC_EmptyPayload(t *testing.T) {
	t.Parallel()

	conn, cleanup := startTestServer(t)
	defer cleanup()

	client := imgvalidationv1.NewImageValidationServiceClient(conn)
	_, err := client.MatchFaces(context.Background(), &imgvalidationv1.MatchFacesRequest{})
	if err == nil {
		t.Fatal("expected invalid argument error")
	}
}

func TestCheckLivenessGRPC(t *testing.T) {
	t.Parallel()

	conn, cleanup := startTestServer(t)
	defer cleanup()

	client := imgvalidationv1.NewImageValidationServiceClient(conn)
	resp, err := client.CheckLiveness(context.Background(), &imgvalidationv1.CheckLivenessRequest{
		ImageData:   []byte("selfie"),
		ReferenceId: "test-ref",
	})
	if err != nil {
		t.Fatalf("rpc error: %v", err)
	}
	if !resp.GetLive() {
		t.Fatal("expected stub liveness checker to report live=true")
	}
}

func TestCheckLivenessGRPC_EmptyPayload(t *testing.T) {
	t.Parallel()

	conn, cleanup := startTestServer(t)
	defer cleanup()

	client := imgvalidationv1.NewImageValidationServiceClient(conn)
	_, err := client.CheckLiveness(context.Background(), &imgvalidationv1.CheckLivenessRequest{})
	if err == nil {
		t.Fatal("expected invalid argument error")
	}
}

func TestMatchFacesGRPC_OversizedSourceImageRejected(t *testing.T) {
	t.Parallel()

	const limit = 16
	conn, cleanup := startTestServerWithLimit(t, limit)
	defer cleanup()

	client := imgvalidationv1.NewImageValidationServiceClient(conn)
	_, err := client.MatchFaces(context.Background(), &imgvalidationv1.MatchFacesRequest{
		SourceImage: make([]byte, limit+1),
		TargetImage: []byte("profile"),
		ReferenceId: "test-ref",
	})
	if err == nil {
		t.Fatal("expected invalid argument error for oversized source_image")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected codes.InvalidArgument, got %v", status.Code(err))
	}
}

func TestMatchFacesGRPC_OversizedTargetImageRejected(t *testing.T) {
	t.Parallel()

	const limit = 16
	conn, cleanup := startTestServerWithLimit(t, limit)
	defer cleanup()

	client := imgvalidationv1.NewImageValidationServiceClient(conn)
	_, err := client.MatchFaces(context.Background(), &imgvalidationv1.MatchFacesRequest{
		SourceImage: []byte("selfie"),
		TargetImage: make([]byte, limit+1),
		ReferenceId: "test-ref",
	})
	if err == nil {
		t.Fatal("expected invalid argument error for oversized target_image")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected codes.InvalidArgument, got %v", status.Code(err))
	}
}

func TestCheckLivenessGRPC_OversizedImageRejected(t *testing.T) {
	t.Parallel()

	const limit = 16
	conn, cleanup := startTestServerWithLimit(t, limit)
	defer cleanup()

	client := imgvalidationv1.NewImageValidationServiceClient(conn)
	_, err := client.CheckLiveness(context.Background(), &imgvalidationv1.CheckLivenessRequest{
		ImageData:   make([]byte, limit+1),
		ReferenceId: "test-ref",
	})
	if err == nil {
		t.Fatal("expected invalid argument error for oversized image_data")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected codes.InvalidArgument, got %v", status.Code(err))
	}
}
