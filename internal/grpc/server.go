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
	validator       validation.Validator
	faceMatcher     validation.FaceMatcher
	livenessChecker validation.LivenessChecker
}

// NewServer wires the validator with stub face/liveness checkers (no rejection).
// Use NewServerWithFace to enable real face match and anti-spoof checks.
func NewServer(v validation.Validator) *Server {
	return NewServerWithFace(v, validation.NewStubFaceMatcher(), validation.NewStubLivenessChecker())
}

// NewServerWithFace wires the validator together with face match and liveness checkers.
func NewServerWithFace(v validation.Validator, faceMatcher validation.FaceMatcher, livenessChecker validation.LivenessChecker) *Server {
	return &Server{validator: v, faceMatcher: faceMatcher, livenessChecker: livenessChecker}
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
		FaceCount:        result.FaceCount,
		FaceConfidence:   result.FaceConfidence,
	}, nil
}

func (s *Server) MatchFaces(
	ctx context.Context,
	req *imgvalidationv1.MatchFacesRequest,
) (*imgvalidationv1.MatchFacesResponse, error) {
	if req == nil || len(req.GetSourceImage()) == 0 || len(req.GetTargetImage()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "source_image and target_image are required")
	}

	result, err := s.faceMatcher.Match(ctx, req.GetSourceImage(), req.GetTargetImage(), req.GetReferenceId())
	if err != nil {
		slog.Error("match faces failed", "error", err, "reference_id", req.GetReferenceId())
		return nil, status.Errorf(codes.Unavailable, "face match failed: %v", err)
	}

	return &imgvalidationv1.MatchFacesResponse{
		Matched:         result.Matched,
		Similarity:      result.Similarity,
		SourceFaceCount: result.SourceFaceCount,
		TargetFaceCount: result.TargetFaceCount,
	}, nil
}

func (s *Server) CheckLiveness(
	ctx context.Context,
	req *imgvalidationv1.CheckLivenessRequest,
) (*imgvalidationv1.CheckLivenessResponse, error) {
	if req == nil || len(req.GetImageData()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "image_data is required")
	}

	result, err := s.livenessChecker.Check(ctx, req.GetImageData(), req.GetReferenceId())
	if err != nil {
		slog.Error("check liveness failed", "error", err, "reference_id", req.GetReferenceId())
		return nil, status.Errorf(codes.Unavailable, "liveness check failed: %v", err)
	}

	return &imgvalidationv1.CheckLivenessResponse{
		Live:  result.Live,
		Score: result.Score,
	}, nil
}
