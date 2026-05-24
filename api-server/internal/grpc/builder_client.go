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
func (c *BuilderClient) StartBuild(ctx context.Context, req interface{}) (interface{}, error) {
	pbReq := &pb.StartBuildRequest{}
	switch r := req.(type) {
	case *BuilderStartBuildRequest:
		pbReq = &pb.StartBuildRequest{
			BuildId: r.BuildID, PackageId: r.PackageID, Version: r.Version,
			Title: r.Title, CveIds: r.CVEIDs, Operator: r.Operator,
			TargetArch: r.TargetArch, HookPlanYaml: r.HookPlanYAML,
			EbpfSource: r.EBPFSource, SigmaRulesYaml: r.SigmaRulesYAML,
			CorrelationYaml: r.CorrelationYAML, PackageMetadataJson: r.PackageMetadataJSON,
		}
	case map[string]interface{}:
		pbReq = &pb.StartBuildRequest{
			BuildId:             mapStr(r, "BuildID"),
			PackageId:           mapStr(r, "PackageID"),
			Version:             mapStr(r, "Version"),
			Title:               mapStr(r, "Title"),
			Operator:            mapStr(r, "Operator"),
			TargetArch:          mapStr(r, "TargetArch"),
			HookPlanYaml:        mapStr(r, "HookPlanYAML"),
			EbpfSource:          mapStr(r, "EBPFSource"),
			SigmaRulesYaml:      mapStr(r, "SigmaRulesYAML"),
			CorrelationYaml:     mapStr(r, "CorrelationYAML"),
			PackageMetadataJson: mapStr(r, "PackageMetadataJSON"),
		}
	default:
		return nil, fmt.Errorf("invalid request type: %T", req)
	}

	resp, err := c.client.StartBuild(ctx, pbReq)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"BuildID":                  resp.BuildId,
		"Status":                   resp.Status,
		"ErrorMessage":             resp.ErrorMessage,
		"BuilderImageDigest":       resp.BuilderImageDigest,
		"ClangVersion":             resp.ClangVersion,
		"BuildLogObjectKey":        resp.BuildLogObjectKey,
		"BuildLogTail":             resp.BuildLogTail,
		"UnsignedPackageObjectKey": resp.UnsignedPackageObjectKey,
		"UnsignedPackageSHA256":    resp.UnsignedPackageSha256,
		"UnsignedPackageSize":      resp.UnsignedPackageSize,
	}, nil
}

// SignPackage sends a sign request to the builder service.
func (c *BuilderClient) SignPackage(ctx context.Context, req interface{}) (interface{}, error) {
	pbReq := &pb.SignPackageRequest{}
	switch r := req.(type) {
	case *BuilderSignRequest:
		pbReq = &pb.SignPackageRequest{
			BuildId: r.BuildID, PackageId: r.PackageID, Version: r.Version,
			Operator: r.Operator, Confirm: r.Confirm,
		}
	case map[string]interface{}:
		confirm, _ := r["Confirm"].(bool)
		pbReq = &pb.SignPackageRequest{
			BuildId: mapStr(r, "BuildID"), PackageId: mapStr(r, "PackageID"),
			Version: mapStr(r, "Version"), Operator: mapStr(r, "Operator"),
			Confirm: confirm,
		}
	default:
		return nil, fmt.Errorf("invalid request type: %T", req)
	}

	resp, err := c.client.SignPackage(ctx, pbReq)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"Success":               resp.Success,
		"Message":               resp.Message,
		"PackageObjectKey":      resp.PackageObjectKey,
		"SignatureObjectKey":    resp.SignatureObjectKey,
		"PackageSHA256":         resp.PackageSha256,
		"PackageSize":           resp.PackageSize,
		"SignatureAlgorithm":    resp.SignatureAlgorithm,
		"SigningKeyFingerprint": resp.SigningKeyFingerprint,
		"SignedAt":              resp.SignedAt,
	}, nil
}

func mapStr(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
