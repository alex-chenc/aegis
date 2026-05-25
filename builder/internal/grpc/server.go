package grpc

import (
	"context"
	"fmt"

	"builder/internal/service"
	pb "builder/pkg/api/v1"
)

type BuilderGRPCServer struct {
	pb.UnimplementedBuilderServiceServer
	service *service.BuilderService
}

func NewBuilderGRPCServer(svc *service.BuilderService) *BuilderGRPCServer {
	return &BuilderGRPCServer{service: svc}
}

func (s *BuilderGRPCServer) GetBuilderInfo(ctx context.Context, req *pb.GetBuilderInfoRequest) (*pb.GetBuilderInfoResponse, error) {
	info, err := s.service.GetBuilderInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("get builder info: %w", err)
	}
	return &pb.GetBuilderInfoResponse{
		BuilderVersion:        info.BuilderVersion,
		BuilderImage:          info.BuilderImage,
		ClangVersion:          info.ClangVersion,
		BpftoolVersion:        info.BPFToolVersion,
		SupportedArches:       info.SupportedArches,
		SupportedTransports:   info.SupportedTransports,
		SigningKeyFingerprint: info.SigningPublicKeyFingerprint,
		BuilderImageDigest:    info.BuilderImageDigest,
		LlvmVersion:           info.LlvmVersion,
		LibbpfVersion:         info.LibbpfVersion,
	}, nil
}

func (s *BuilderGRPCServer) StartBuild(ctx context.Context, req *pb.StartBuildRequest) (*pb.StartBuildResponse, error) {
	result, err := s.service.StartBuild(ctx, service.BuildRequest{
		BuildID:             req.BuildId,
		PackageID:           req.PackageId,
		Version:             req.Version,
		Title:               req.Title,
		CVEIDs:              req.CveIds,
		Operator:            req.Operator,
		BuilderProfile:      req.BuilderProfile,
		TargetArch:          req.TargetArch,
		HookPlanYAML:        req.HookPlanYaml,
		EBPFSource:          req.EbpfSource,
		SigmaRulesYAML:      req.SigmaRulesYaml,
		CorrelationYAML:     req.CorrelationYaml,
		PackageMetadataJSON: req.PackageMetadataJson,
	})
	if err != nil {
		return nil, fmt.Errorf("start build: %w", err)
	}

	artifacts := make([]*pb.BuildArtifact, len(result.Artifacts))
	for i, a := range result.Artifacts {
		artifacts[i] = &pb.BuildArtifact{
			Name:      a.Name,
			Transport: a.Transport,
			ObjectKey: a.ObjectKey,
			Sha256:    a.SHA256,
			Size:      a.Size,
		}
	}

	hooks := make([]*pb.HookSummary, len(result.HookSummary))
	for i, h := range result.HookSummary {
		hooks[i] = &pb.HookSummary{
			HookType:       h.HookType,
			AttachPoint:    h.AttachPoint,
			ProgramSection: h.ProgramSection,
			RiskLevel:      h.RiskLevel,
		}
	}

	return &pb.StartBuildResponse{
		BuildId:                  result.BuildID,
		Status:                   result.Status,
		ErrorMessage:             result.ErrorMessage,
		BuilderImageDigest:       result.BuilderImageDigest,
		ClangVersion:             result.ClangVersion,
		BuildLogObjectKey:        result.BuildLogObjectKey,
		BuildLogTail:             result.BuildLogTail,
		Artifacts:                artifacts,
		HookSummary:              hooks,
		EventSchemaJson:          result.EventSchemaJSON,
		UnsignedPackageObjectKey: result.UnsignedPackageObjectKey,
		UnsignedPackageSha256:    result.UnsignedPackageSHA256,
		UnsignedPackageSize:      result.UnsignedPackageSize,
	}, nil
}

func (s *BuilderGRPCServer) GetPackageBuildStatus(ctx context.Context, req *pb.GetPackageBuildStatusRequest) (*pb.GetPackageBuildStatusResponse, error) {
	info, err := s.service.GetPackageBuildStatus(ctx, req.PackageId, req.Version, req.BuildId)
	if err != nil {
		return nil, fmt.Errorf("get package build status: %w", err)
	}
	return &pb.GetPackageBuildStatusResponse{
		BuildId:         info.BuildID,
		Status:          info.Status,
		ErrorMessage:    info.ErrorMessage,
		ProgressPercent: info.ProgressPercent,
	}, nil
}

func (s *BuilderGRPCServer) ReviewBuild(ctx context.Context, req *pb.ReviewBuildRequest) (*pb.ReviewBuildResponse, error) {
	err := s.service.ReviewBuild(ctx, req.BuildId, req.PackageId, req.Version, req.Approved, req.Comment, req.Reviewer)
	if err != nil {
		return &pb.ReviewBuildResponse{
			Success:   false,
			Message:   err.Error(),
			NewStatus: "",
		}, nil
	}

	newStatus := "rejected"
	if req.Approved {
		newStatus = "success"
	}

	return &pb.ReviewBuildResponse{
		Success:   true,
		Message:   "review processed",
		NewStatus: newStatus,
	}, nil
}

func (s *BuilderGRPCServer) SignPackage(ctx context.Context, req *pb.SignPackageRequest) (*pb.SignPackageResponse, error) {
	result, err := s.service.SignPackage(ctx, service.SignRequest{
		BuildID:   req.BuildId,
		PackageID: req.PackageId,
		Version:   req.Version,
		Operator:  req.Operator,
		Confirm:   req.Confirm,
	})
	if err != nil {
		return nil, fmt.Errorf("sign package: %w", err)
	}
	return &pb.SignPackageResponse{
		Success:               result.Success,
		Message:               result.Message,
		PackageObjectKey:      result.PackageObjectKey,
		SignatureObjectKey:    result.SignatureObjectKey,
		PackageSha256:         result.PackageSHA256,
		PackageSize:           result.PackageSize,
		SignatureAlgorithm:    result.SignatureAlgorithm,
		SigningKeyFingerprint: result.SigningKeyFingerprint,
		SignedAt:              result.SignedAt,
	}, nil
}
