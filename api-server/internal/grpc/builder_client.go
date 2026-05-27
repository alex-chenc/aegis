package grpc

import (
	"context"
	"fmt"
	"time"

	pb "api-server/pkg/api/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// BuilderClient is the gRPC client for communicating with the Builder service
type BuilderClient struct {
	conn   *grpc.ClientConn
	client pb.BuilderServiceClient
}

// BuilderStartBuildRequest is the service-layer build request
type BuilderStartBuildRequest struct {
	BuildID             string
	PackageID           string
	Version             string
	Title               string
	CVEIDs              []string
	Operator            string
	BuilderProfile      string
	TargetArch          string
	HookPlanYAML        string
	EBPFSource          string
	SigmaRulesYAML      string
	CorrelationYAML     string
	PackageMetadataJSON string
}

// BuilderStartBuildResponse is the service-layer build response
type BuilderStartBuildResponse struct {
	BuildID                  string
	Status                   string
	ErrorMessage             string
	BuilderImageDigest       string
	ClangVersion             string
	BuildLogObjectKey        string
	BuildLogTail             string
	UnsignedPackageObjectKey string
	UnsignedPackageSHA256    string
	UnsignedPackageSize      int64
}

// BuilderSignRequest is the service-layer sign request
type BuilderSignRequest struct {
	BuildID   string
	PackageID string
	Version   string
	Operator  string
	Confirm   bool
}

// BuilderSignResponse is the service-layer sign response
type BuilderSignResponse struct {
	Success               bool
	Message               string
	PackageObjectKey      string
	SignatureObjectKey    string
	PackageSHA256         string
	PackageSize           int64
	SignatureAlgorithm    string
	SigningKeyFingerprint string
	SignedAt              int64
}

// NewBuilderClient creates a new gRPC client connection to the Builder service
func NewBuilderClient(address string) (*BuilderClient, error) {
	kaParams := keepalive.ClientParameters{
		Time:                10 * time.Second,
		Timeout:             3 * time.Second,
		PermitWithoutStream: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(kaParams),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to builder gRPC: %w", err)
	}

	return &BuilderClient{
		conn:   conn,
		client: pb.NewBuilderServiceClient(conn),
	}, nil
}

// Close closes the gRPC connection
func (c *BuilderClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// StartBuild sends a build request to the builder service.
func (c *BuilderClient) StartBuild(ctx context.Context, req *BuilderStartBuildRequest) (*BuilderStartBuildResponse, error) {
	pbReq := &pb.StartBuildRequest{
		BuildId:             req.BuildID,
		PackageId:           req.PackageID,
		Version:             req.Version,
		Title:               req.Title,
		CveIds:              req.CVEIDs,
		Operator:            req.Operator,
		BuilderProfile:      req.BuilderProfile,
		TargetArch:          req.TargetArch,
		HookPlanYaml:        req.HookPlanYAML,
		EbpfSource:          req.EBPFSource,
		SigmaRulesYaml:      req.SigmaRulesYAML,
		CorrelationYaml:     req.CorrelationYAML,
		PackageMetadataJson: req.PackageMetadataJSON,
	}

	resp, err := c.client.StartBuild(ctx, pbReq)
	if err != nil {
		return nil, err
	}
	return &BuilderStartBuildResponse{
		BuildID:                  resp.BuildId,
		Status:                   resp.Status,
		ErrorMessage:             resp.ErrorMessage,
		BuilderImageDigest:       resp.BuilderImageDigest,
		ClangVersion:             resp.ClangVersion,
		BuildLogObjectKey:        resp.BuildLogObjectKey,
		BuildLogTail:             resp.BuildLogTail,
		UnsignedPackageObjectKey: resp.UnsignedPackageObjectKey,
		UnsignedPackageSHA256:    resp.UnsignedPackageSha256,
		UnsignedPackageSize:      resp.UnsignedPackageSize,
	}, nil
}

// SignPackage sends a sign request to the builder service.
func (c *BuilderClient) SignPackage(ctx context.Context, req *BuilderSignRequest) (*BuilderSignResponse, error) {
	pbReq := &pb.SignPackageRequest{
		BuildId:   req.BuildID,
		PackageId: req.PackageID,
		Version:   req.Version,
		Operator:  req.Operator,
		Confirm:   req.Confirm,
	}

	resp, err := c.client.SignPackage(ctx, pbReq)
	if err != nil {
		return nil, err
	}
	return &BuilderSignResponse{
		Success:               resp.Success,
		Message:               resp.Message,
		PackageObjectKey:      resp.PackageObjectKey,
		SignatureObjectKey:    resp.SignatureObjectKey,
		PackageSHA256:         resp.PackageSha256,
		PackageSize:           resp.PackageSize,
		SignatureAlgorithm:    resp.SignatureAlgorithm,
		SigningKeyFingerprint: resp.SigningKeyFingerprint,
		SignedAt:              resp.SignedAt,
	}, nil
}

// GetPackageBuildStatus queries the build status from the builder service.
func (c *BuilderClient) GetPackageBuildStatus(ctx context.Context, packageID, version, buildID string) (*pb.GetPackageBuildStatusResponse, error) {
	return c.client.GetPackageBuildStatus(ctx, &pb.GetPackageBuildStatusRequest{
		PackageId: packageID,
		Version:   version,
		BuildId:   buildID,
	})
}

// ReviewBuild submits a build review to the builder service.
func (c *BuilderClient) ReviewBuild(ctx context.Context, buildID, packageID, version string, approved bool, comment, reviewer string) (*pb.ReviewBuildResponse, error) {
	return c.client.ReviewBuild(ctx, &pb.ReviewBuildRequest{
		BuildId:   buildID,
		PackageId: packageID,
		Version:   version,
		Approved:  approved,
		Comment:   comment,
		Reviewer:  reviewer,
	})
}
