package grpcserver

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"img-validation-service/internal/config"
	imgvalidationv1 "img-validation-service/internal/grpc/pb/imgvalidation/v1"
	"img-validation-service/internal/validation"
)

// Server implements ImageValidationService.
type Server struct {
	imgvalidationv1.UnimplementedImageValidationServiceServer
	validator         validation.Validator
	faceMatcher       validation.FaceMatcher
	livenessChecker   validation.LivenessChecker
	maxImageSizeBytes int64
}

// NewServer wires the validator with stub face/liveness checkers (no rejection).
// Use NewServerWithFace to enable real face match and anti-spoof checks.
func NewServer(v validation.Validator) *Server {
	return NewServerWithFace(v, validation.NewStubFaceMatcher(), validation.NewStubLivenessChecker())
}

// NewServerWithFace wires the validator together with face match and liveness checkers,
// using the default max image size limit.
func NewServerWithFace(v validation.Validator, faceMatcher validation.FaceMatcher, livenessChecker validation.LivenessChecker) *Server {
	return NewServerWithFaceAndLimit(v, faceMatcher, livenessChecker, config.DefaultMaxImageSize)
}

// NewServerWithFaceAndLimit wires the validator together with face match and liveness
// checkers, applying maxImageSizeBytes as the size cap enforced on MatchFaces and
// CheckLiveness payloads before they are forwarded to the sidecars. ValidateImage enforces
// its own limit via the validator. A non-positive maxImageSizeBytes falls back to the
// package default.
func NewServerWithFaceAndLimit(v validation.Validator, faceMatcher validation.FaceMatcher, livenessChecker validation.LivenessChecker, maxImageSizeBytes int64) *Server {
	if maxImageSizeBytes <= 0 {
		maxImageSizeBytes = config.DefaultMaxImageSize
	}
	return &Server{
		validator:         v,
		faceMatcher:       faceMatcher,
		livenessChecker:   livenessChecker,
		maxImageSizeBytes: maxImageSizeBytes,
	}
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
	if int64(len(req.GetSourceImage())) > s.maxImageSizeBytes {
		return nil, status.Errorf(codes.InvalidArgument, "source_image exceeds max size of %d bytes", s.maxImageSizeBytes)
	}
	if int64(len(req.GetTargetImage())) > s.maxImageSizeBytes {
		return nil, status.Errorf(codes.InvalidArgument, "target_image exceeds max size of %d bytes", s.maxImageSizeBytes)
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
	if int64(len(req.GetImageData())) > s.maxImageSizeBytes {
		return nil, status.Errorf(codes.InvalidArgument, "image_data exceeds max size of %d bytes", s.maxImageSizeBytes)
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
