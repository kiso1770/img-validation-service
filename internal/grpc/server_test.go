package grpcserver_test

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	grpcserver "img-validation-service/internal/grpc"
	imgvalidationv1 "img-validation-service/internal/grpc/pb/imgvalidation/v1"
	"img-validation-service/internal/validation"
)

const bufSize = 1024 * 1024

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
