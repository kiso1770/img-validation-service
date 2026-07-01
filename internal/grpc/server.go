package grpcserver

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	imgvalidationv1 "img-validation-service/internal/grpc/pb/imgvalidation/v1"
	"img-validation-service/internal/validation"
)

// Server implements ImageValidationService.
type Server struct {
	imgvalidationv1.UnimplementedImageValidationServiceServer
	validator validation.Validator
}

func NewServer(v validation.Validator) *Server {
	return &Server{validator: v}
}

func (s *Server) ValidateImage(
	ctx context.Context,
	req *imgvalidationv1.ValidateImageRequest,
) (*imgvalidationv1.ValidateImageResponse, error) {
	if req == nil || len(req.GetImageData()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "image_data is required")
	}

	result, err := s.validator.Validate(
		ctx,
		req.GetImageData(),
		req.GetContentTypeHint(),
		req.GetPurpose(),
		req.GetReferenceId(),
	)
	if err != nil {
		slog.Error("validate image failed", "error", err, "reference_id", req.GetReferenceId())
		return nil, status.Errorf(codes.Unavailable, "validation failed: %v", err)
	}

	return &imgvalidationv1.ValidateImageResponse{
		Passed:           result.Passed,
		NsfwScore:        result.NSFWScore,
		FormatValid:      result.FormatValid,
		DetectedMimeType: result.DetectedMIMEType,
		Width:            result.Width,
		Height:           result.Height,
		SizeBytes:        result.SizeBytes,
		RejectionReasons: result.RejectionReasons,
	}, nil
}
