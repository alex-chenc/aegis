package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

var (
	ErrMCPPlatformInvalidEndpoint      = errors.New("invalid remote MCP endpoint")
	ErrMCPPlatformUnsupportedTransport = errors.New("unsupported MCP transport")
	ErrMCPPlatformJobConflict          = errors.New("onboarding idempotency key conflicts with a different request")
	ErrMCPPlatformSelfApproval         = errors.New("request owner cannot approve its own MCP review")
	ErrMCPPlatformClientEndpointDenied = errors.New("mcp client endpoint access denied")
	ErrMCPPlatformToolNotAllowed       = errors.New("mcp tool is not allowed for this client")
	ErrMCPPlatformSecurityBlocked      = errors.New("mcp invocation blocked by security policy")
)

type MCPOnboardingRequest struct {
	DisplayName     string     `json:"display_name" binding:"required"`
	EndpointURL     string     `json:"endpoint_url" binding:"required"`
	AuthType        string     `json:"auth_type" binding:"required"`
	CredentialRef   string     `json:"credential_ref"`
	OwnerTeamID     *uuid.UUID `json:"owner_team_id"`
	Environment     string     `json:"environment" binding:"required"`
	TargetCatalogID *uuid.UUID `json:"target_catalog_id"`
	PublishPolicy   string     `json:"publish_policy" binding:"required"`
}

type MCPPlatformService struct {
	repo              *repository.MCPPlatformRepository
	logger            *zap.Logger
	client            *http.Client
	credentialBroker  func(context.Context, string) (string, error)
	catalogSigningKey []byte
}

func NewMCPPlatformService(repo *repository.MCPPlatformRepository, logger *zap.Logger) *MCPPlatformService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &MCPPlatformService{
		repo:   repo,
		logger: logger.Named("mcp_platform"),
		client: &http.Client{
			Timeout:       30 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
			Transport: &http.Transport{
				// Do not inherit process HTTP(S)_PROXY: a proxy would move the
				// connection target outside the SSRF dial guard below.
				Proxy:             nil,
				DialContext:       safeMCPDialContext,
				ForceAttemptHTTP2: true,
			},
		},
	}
}

// SetCredentialBroker injects the control-plane credential resolver. The
// resolver must return a short-lived secret for an opaque credential ref.
func (s *MCPPlatformService) SetCredentialBroker(resolve func(context.Context, string) (string, error)) {
	s.credentialBroker = resolve
}

func (s *MCPPlatformService) SetCatalogSigningKey(key string) {
	s.catalogSigningKey = []byte(strings.TrimSpace(key))
}

func (s *MCPPlatformService) CreateOnboardingJob(ctx context.Context, req MCPOnboardingRequest, idempotencyKey, operator string) (*model.MCPOnboardingJob, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return nil, errors.New("idempotency key is required")
	}
	if err := validateMCPOnboardingRequest(req); err != nil {
		return nil, err
	}
	if existing, err := s.repo.FindOnboardingByIdempotency(ctx, idempotencyKey); err == nil {
		if existing.DisplayName != req.DisplayName || existing.EndpointDisplay != safeEndpointDisplay(req.EndpointURL) || existing.Environment != req.Environment {
			return nil, ErrMCPPlatformJobConflict
		}
		return existing, nil
	} else if !errors.Is(err, repository.ErrMCPPlatformNotFound) {
		return nil, err
	}

	now := time.Now().UTC()
	job := &model.MCPOnboardingJob{
		ID: uuid.New(), IdempotencyKey: idempotencyKey, DisplayName: strings.TrimSpace(req.DisplayName),
		EndpointURL: strings.TrimSpace(req.EndpointURL), EndpointDisplay: safeEndpointDisplay(req.EndpointURL),
		CredentialRef: strings.TrimSpace(req.CredentialRef), AuthType: strings.TrimSpace(req.AuthType),
		OwnerTeamID: req.OwnerTeamID, OwnerUserID: operator, Environment: strings.TrimSpace(req.Environment),
		TargetCatalogID: req.TargetCatalogID, PublishPolicy: strings.TrimSpace(req.PublishPolicy),
		Status: model.MCPPlatformJobCreated, Step: model.MCPPlatformJobCreated, CreatedBy: operator,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.CreateOnboardingJob(ctx, job); err != nil {
		return nil, err
	}
	s.logger.Info("mcp_onboarding_started", zap.String("job_id", job.ID.String()), zap.String("operator", operator), zap.String("environment", job.Environment))
	go s.runOnboarding(context.Background(), job.ID)
	return job, nil
}

func (s *MCPPlatformService) RetryOnboardingJob(ctx context.Context, id uuid.UUID, operator string) (*model.MCPOnboardingJob, error) {
	job, err := s.repo.GetOnboardingJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if job.Status != model.MCPPlatformJobFailed && job.Status != model.MCPPlatformJobCancelled {
		return job, errors.New("only failed or cancelled onboarding jobs can be retried")
	}
	job.Status, job.Step, job.ErrorCode, job.ErrorMessage = model.MCPPlatformJobCreated, model.MCPPlatformJobCreated, "", ""
	job.Attempt++
	job.CreatedBy = operator
	job.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateOnboardingJob(ctx, job); err != nil {
		return nil, err
	}
	go s.runOnboarding(context.Background(), job.ID)
	return job, nil
}

func (s *MCPPlatformService) CancelOnboardingJob(ctx context.Context, id uuid.UUID, operator string) (*model.MCPOnboardingJob, error) {
	job, err := s.repo.GetOnboardingJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if job.Status == model.MCPPlatformJobActive || job.Status == model.MCPPlatformJobFailed || job.Status == model.MCPPlatformJobCancelled {
		return job, nil
	}
	job.Status, job.Step, job.ErrorCode, job.ErrorMessage = model.MCPPlatformJobCancelled, model.MCPPlatformJobCancelled, "cancelled_by_operator", "onboarding cancelled"
	job.CreatedBy = operator
	job.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateOnboardingJob(ctx, job); err != nil {
		return nil, err
	}
	s.logger.Info("mcp_onboarding_cancelled", zap.String("job_id", id.String()), zap.String("operator", operator))
	return job, nil
}

func (s *MCPPlatformService) runOnboarding(ctx context.Context, id uuid.UUID) {
	job, err := s.repo.GetOnboardingJob(ctx, id)
	if err != nil {
		s.logger.Error("mcp_onboarding_load_failed", zap.String("job_id", id.String()), zap.Error(err))
		return
	}
	setStatus := func(status, step string) bool {
		latest, loadErr := s.repo.GetOnboardingJob(ctx, id)
		if loadErr == nil && latest.Status == model.MCPPlatformJobCancelled {
			return false
		}
		job.Status, job.Step, job.UpdatedAt = status, step, time.Now().UTC()
		if err := s.repo.UpdateOnboardingJob(ctx, job); err != nil {
			s.logger.Error("mcp_onboarding_status_update_failed", zap.String("job_id", id.String()), zap.String("step", step), zap.Error(err))
			return false
		}
		return true
	}
	fail := func(code string, err error) {
		if latest, loadErr := s.repo.GetOnboardingJob(ctx, id); loadErr == nil && latest.Status == model.MCPPlatformJobCancelled {
			return
		}
		job.Status, job.Step, job.ErrorCode, job.ErrorMessage, job.UpdatedAt = model.MCPPlatformJobFailed, model.MCPPlatformJobFailed, code, safeMCPError(err), time.Now().UTC()
		_ = s.repo.UpdateOnboardingJob(ctx, job)
		s.logger.Warn("mcp_onboarding_failed", zap.String("job_id", id.String()), zap.String("error_code", code))
	}
	if !setStatus(model.MCPPlatformJobValidatingEndpoint, model.MCPPlatformJobValidatingEndpoint) {
		return
	}
	parsed, err := validateRemoteEndpoint(job.EndpointURL, job.Environment)
	if err != nil {
		fail("invalid_endpoint", err)
		return
	}
	if job.TargetCatalogID != nil {
		if _, err := s.repo.GetCatalog(ctx, *job.TargetCatalogID); err != nil {
			fail("catalog_not_found", err)
			return
		}
	}
	if job.Status == model.MCPPlatformJobCancelled {
		return
	}
	if job.AuthType == model.MCPPlatformAuthOAuth2 && strings.TrimSpace(job.CredentialRef) == "" {
		if !setStatus(model.MCPPlatformJobAwaitingAuth, model.MCPPlatformJobAwaitingAuth) {
			return
		}
		return
	}
	if !setStatus(model.MCPPlatformJobAuthenticating, model.MCPPlatformJobAuthenticating) {
		return
	}
	if err := validateAuthRef(job.AuthType, job.CredentialRef, job.Environment); err != nil {
		fail("invalid_auth", err)
		return
	}
	if !setStatus(model.MCPPlatformJobDiscovering, model.MCPPlatformJobDiscovering) {
		return
	}
	discovery, err := s.discover(ctx, parsed, job.AuthType, job.CredentialRef)
	if err != nil {
		fail("discovery_failed", err)
		return
	}
	if !setStatus(model.MCPPlatformJobValidatingTools, model.MCPPlatformJobValidatingTools) {
		return
	}
	tools, err := validateDiscoveredTools(discovery.Tools)
	if err != nil {
		fail("invalid_tools", err)
		return
	}
	if !setStatus(model.MCPPlatformJobSecurityScanning, model.MCPPlatformJobSecurityScanning) {
		return
	}
	if err := scanDiscoveredTools(tools); err != nil {
		fail("security_scan_failed", err)
		return
	}
	if !setStatus(model.MCPPlatformJobClassifying, model.MCPPlatformJobClassifying) {
		return
	}
	riskTier := classifyTools(tools)
	if !setStatus(model.MCPPlatformJobBuildingRelease, model.MCPPlatformJobBuildingRelease) {
		return
	}
	server := &model.MCPServer{
		ID: uuid.New(), ServerKey: "mcp_" + uuid.NewString()[:8], DisplayName: job.DisplayName,
		OwnerTeamID: job.OwnerTeamID, OwnerUserID: job.OwnerUserID, Environment: job.Environment,
		Transport: model.MCPPlatformTransportStreamableHTTP, EndpointURL: job.EndpointURL, EndpointDisplay: job.EndpointDisplay,
		CredentialRef: job.CredentialRef, AuthType: job.AuthType, ProtocolVersion: discovery.ProtocolVersion,
		RiskTier: riskTier, LifecycleStatus: model.MCPPlatformServerDraft, ToolCount: len(tools), CreatedBy: job.CreatedBy, UpdatedBy: job.CreatedBy,
		LastHealthStatus: "healthy", LastSyncedAt: mcpPtrTime(time.Now().UTC()),
	}
	if err := s.repo.CreateServer(ctx, server); err != nil {
		fail("server_persist_failed", err)
		return
	}
	serverRevision := &model.MCPServerRevision{
		ID: uuid.New(), ServerID: server.ID, RevisionNo: 1, ProtocolVersion: discovery.ProtocolVersion,
		Capabilities: datatypes.JSON(discovery.Capabilities), ToolsSnapshot: datatypes.JSON(discovery.RawTools),
		Digest: digestJSON(discovery.RawTools), Status: model.MCPPlatformServerApproved, CreatedBy: job.CreatedBy,
	}
	if err := s.repo.CreateServerRevision(ctx, serverRevision); err != nil {
		fail("revision_persist_failed", err)
		return
	}
	shouldAutoPublish := job.PublishPolicy == "auto_if_l1" && riskTier == model.MCPPlatformRiskL1
	toolRows := make([]model.MCPToolRevision, 0, len(tools))
	for _, tool := range tools {
		toolRows = append(toolRows, model.MCPToolRevision{ID: uuid.New(), ServerRevisionID: serverRevision.ID, UpstreamName: tool.Name, Alias: tool.Alias, Title: tool.Title, Description: tool.Description, InputSchema: datatypes.JSON(tool.InputSchema), OutputSchema: datatypes.JSON(tool.OutputSchema), VerifiedMetadata: datatypes.JSON(tool.VerifiedMetadata), RiskTier: tool.RiskTier, Status: "approved"})
	}
	if err := s.repo.CreateToolRevisions(ctx, toolRows); err != nil {
		fail("tool_persist_failed", err)
		return
	}
	var release *model.MCPCatalogRelease
	if job.TargetCatalogID != nil {
		release, err = s.createRelease(ctx, *job.TargetCatalogID, serverRevision, toolRows, job.CreatedBy)
		if err != nil {
			fail("release_persist_failed", err)
			return
		}
	}
	server.ActiveRevisionID, server.LifecycleStatus, server.UpdatedBy, server.UpdatedAt = &serverRevision.ID, model.MCPPlatformServerApproved, job.CreatedBy, time.Now().UTC()
	if !shouldAutoPublish || release == nil {
		job.Status, job.Step = model.MCPPlatformJobAwaitingApproval, model.MCPPlatformJobAwaitingApproval
		approval := &model.MCPApprovalRequest{ID: uuid.New(), ApprovalType: model.MCPPlatformApprovalAdmission, SubjectType: "server_revision", SubjectID: serverRevision.ID, RequestedBy: job.CreatedBy, Status: model.MCPPlatformApprovalPending, RequestDigest: serverRevision.Digest, Reason: "remote MCP admission requires review", CreatedAt: time.Now().UTC()}
		if err := s.repo.CreateApproval(ctx, approval); err != nil {
			fail("approval_persist_failed", err)
			return
		}
	}
	if err := s.repo.UpdateServer(ctx, server); err != nil {
		fail("server_update_failed", err)
		return
	}
	if shouldAutoPublish && release != nil {
		if err := s.repo.ActivateRelease(ctx, release.ID, release.ManifestDigest); err != nil {
			fail("release_publish_failed", err)
			return
		}
		server.LifecycleStatus = model.MCPPlatformServerPublished
		server.PublishedToolCount = len(tools)
		server.UpdatedAt = time.Now().UTC()
		if err := s.repo.UpdateServer(ctx, server); err != nil {
			fail("server_publish_state_failed", err)
			return
		}
		job.Status, job.Step = model.MCPPlatformJobActive, model.MCPPlatformJobActive
	}
	job.ServerID, job.RevisionID, job.UpdatedAt = &server.ID, &serverRevision.ID, time.Now().UTC()
	if job.Status == model.MCPPlatformJobActive {
		job.CompletedAt = mcpPtrTime(time.Now().UTC())
	}
	if err := s.repo.UpdateOnboardingJob(ctx, job); err != nil {
		s.logger.Error("mcp_onboarding_completion_persist_failed", zap.String("job_id", id.String()), zap.Error(err))
		return
	}
	s.logger.Info("mcp_onboarding_completed", zap.String("job_id", id.String()), zap.String("status", job.Status), zap.String("risk_tier", riskTier), zap.Int("tool_count", len(tools)))
}

func (s *MCPPlatformService) createRelease(ctx context.Context, catalogID uuid.UUID, revision *model.MCPServerRevision, tools []model.MCPToolRevision, createdBy string) (*model.MCPCatalogRelease, error) {
	releaseNo, err := s.repo.NextCatalogReleaseNo(ctx, catalogID)
	if err != nil {
		return nil, err
	}
	manifestTools := make([]map[string]interface{}, 0, len(tools))
	releaseTools := make([]model.MCPCatalogReleaseTool, 0, len(tools))
	for idx, tool := range tools {
		manifestTools = append(manifestTools, map[string]interface{}{"tool_revision_id": tool.ID.String(), "exposed_name": tool.Alias, "risk_tier": tool.RiskTier})
		releaseTools = append(releaseTools, model.MCPCatalogReleaseTool{ID: uuid.New(), ToolRevisionID: tool.ID, ExposedName: tool.Alias, Title: tool.Title, Description: tool.Description, InputSchema: tool.InputSchema, OutputSchema: tool.OutputSchema, ApprovalMode: "none", RateLimit: datatypes.JSON([]byte(`{}`)), Resource: datatypes.JSON([]byte(`{}`)), Status: "staged", DisplayOrder: idx})
	}
	manifest := map[string]interface{}{"protocol_version": revision.ProtocolVersion, "server_revision_id": revision.ID.String(), "server_revision_digest": revision.Digest, "tools": manifestTools}
	// A release is staged first. The caller activates it only after the server
	// pointer and immutable revision have been persisted successfully.
	release := &model.MCPCatalogRelease{ID: uuid.New(), CatalogID: catalogID, ServerRevisionID: &revision.ID, ReleaseNo: releaseNo, Manifest: datatypes.JSON(mustCanonicalJSON(manifest)), ManifestDigest: digestJSON(manifest), Status: "staged", CreatedBy: createdBy, CreatedAt: time.Now().UTC()}
	if err := s.repo.CreateCatalogRelease(ctx, release); err != nil {
		return nil, err
	}
	for idx := range releaseTools {
		releaseTools[idx].ReleaseID = release.ID
	}
	if err := s.repo.CreateCatalogReleaseTools(ctx, releaseTools); err != nil {
		return nil, err
	}
	return release, nil
}

func mustCanonicalJSON(value interface{}) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		return []byte(`{}`)
	}
	return data
}

type mcpDiscovery struct {
	ProtocolVersion string
	Capabilities    []byte
	RawTools        []byte
	Tools           []mcpDiscoveredTool
}
type mcpDiscoveredTool struct {
	Name, Alias, Title, Description, RiskTier   string
	InputSchema, OutputSchema, VerifiedMetadata []byte
}

func (s *MCPPlatformService) discover(ctx context.Context, endpoint *url.URL, authType, credentialRef string) (*mcpDiscovery, error) {
	initializeParams := map[string]interface{}{"protocolVersion": "2025-11-25", "capabilities": map[string]interface{}{}, "clientInfo": map[string]string{"name": "aegis-mcp-gateway", "version": "6.3"}}
	initResult, err := s.rpc(ctx, endpoint, "initialize", initializeParams, authType, credentialRef)
	if err != nil {
		return nil, err
	}
	protocolVersion := "2025-11-25"
	if v, ok := initResult["protocolVersion"].(string); ok && v != "" {
		protocolVersion = v
	}
	toolsResult, err := s.rpc(ctx, endpoint, "tools/list", map[string]interface{}{}, authType, credentialRef)
	if err != nil {
		return nil, err
	}
	toolsValue, ok := toolsResult["tools"].([]interface{})
	if !ok {
		return nil, errors.New("tools/list result does not contain tools")
	}
	rawTools, _ := json.Marshal(toolsValue)
	tools := make([]mcpDiscoveredTool, 0, len(toolsValue))
	aliases := map[string]int{}
	for _, value := range toolsValue {
		obj, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.New("tool entry is not an object")
		}
		name, ok := obj["name"].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return nil, errors.New("tool name is required")
		}
		alias := safeToolAlias(name, aliases)
		inputSchema, _ := json.Marshal(obj["inputSchema"])
		if len(inputSchema) == 0 || string(inputSchema) == "null" {
			inputSchema = []byte(`{}`)
		}
		outputSchema, _ := json.Marshal(obj["outputSchema"])
		if len(outputSchema) == 0 || string(outputSchema) == "null" {
			outputSchema = []byte(`{}`)
		}
		verified, _ := json.Marshal(map[string]interface{}{"source": "aegis_discovery", "annotations_verified": false})
		title, _ := obj["title"].(string)
		description, _ := obj["description"].(string)
		tools = append(tools, mcpDiscoveredTool{Name: name, Alias: alias, Title: title, Description: description, InputSchema: inputSchema, OutputSchema: outputSchema, VerifiedMetadata: verified, RiskTier: model.MCPPlatformRiskL2})
	}
	capabilities, _ := json.Marshal(initResult["capabilities"])
	if len(capabilities) == 0 || string(capabilities) == "null" {
		capabilities = []byte(`{}`)
	}
	return &mcpDiscovery{ProtocolVersion: protocolVersion, Capabilities: capabilities, RawTools: rawTools, Tools: tools}, nil
}

func (s *MCPPlatformService) rpc(ctx context.Context, endpoint *url.URL, method string, params interface{}, authType, credentialRef string) (map[string]interface{}, error) {
	body, err := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": uuid.NewString(), "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if credentialRef != "" && (authType == model.MCPPlatformAuthBearer || authType == model.MCPPlatformAuthAPIKey) {
		if s.credentialBroker == nil {
			return nil, errors.New("credential broker is not configured")
		}
		secret, err := s.credentialBroker(ctx, credentialRef)
		if err != nil {
			return nil, errors.New("credential broker resolution failed")
		}
		if strings.TrimSpace(secret) == "" {
			return nil, errors.New("credential broker returned an empty secret")
		}
		if authType == model.MCPPlatformAuthBearer {
			req.Header.Set("Authorization", "Bearer "+secret)
		} else {
			req.Header.Set("X-API-Key", secret)
		}
	}
	client := s.client
	if isTrustedLocalMCPEndpoint(endpoint) {
		// The development Compose service is intentionally the only private
		// endpoint admitted here. All other endpoints continue through the
		// public-address SSRF guard in s.client.Transport.
		client = &http.Client{
			Timeout:       s.client.Timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
			Transport: &http.Transport{
				Proxy:             nil,
				DialContext:       localMCPDialContext,
				ForceAttemptHTTP2: true,
			},
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		// Do not persist *url.Error: its Error string includes the complete
		// endpoint. Durable job errors and logs carry only a stable safe code.
		return nil, errors.New("upstream request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, 10<<20)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		data = extractSSEData(data)
	}
	var envelope struct {
		Result map[string]interface{} `json:"result"`
		Error  map[string]interface{} `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, errors.New("invalid upstream JSON-RPC response")
	}
	if envelope.Error != nil {
		return nil, errors.New("upstream JSON-RPC error")
	}
	if envelope.Result == nil {
		return nil, errors.New("upstream response missing result")
	}
	return envelope.Result, nil
}

func extractSSEData(data []byte) []byte {
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "data:") {
			return []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	return data
}

func validateMCPOnboardingRequest(req MCPOnboardingRequest) error {
	if len(strings.TrimSpace(req.DisplayName)) < 2 || len(req.DisplayName) > 160 {
		return errors.New("display_name must be between 2 and 160 characters")
	}
	if req.Environment == "" {
		return errors.New("environment is required")
	}
	if req.PublishPolicy == "" {
		return errors.New("publish_policy is required")
	}
	if req.AuthType == "" {
		return errors.New("auth_type is required")
	}
	_, err := validateRemoteEndpoint(req.EndpointURL, req.Environment)
	return err
}

func validateRemoteEndpoint(raw, environment string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, ErrMCPPlatformInvalidEndpoint
	}
	if u.User != nil || u.Fragment != "" || u.RawQuery != "" {
		return nil, errors.New("endpoint must not contain userinfo, query, or fragment")
	}
	if environment == "prod" && u.Scheme != "https" {
		return nil, errors.New("production MCP endpoint must use HTTPS")
	}
	if u.Scheme != "https" && !(environment != "prod" && u.Scheme == "http") {
		return nil, ErrMCPPlatformInvalidEndpoint
	}
	if isTrustedLocalMCPEndpointForEnvironment(u, environment) {
		return u, nil
	}
	if net.ParseIP(u.Hostname()) != nil && isForbiddenMCPIP(net.ParseIP(u.Hostname())) {
		return nil, errors.New("endpoint resolves to a forbidden address")
	}
	return u, nil
}

// isTrustedLocalMCPEndpointForEnvironment is a narrow development-only
// exception for the Aegis self-MCP Compose service. It is not a general
// private-network allowlist and cannot be used for production onboarding.
func isTrustedLocalMCPEndpointForEnvironment(u *url.URL, environment string) bool {
	return environment == "dev" && isTrustedLocalMCPEndpoint(u)
}

func isTrustedLocalMCPEndpoint(u *url.URL) bool {
	return u != nil && u.Scheme == "http" && strings.EqualFold(u.Hostname(), "aegis-mcp") && u.Port() == "8085"
}

func localMCPDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || !strings.EqualFold(host, "aegis-mcp") || port != "8085" {
		return nil, errors.New("local MCP dial target is not allowed")
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	var lastErr error
	for _, ip := range ips {
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("local MCP service did not resolve")
}

func safeMCPDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if isForbiddenMCPIP(ip) {
			return nil, errors.New("upstream address is forbidden")
		}
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	var lastErr error
	for _, ip := range ips {
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("upstream address did not resolve")
}

func isForbiddenMCPIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	private := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "100.64.0.0/10", "::/128", "fc00::/7", "fe80::/10"}
	for _, cidr := range private {
		_, n, _ := net.ParseCIDR(cidr)
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func validateAuthRef(authType, ref, environment string) error {
	switch authType {
	case model.MCPPlatformAuthNone:
		if environment == "prod" {
			return errors.New("production remote MCP requires authentication")
		}
	case model.MCPPlatformAuthOAuth2:
		if strings.TrimSpace(ref) == "" {
			return nil
		}
	case model.MCPPlatformAuthBearer, model.MCPPlatformAuthAPIKey:
		if strings.TrimSpace(ref) == "" {
			return errors.New("credential_ref is required for selected authentication")
		}
	default:
		return errors.New("unsupported authentication type")
	}
	return nil
}

func validateDiscoveredTools(tools []mcpDiscoveredTool) ([]mcpDiscoveredTool, error) {
	if len(tools) > 1000 {
		return nil, errors.New("remote server returned too many tools")
	}
	for i := range tools {
		if len(tools[i].Name) > 255 || strings.ContainsAny(tools[i].Name, "\r\n") {
			return nil, errors.New("invalid tool name")
		}
		if len(tools[i].InputSchema) > 512*1024 || len(tools[i].OutputSchema) > 512*1024 {
			return nil, errors.New("tool schema is too large")
		}
	}
	return tools, nil
}

func scanDiscoveredTools(tools []mcpDiscoveredTool) error {
	for _, tool := range tools {
		lower := strings.ToLower(tool.Name + " " + tool.Description)
		if strings.Contains(lower, "<script") || strings.Contains(lower, "javascript:") {
			return fmt.Errorf("tool %q contains unsafe markup", tool.Name)
		}
	}
	return nil
}

func classifyTools(tools []mcpDiscoveredTool) string {
	for _, tool := range tools {
		lower := strings.ToLower(tool.Name + " " + tool.Description)
		if strings.Contains(lower, "delete") || strings.Contains(lower, "execute") || strings.Contains(lower, "write") || strings.Contains(lower, "send") {
			return model.MCPPlatformRiskL3
		}
	}
	return model.MCPPlatformRiskL2
}

func safeToolAlias(name string, used map[string]int) string {
	base := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, name)
	if base == "" {
		base = "tool"
	}
	used[base]++
	if used[base] == 1 {
		return base
	}
	return fmt.Sprintf("%s_%d", base, used[base])
}

func digestJSON(value interface{}) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func safeEndpointDisplay(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "remote endpoint"
	}
	return u.Scheme + "://" + u.Host + strings.TrimSuffix(u.Path, "/")
}
func safeMCPError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 240 {
		msg = msg[:240]
	}
	return msg
}
func mcpPtrTime(t time.Time) *time.Time { return &t }

func (s *MCPPlatformService) Overview(ctx context.Context) (map[string]interface{}, error) {
	servers, err := s.repo.CountOperationalServers(ctx)
	if err != nil {
		return nil, err
	}
	tools, err := s.repo.CountOperationalPublishedTools(ctx)
	if err != nil {
		return nil, err
	}
	clients, err := s.repo.CountByTable(ctx, "mcp_clients", "status = ?", "active")
	if err != nil {
		return nil, err
	}
	pending, err := s.repo.CountPendingApprovals(ctx)
	if err != nil {
		return nil, err
	}
	highRisk, err := s.repo.CountRecentOperationalHighRiskCalls(ctx, time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"remote_servers": servers, "published_tools": tools, "active_clients": clients, "pending_approvals": pending, "high_risk_calls_24h": highRisk, "updated_at": time.Now().UTC()}, nil
}

func (s *MCPPlatformService) ListServers(ctx context.Context, q repository.MCPServerQuery) ([]model.MCPServer, int64, error) {
	return s.repo.ListServers(ctx, q)
}

// FindPublishedServers resolves an Assistant-provided service name against
// the operational MCP catalog. Raw endpoint URLs are intentionally not
// accepted here; Client authorization must bind to a published server record.
func (s *MCPPlatformService) FindPublishedServers(ctx context.Context, serviceName string) ([]model.MCPServer, error) {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		return nil, errors.New("service_name is required")
	}
	items, _, err := s.repo.ListServers(ctx, repository.MCPServerQuery{
		Keyword: serviceName, Status: model.MCPPlatformServerPublished, Page: 1, PageSize: 100,
	})
	return items, err
}
func (s *MCPPlatformService) GetServer(ctx context.Context, id uuid.UUID) (*model.MCPServer, error) {
	return s.repo.GetServer(ctx, id)
}

func (s *MCPPlatformService) GetInvocation(ctx context.Context, id uuid.UUID) (*model.MCPInvocation, error) {
	return s.repo.GetInvocation(ctx, id)
}

// RetireServer is the reversible delete operation exposed by the control
// plane. The service remains queryable for audit, while its active Client
// grants and credentials are revoked atomically by the repository.
func (s *MCPPlatformService) RetireServer(ctx context.Context, id uuid.UUID, operator string) (*model.MCPServer, error) {
	server, err := s.repo.GetServer(ctx, id)
	if err != nil {
		return nil, err
	}
	revokedClients, err := s.repo.RetireServer(ctx, id, operator)
	if err != nil {
		return nil, err
	}
	server.LifecycleStatus = model.MCPPlatformServerRetired
	s.logger.Info("mcp_server_retired",
		zap.String("server_id", id.String()),
		zap.String("operator", operator),
		zap.Int64("revoked_client_count", revokedClients),
	)
	return server, nil
}
func (s *MCPPlatformService) ListJobs(ctx context.Context, q repository.MCPOnboardingJobQuery) ([]model.MCPOnboardingJob, int64, error) {
	return s.repo.ListOnboardingJobs(ctx, q)
}
func (s *MCPPlatformService) GetJob(ctx context.Context, id uuid.UUID) (*model.MCPOnboardingJob, error) {
	return s.repo.GetOnboardingJob(ctx, id)
}

type MCPToolAuditItem struct {
	model.MCPToolRevision
	ServerID   uuid.UUID `json:"server_id"`
	ServerName string    `json:"server_name"`
}

func (s *MCPPlatformService) ListTools(ctx context.Context, serverRevisionID *uuid.UUID, page, size int) ([]MCPToolAuditItem, int64, error) {
	rows, total, err := s.repo.ListToolAuditRows(ctx, serverRevisionID, page, size)
	if err != nil {
		return nil, 0, err
	}
	items := make([]MCPToolAuditItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, MCPToolAuditItem{MCPToolRevision: row.MCPToolRevision, ServerID: row.ServerID, ServerName: row.ServerName})
	}
	return items, total, nil
}
func (s *MCPPlatformService) ListCatalogs(ctx context.Context, page, size int) ([]model.MCPCatalog, int64, error) {
	return s.repo.ListCatalogs(ctx, page, size)
}
func (s *MCPPlatformService) ListClients(ctx context.Context, page, size int) ([]model.MCPClient, int64, error) {
	return s.repo.ListClients(ctx, page, size)
}
func (s *MCPPlatformService) ListApprovals(ctx context.Context, status string, page, size int) ([]model.MCPApprovalRequest, int64, error) {
	return s.repo.ListApprovals(ctx, status, page, size)
}

type MCPInvocationAuditItem struct {
	ID             uuid.UUID  `json:"id"`
	ClientID       *uuid.UUID `json:"client_id,omitempty"`
	ClientKey      string     `json:"client_key"`
	ClientName     string     `json:"client_name"`
	ServerID       uuid.UUID  `json:"server_id"`
	ServerName     string     `json:"server_name"`
	ToolRevisionID *uuid.UUID `json:"tool_revision_id,omitempty"`
	ToolAlias      string     `json:"tool_alias"`
	ToolEnabled    bool       `json:"tool_enabled"`
	Status         string     `json:"status"`
	PolicyDecision string     `json:"policy_decision,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

func (s *MCPPlatformService) ListInvocations(ctx context.Context, page, size int) ([]MCPInvocationAuditItem, int64, error) {
	rows, total, err := s.repo.ListInvocations(ctx, page, size)
	if err != nil {
		return nil, 0, err
	}
	type invocationGrantState struct {
		serverID uuid.UUID
		tools    map[string]struct{}
	}
	grantStates := make(map[uuid.UUID]invocationGrantState)
	items := make([]MCPInvocationAuditItem, 0, len(rows))
	for _, row := range rows {
		enabled := false
		if row.ClientID != nil {
			state, found := grantStates[*row.ClientID]
			if !found {
				state.tools = make(map[string]struct{})
				if grant, grantErr := s.repo.GetActiveGrantByClientID(ctx, *row.ClientID); grantErr == nil {
					var aliases []string
					if json.Unmarshal(grant.ToolAllowlist, &aliases) == nil {
						for _, alias := range aliases {
							state.tools[alias] = struct{}{}
						}
					}
					if release, releaseErr := s.repo.GetActiveCatalogRelease(ctx, grant.CatalogID); releaseErr == nil && release.ServerRevisionID != nil {
						if revision, revisionErr := s.repo.GetServerRevision(ctx, *release.ServerRevisionID); revisionErr == nil {
							state.serverID = revision.ServerID
						}
					}
				}
				grantStates[*row.ClientID] = state
			}
			_, aliasEnabled := state.tools[row.ToolAlias]
			enabled = aliasEnabled && row.ToolRevisionID != nil && row.ServerID != nil && state.serverID == *row.ServerID
		}
		serverID := uuid.Nil
		if row.ServerID != nil {
			serverID = *row.ServerID
		}
		items = append(items, MCPInvocationAuditItem{
			ID: row.ID, ClientID: row.ClientID, ClientKey: row.ClientKey, ClientName: row.ClientName,
			ServerID: serverID, ServerName: row.ServerName, ToolRevisionID: row.ToolRevisionID,
			ToolAlias: row.ToolAlias, ToolEnabled: enabled, Status: row.Status,
			PolicyDecision: row.PolicyDecision, CreatedAt: row.CreatedAt, CompletedAt: row.CompletedAt,
		})
	}
	return items, total, nil
}

type MCPSecurityVerdictAuditItem struct {
	model.MCPSecurityVerdict
	ClientID     *uuid.UUID `json:"client_id,omitempty"`
	ClientKey    string     `json:"client_key"`
	ClientName   string     `json:"client_name"`
	ServerID     uuid.UUID  `json:"server_id"`
	ServerName   string     `json:"server_name"`
	ToolAlias    string     `json:"tool_alias"`
	MatchedRules []string   `json:"matched_rules"`
	Status       string     `json:"invocation_status"`
	CreatedAt    time.Time  `json:"invocation_created_at"`
}

func (s *MCPPlatformService) ListSecurityVerdicts(ctx context.Context, page, size int) ([]MCPSecurityVerdictAuditItem, int64, error) {
	rows, total, err := s.repo.ListSecurityVerdictAuditRows(ctx, page, size)
	if err != nil {
		return nil, 0, err
	}
	invocationIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		invocationIDs = append(invocationIDs, row.InvocationID)
	}
	matchedRules, err := s.repo.ListSecurityRuleMatchNames(ctx, invocationIDs)
	if err != nil {
		return nil, 0, err
	}
	items := make([]MCPSecurityVerdictAuditItem, 0, len(rows))
	for _, row := range rows {
		matched := matchedRules[row.InvocationID]
		if matched == nil {
			matched = []string{}
		}
		serverID := uuid.Nil
		if row.ServerID != nil {
			serverID = *row.ServerID
		}
		items = append(items, MCPSecurityVerdictAuditItem{
			MCPSecurityVerdict: row.MCPSecurityVerdict, ClientID: row.ClientID,
			ClientKey: row.ClientKey, ClientName: row.ClientName, ServerID: serverID,
			ServerName: row.ServerName, ToolAlias: row.ToolAlias, MatchedRules: matched, Status: row.Status, CreatedAt: row.CreatedAt,
		})
	}
	return items, total, nil
}

func (s *MCPPlatformService) ListSecurityRules(ctx context.Context, page, size int) ([]model.MCPRuleDefinition, int64, error) {
	return s.repo.ListSecurityRules(ctx, page, size)
}

func (s *MCPPlatformService) SetSecurityRuleEnabled(ctx context.Context, id uuid.UUID, enabled bool, operator string) (*model.MCPRuleDefinition, error) {
	item, err := s.repo.SetSecurityRuleEnabled(ctx, id, enabled)
	if err != nil {
		return nil, err
	}
	s.logger.Info("mcp_security_rule_enabled_changed", zap.String("rule_id", item.ID.String()), zap.String("rule_key", item.RuleKey), zap.String("operator", operator), zap.Bool("enabled", enabled))
	return item, nil
}

type MCPCatalogCreateRequest struct {
	CatalogKey  string `json:"catalog_key" binding:"required"`
	DisplayName string `json:"display_name" binding:"required"`
}

func (s *MCPPlatformService) CreateCatalog(ctx context.Context, req MCPCatalogCreateRequest, operator string) (*model.MCPCatalog, error) {
	key := strings.TrimSpace(req.CatalogKey)
	name := strings.TrimSpace(req.DisplayName)
	if len(key) < 2 || len(key) > 100 || !isSafeCatalogKey(key) {
		return nil, errors.New("catalog_key must contain only letters, numbers, hyphens, or underscores")
	}
	if len(name) < 2 || len(name) > 160 {
		return nil, errors.New("display_name must be between 2 and 160 characters")
	}
	item := &model.MCPCatalog{ID: uuid.New(), CatalogKey: key, DisplayName: name, Status: "active", CreatedBy: operator, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := s.repo.CreateCatalog(ctx, item); err != nil {
		return nil, err
	}
	s.logger.Info("mcp_catalog_created", zap.String("catalog_id", item.ID.String()), zap.String("catalog_key_hash", digestJSON(key)))
	return item, nil
}

type MCPCatalogSnapshot struct {
	Version    string                   `json:"version"`
	CatalogKey string                   `json:"catalog_key"`
	ReleaseID  string                   `json:"release_id"`
	ExpiresAt  time.Time                `json:"expires_at"`
	Tools      []MCPCatalogSnapshotTool `json:"tools"`
}

type MCPCatalogSnapshotTool struct {
	ExposedName  string          `json:"exposed_name"`
	Title        string          `json:"title,omitempty"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}

type SignedMCPCatalogSnapshot struct {
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature"`
}

func (s *MCPPlatformService) BuildCatalogSnapshot(ctx context.Context, catalogID uuid.UUID) (*SignedMCPCatalogSnapshot, error) {
	if len(s.catalogSigningKey) == 0 {
		return nil, errors.New("catalog signing key is not configured")
	}
	catalog, err := s.repo.GetCatalog(ctx, catalogID)
	if err != nil {
		return nil, err
	}
	release, err := s.repo.GetActiveCatalogRelease(ctx, catalogID)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListCatalogReleaseTools(ctx, release.ID)
	if err != nil {
		return nil, err
	}
	tools := make([]MCPCatalogSnapshotTool, 0, len(items))
	for _, item := range items {
		tools = append(tools, MCPCatalogSnapshotTool{ExposedName: item.ExposedName, Title: item.Title, Description: item.Description, InputSchema: json.RawMessage(item.InputSchema), OutputSchema: json.RawMessage(item.OutputSchema)})
	}
	payload := MCPCatalogSnapshot{Version: fmt.Sprintf("%d:%s", release.ReleaseNo, release.ManifestDigest), CatalogKey: catalog.CatalogKey, ReleaseID: release.ID.String(), ExpiresAt: time.Now().UTC().Add(15 * time.Minute), Tools: tools}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, s.catalogSigningKey)
	_, _ = mac.Write(data)
	return &SignedMCPCatalogSnapshot{Payload: data, Signature: hex.EncodeToString(mac.Sum(nil))}, nil
}

func (s *MCPPlatformService) DecideApproval(ctx context.Context, id uuid.UUID, expectedDigest, status, reason, operator, operatorRole string) (*model.MCPApprovalRequest, error) {
	if status != model.MCPPlatformApprovalApproved && status != model.MCPPlatformApprovalRejected {
		return nil, errors.New("unsupported approval decision")
	}
	current, err := s.repo.GetApproval(ctx, id)
	if err != nil {
		return nil, err
	}
	if current.RequestedBy == operator && operatorRole != model.RoleAdmin {
		return nil, ErrMCPPlatformSelfApproval
	}
	if expectedDigest == "" || expectedDigest != current.RequestDigest {
		return nil, errors.New("approval request digest mismatch")
	}
	var revision *model.MCPServerRevision
	var server *model.MCPServer
	if current.SubjectType == "server_revision" {
		revision, err = s.repo.GetServerRevision(ctx, current.SubjectID)
		if err != nil {
			return nil, err
		}
		server, err = s.repo.GetServer(ctx, revision.ServerID)
		if err != nil {
			return nil, err
		}
		if server.LifecycleStatus == model.MCPPlatformServerRetired {
			return nil, errors.New("approval target server is retired")
		}
	}
	item, err := s.repo.UpdateApprovalDecision(ctx, id, expectedDigest, status, operator, reason)
	if err != nil {
		return nil, err
	}
	if current.SubjectType == "server_revision" {
		if status == model.MCPPlatformApprovalApproved {
			server.LifecycleStatus = model.MCPPlatformServerPublished
			server.PublishedToolCount = server.ToolCount
			if release, releaseErr := s.repo.GetCatalogReleaseByServerRevision(ctx, revision.ID); releaseErr == nil && release.Status == "staged" {
				if activateErr := s.repo.ActivateRelease(ctx, release.ID, release.ManifestDigest); activateErr != nil {
					return nil, activateErr
				}
			}
		} else {
			server.LifecycleStatus = model.MCPPlatformServerQuarantined
		}
		server.UpdatedBy = operator
		if err := s.repo.UpdateServer(ctx, server); err != nil {
			return nil, err
		}
	}
	s.logger.Info("mcp_approval_decided", zap.String("approval_id", id.String()), zap.String("status", status), zap.String("operator_hash", digestJSON(operator)))
	return item, nil
}

func isSafeCatalogKey(value string) bool {
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

type MCPClientCreateRequest struct {
	ClientKey   string `json:"client_key" binding:"required"`
	DisplayName string `json:"display_name" binding:"required"`
	ClientType  string `json:"client_type" binding:"required"`
}

func (s *MCPPlatformService) CreateClient(ctx context.Context, req MCPClientCreateRequest, operator string) (*model.MCPClient, error) {
	if !isSafeCatalogKey(strings.TrimSpace(req.ClientKey)) {
		return nil, errors.New("client_key contains unsupported characters")
	}
	if len(strings.TrimSpace(req.DisplayName)) < 2 || len(req.DisplayName) > 160 {
		return nil, errors.New("display_name must be between 2 and 160 characters")
	}
	switch req.ClientType {
	case "public", "confidential", "service":
	default:
		return nil, errors.New("unsupported client_type")
	}
	item := &model.MCPClient{ID: uuid.New(), ClientKey: strings.TrimSpace(req.ClientKey), DisplayName: strings.TrimSpace(req.DisplayName), ClientType: req.ClientType, Status: "active", CreatedBy: operator, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := s.repo.CreateClient(ctx, item); err != nil {
		return nil, err
	}
	s.logger.Info("mcp_client_created", zap.String("client_id", item.ID.String()), zap.String("client_key_hash", digestJSON(item.ClientKey)))
	return item, nil
}

type MCPGrantCreateRequest struct {
	ClientID      uuid.UUID       `json:"client_id" binding:"required"`
	CatalogID     uuid.UUID       `json:"catalog_id" binding:"required"`
	ToolAllowlist json.RawMessage `json:"tool_allowlist" binding:"required"`
	ResourceScope json.RawMessage `json:"resource_scope"`
	ExpiresAt     *time.Time      `json:"expires_at"`
}

func (s *MCPPlatformService) CreateGrant(ctx context.Context, req MCPGrantCreateRequest, operator string) (*model.MCPClientGrant, error) {
	client, err := s.repo.GetClient(ctx, req.ClientID)
	if err != nil || client.Status != "active" {
		return nil, errors.New("client is not active")
	}
	if _, err := s.repo.GetCatalog(ctx, req.CatalogID); err != nil {
		return nil, errors.New("catalog not found")
	}
	if len(req.ToolAllowlist) == 0 || !json.Valid(req.ToolAllowlist) || len(req.ToolAllowlist) > 128*1024 {
		return nil, errors.New("tool_allowlist must be valid bounded JSON")
	}
	resource := req.ResourceScope
	if len(resource) == 0 {
		resource = json.RawMessage(`{}`)
	}
	if !json.Valid(resource) || len(resource) > 128*1024 {
		return nil, errors.New("resource_scope must be valid bounded JSON")
	}
	item := &model.MCPClientGrant{ID: uuid.New(), ClientID: req.ClientID, CatalogID: req.CatalogID, ToolAllowlist: datatypes.JSON(req.ToolAllowlist), ResourceScope: datatypes.JSON(resource), Status: "pending", ExpiresAt: req.ExpiresAt, CreatedBy: operator, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := s.repo.CreateGrant(ctx, item); err != nil {
		return nil, err
	}
	s.logger.Info("mcp_grant_created", zap.String("grant_id", item.ID.String()), zap.String("client_id", req.ClientID.String()), zap.String("catalog_id", req.CatalogID.String()))
	return item, nil
}

// MCPClientEndpointCreateRequest is the user-facing request for a dedicated
// AI Agent endpoint. A server is deliberately selected instead of accepting a
// raw endpoint URL; the Agent can only reach an already published Aegis MCP
// server revision.
type MCPClientEndpointCreateRequest struct {
	ClientKey   string     `json:"client_key" binding:"required"`
	DisplayName string     `json:"display_name" binding:"required"`
	ClientType  string     `json:"client_type" binding:"required"`
	ServerID    uuid.UUID  `json:"server_id" binding:"required"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

type MCPClientEndpointTool struct {
	Alias       string `json:"alias"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	RiskTier    string `json:"risk_tier"`
	Enabled     bool   `json:"enabled"`
}

type MCPClientEndpointView struct {
	ClientID    uuid.UUID               `json:"client_id"`
	ClientKey   string                  `json:"client_key"`
	DisplayName string                  `json:"display_name"`
	ClientType  string                  `json:"client_type"`
	Status      string                  `json:"status"`
	GrantID     uuid.UUID               `json:"grant_id"`
	ServerID    uuid.UUID               `json:"server_id"`
	ServerName  string                  `json:"server_name"`
	Endpoint    string                  `json:"endpoint"`
	ExpiresAt   *time.Time              `json:"expires_at,omitempty"`
	Tools       []MCPClientEndpointTool `json:"tools"`
}

type MCPClientEndpointCreated struct {
	MCPClientEndpointView
	Token string `json:"token"`
}

func generateMCPClientToken() (token, prefix, hash string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", "", "", err
	}
	token = "aegis_mcp_" + hex.EncodeToString(raw)
	prefix = token[:18]
	sum := sha256.Sum256([]byte(token))
	hash = hex.EncodeToString(sum[:])
	return token, prefix, hash, nil
}

func normalizeToolAllowlist(raw []string, available []model.MCPToolRevision) ([]string, error) {
	availableSet := make(map[string]struct{}, len(available))
	for _, tool := range available {
		availableSet[tool.Alias] = struct{}{}
	}
	if len(raw) == 0 {
		for _, tool := range available {
			raw = append(raw, tool.Alias)
		}
	}
	seen := make(map[string]struct{}, len(raw))
	result := make([]string, 0, len(raw))
	for _, alias := range raw {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		if _, ok := availableSet[alias]; !ok {
			return nil, fmt.Errorf("tool %q is not published by selected server", alias)
		}
		if _, ok := seen[alias]; ok {
			continue
		}
		seen[alias] = struct{}{}
		result = append(result, alias)
	}
	return result, nil
}

func endpointURL(base, clientKey string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = "http://localhost:8084"
	}
	return base + "/mcp/v1/clients/" + clientKey
}

func (s *MCPPlatformService) CreateClientEndpoint(ctx context.Context, req MCPClientEndpointCreateRequest, operator, publicGatewayBaseURL string) (*MCPClientEndpointCreated, error) {
	key := strings.TrimSpace(req.ClientKey)
	name := strings.TrimSpace(req.DisplayName)
	if !isSafeCatalogKey(key) || len(key) < 2 || len(key) > 80 {
		return nil, errors.New("client_key contains unsupported characters")
	}
	if len(name) < 2 || len(name) > 160 {
		return nil, errors.New("display_name must be between 2 and 160 characters")
	}
	if req.ClientType != "service" && req.ClientType != "confidential" {
		return nil, errors.New("client_type must be service or confidential")
	}
	server, err := s.repo.GetServer(ctx, req.ServerID)
	if err != nil || server.LifecycleStatus != model.MCPPlatformServerPublished || server.ActiveRevisionID == nil {
		return nil, errors.New("selected MCP server is not published")
	}
	revision, err := s.repo.GetServerRevision(ctx, *server.ActiveRevisionID)
	if err != nil {
		return nil, errors.New("selected MCP server revision was not found")
	}
	tools, err := s.repo.ListToolRevisions(ctx, &revision.ID)
	if err != nil {
		return nil, err
	}
	availableTools := approvedMCPTools(tools)
	// Endpoint creation binds one service. Fine-grained tool selection belongs
	// exclusively to Client Grants, so every currently published tool starts on.
	allowlist, err := normalizeToolAllowlist(nil, availableTools)
	if err != nil {
		return nil, err
	}

	client := &model.MCPClient{ID: uuid.New(), ClientKey: key, DisplayName: name, ClientType: req.ClientType, Status: "active", CreatedBy: operator, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := s.repo.CreateClient(ctx, client); err != nil {
		return nil, err
	}
	// Use a private catalog/release for this endpoint. This keeps the existing
	// catalog model intact while guaranteeing that the client is bound to one
	// server revision only.
	catalog := &model.MCPCatalog{ID: uuid.New(), CatalogKey: "client_" + key, DisplayName: name + " 工具目录", Status: "active", CreatedBy: operator, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := s.repo.CreateCatalog(ctx, catalog); err != nil {
		return nil, err
	}
	release, err := s.createRelease(ctx, catalog.ID, revision, tools, operator)
	if err != nil {
		return nil, err
	}
	if err := s.repo.ActivateRelease(ctx, release.ID, release.ManifestDigest); err != nil {
		return nil, err
	}
	allowlistJSON, _ := json.Marshal(allowlist)
	grant := &model.MCPClientGrant{ID: uuid.New(), ClientID: client.ID, CatalogID: catalog.ID, ToolAllowlist: datatypes.JSON(allowlistJSON), ResourceScope: datatypes.JSON([]byte(`{}`)), Status: "active", ExpiresAt: req.ExpiresAt, CreatedBy: operator, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := s.repo.CreateGrant(ctx, grant); err != nil {
		return nil, err
	}
	token, prefix, tokenHash, err := generateMCPClientToken()
	if err != nil {
		return nil, err
	}
	credential := &model.MCPClientCredential{ID: uuid.New(), ClientID: client.ID, TokenPrefix: prefix, TokenHash: tokenHash, Status: "active", ExpiresAt: req.ExpiresAt, CreatedBy: operator, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := s.repo.CreateClientCredential(ctx, credential); err != nil {
		return nil, err
	}
	view := MCPClientEndpointView{ClientID: client.ID, ClientKey: client.ClientKey, DisplayName: client.DisplayName, ClientType: client.ClientType, Status: grant.Status, GrantID: grant.ID, ServerID: server.ID, ServerName: server.DisplayName, Endpoint: endpointURL(publicGatewayBaseURL, key), ExpiresAt: grant.ExpiresAt, Tools: endpointTools(availableTools, allowlist)}
	s.logger.Info("mcp_client_endpoint_created", zap.String("client_id", client.ID.String()), zap.String("grant_id", grant.ID.String()), zap.String("server_id", server.ID.String()), zap.Int("tool_count", len(allowlist)))
	return &MCPClientEndpointCreated{MCPClientEndpointView: view, Token: token}, nil
}

func endpointTools(tools []model.MCPToolRevision, allowlist []string) []MCPClientEndpointTool {
	allowed := make(map[string]struct{}, len(allowlist))
	for _, alias := range allowlist {
		allowed[alias] = struct{}{}
	}
	result := make([]MCPClientEndpointTool, 0, len(tools))
	for _, tool := range tools {
		_, enabled := allowed[tool.Alias]
		result = append(result, MCPClientEndpointTool{Alias: tool.Alias, Title: tool.Title, Description: tool.Description, RiskTier: tool.RiskTier, Enabled: enabled})
	}
	return result
}

func approvedMCPTools(tools []model.MCPToolRevision) []model.MCPToolRevision {
	result := make([]model.MCPToolRevision, 0, len(tools))
	for _, tool := range tools {
		if tool.Status == "approved" {
			result = append(result, tool)
		}
	}
	return result
}

func (s *MCPPlatformService) ListClientEndpoints(ctx context.Context, publicGatewayBaseURL string, page, size int) ([]MCPClientEndpointView, int64, error) {
	clients, total, err := s.repo.ListClients(ctx, page, size)
	if err != nil {
		return nil, 0, err
	}
	result := make([]MCPClientEndpointView, 0, len(clients))
	for _, client := range clients {
		grant, grantErr := s.repo.GetActiveGrantByClientID(ctx, client.ID)
		if grantErr != nil {
			continue
		}
		release, releaseErr := s.repo.GetActiveCatalogRelease(ctx, grant.CatalogID)
		if releaseErr != nil || release.ServerRevisionID == nil {
			continue
		}
		serverRevision, revisionErr := s.repo.GetServerRevision(ctx, *release.ServerRevisionID)
		if revisionErr != nil {
			continue
		}
		server, serverErr := s.repo.GetServer(ctx, serverRevision.ServerID)
		if serverErr != nil {
			continue
		}
		tools, toolsErr := s.repo.ListToolRevisions(ctx, &serverRevision.ID)
		if toolsErr != nil {
			return nil, 0, toolsErr
		}
		var allowlist []string
		if err := json.Unmarshal(grant.ToolAllowlist, &allowlist); err != nil {
			return nil, 0, err
		}
		result = append(result, MCPClientEndpointView{ClientID: client.ID, ClientKey: client.ClientKey, DisplayName: client.DisplayName, ClientType: client.ClientType, Status: grant.Status, GrantID: grant.ID, ServerID: server.ID, ServerName: server.DisplayName, Endpoint: endpointURL(publicGatewayBaseURL, client.ClientKey), ExpiresAt: grant.ExpiresAt, Tools: endpointTools(approvedMCPTools(tools), allowlist)})
	}
	return result, total, nil
}

func (s *MCPPlatformService) UpdateClientEndpointTools(ctx context.Context, grantID uuid.UUID, aliases []string, operator, publicGatewayBaseURL string) (*MCPClientEndpointView, error) {
	grant, err := s.repo.GetGrant(ctx, grantID)
	if err != nil || grant.Status != "active" {
		return nil, ErrMCPPlatformClientEndpointDenied
	}
	release, err := s.repo.GetActiveCatalogRelease(ctx, grant.CatalogID)
	if err != nil || release.ServerRevisionID == nil {
		return nil, ErrMCPPlatformClientEndpointDenied
	}
	tools, err := s.repo.ListToolRevisions(ctx, release.ServerRevisionID)
	if err != nil {
		return nil, err
	}
	availableTools := approvedMCPTools(tools)
	allowlist, err := normalizeToolAllowlist(aliases, availableTools)
	if err != nil {
		return nil, err
	}
	// An explicit empty array means disable every tool. This is different from
	// a missing allowlist during creation, where all published tools are on.
	if aliases != nil && len(aliases) == 0 {
		allowlist = []string{}
	}
	data, _ := json.Marshal(allowlist)
	grant.ToolAllowlist = datatypes.JSON(data)
	if err := s.repo.UpdateGrant(ctx, grant); err != nil {
		return nil, err
	}
	client, err := s.repo.GetClient(ctx, grant.ClientID)
	if err != nil {
		return nil, err
	}
	serverRevision, err := s.repo.GetServerRevision(ctx, *release.ServerRevisionID)
	if err != nil {
		return nil, err
	}
	server, err := s.repo.GetServer(ctx, serverRevision.ServerID)
	if err != nil {
		return nil, err
	}
	s.logger.Info("mcp_client_tool_allowlist_updated", zap.String("client_id", client.ID.String()), zap.String("grant_id", grant.ID.String()), zap.String("server_id", server.ID.String()), zap.String("operator", operator), zap.Int("enabled_tool_count", len(allowlist)))
	return &MCPClientEndpointView{ClientID: client.ID, ClientKey: client.ClientKey, DisplayName: client.DisplayName, ClientType: client.ClientType, Status: grant.Status, GrantID: grant.ID, ServerID: server.ID, ServerName: server.DisplayName, Endpoint: endpointURL(publicGatewayBaseURL, client.ClientKey), ExpiresAt: grant.ExpiresAt, Tools: endpointTools(availableTools, allowlist)}, nil
}

// RevokeClientEndpoint is the reversible delete operation for a Client
// authorization. It revokes all grants and credentials, preserving the Client
// identity and invocation history for audit.
func (s *MCPPlatformService) RevokeClientEndpoint(ctx context.Context, clientID uuid.UUID, operator string) (*MCPClientEndpointRevokeResult, error) {
	client, err := s.repo.GetClient(ctx, clientID)
	if err != nil {
		return nil, err
	}
	grant, grantErr := s.repo.GetActiveGrantByClientID(ctx, clientID)
	grantID := uuid.Nil
	if grantErr == nil {
		grantID = grant.ID
	} else if !errors.Is(grantErr, repository.ErrMCPPlatformNotFound) {
		return nil, grantErr
	}
	changed, err := s.repo.RevokeClientEndpoint(ctx, clientID)
	if err != nil {
		return nil, err
	}
	client.Status = "revoked"
	s.logger.Info("mcp_client_endpoint_revoked",
		zap.String("client_id", clientID.String()),
		zap.String("client_key_hash", digestJSON(client.ClientKey)),
		zap.String("grant_id", grantID.String()),
		zap.String("operator", operator),
		zap.Int64("changed", changed),
	)
	return &MCPClientEndpointRevokeResult{ClientID: clientID, ClientKey: client.ClientKey, GrantID: grantID, Status: client.Status, Revoked: true, Changed: changed > 0}, nil
}

type MCPClientEndpointRevokeResult struct {
	ClientID  uuid.UUID `json:"client_id"`
	ClientKey string    `json:"client_key"`
	GrantID   uuid.UUID `json:"grant_id,omitempty"`
	Status    string    `json:"status"`
	Revoked   bool      `json:"revoked"`
	Changed   bool      `json:"changed"`
}

type MCPInvocationToolDisableResult struct {
	InvocationID uuid.UUID `json:"invocation_id"`
	ClientID     uuid.UUID `json:"client_id"`
	GrantID      uuid.UUID `json:"grant_id"`
	ServerID     uuid.UUID `json:"server_id"`
	ToolAlias    string    `json:"tool_alias"`
	Disabled     bool      `json:"disabled"`
	Changed      bool      `json:"changed"`
}

// DisableInvocationTool revokes one tool from the active grant of the Client
// that produced the invocation. It does not disable the tool for other Clients.
func (s *MCPPlatformService) DisableInvocationTool(ctx context.Context, invocationID uuid.UUID, operator string) (*MCPInvocationToolDisableResult, error) {
	invocation, err := s.repo.GetInvocation(ctx, invocationID)
	if err != nil {
		return nil, err
	}
	if invocation.ClientID == nil || invocation.ToolRevisionID == nil {
		return nil, ErrMCPPlatformClientEndpointDenied
	}
	tool, err := s.repo.GetToolRevision(ctx, *invocation.ToolRevisionID)
	if err != nil {
		return nil, err
	}
	historicalRevision, err := s.repo.GetServerRevision(ctx, tool.ServerRevisionID)
	if err != nil {
		return nil, err
	}
	grant, err := s.repo.GetActiveGrantByClientID(ctx, *invocation.ClientID)
	if err != nil {
		return nil, ErrMCPPlatformClientEndpointDenied
	}
	release, err := s.repo.GetActiveCatalogRelease(ctx, grant.CatalogID)
	if err != nil || release.ServerRevisionID == nil {
		return nil, ErrMCPPlatformClientEndpointDenied
	}
	activeRevision, err := s.repo.GetServerRevision(ctx, *release.ServerRevisionID)
	if err != nil || activeRevision.ServerID != historicalRevision.ServerID {
		return nil, ErrMCPPlatformClientEndpointDenied
	}
	var allowlist []string
	if err := json.Unmarshal(grant.ToolAllowlist, &allowlist); err != nil {
		return nil, ErrMCPPlatformClientEndpointDenied
	}
	alias := tool.Alias
	changed := false
	updated := make([]string, 0, len(allowlist))
	for _, item := range allowlist {
		if item == alias {
			changed = true
			continue
		}
		updated = append(updated, item)
	}
	if changed {
		data, _ := json.Marshal(updated)
		grant.ToolAllowlist = datatypes.JSON(data)
		if err := s.repo.UpdateGrant(ctx, grant); err != nil {
			return nil, err
		}
	}
	result := &MCPInvocationToolDisableResult{InvocationID: invocation.ID, ClientID: *invocation.ClientID, GrantID: grant.ID, ServerID: historicalRevision.ServerID, ToolAlias: alias, Disabled: true, Changed: changed}
	s.logger.Info("mcp_client_tool_disabled_from_audit", zap.String("invocation_id", invocation.ID.String()), zap.String("client_id", invocation.ClientID.String()), zap.String("grant_id", grant.ID.String()), zap.String("server_id", historicalRevision.ServerID.String()), zap.String("tool_alias", alias), zap.String("operator", operator), zap.Bool("changed", changed))
	return result, nil
}

type MCPRuntimeTool struct {
	Name         string          `json:"name"`
	Title        string          `json:"title,omitempty"`
	Description  string          `json:"description,omitempty"`
	RiskTier     string          `json:"risk_tier,omitempty"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
}

func (s *MCPPlatformService) resolveRuntime(ctx context.Context, token, clientKey string) (*model.MCPClient, *model.MCPClientGrant, *model.MCPServer, []model.MCPToolRevision, error) {
	if len(token) < 20 || len(token) > 256 {
		return nil, nil, nil, nil, ErrMCPPlatformClientEndpointDenied
	}
	sum := sha256.Sum256([]byte(token))
	credential, err := s.repo.GetActiveClientCredentialByHash(ctx, hex.EncodeToString(sum[:]))
	if err != nil {
		return nil, nil, nil, nil, ErrMCPPlatformClientEndpointDenied
	}
	client, err := s.repo.GetClient(ctx, credential.ClientID)
	if err != nil || client.Status != "active" || (clientKey != "" && client.ClientKey != clientKey) {
		return nil, nil, nil, nil, ErrMCPPlatformClientEndpointDenied
	}
	grant, err := s.repo.GetActiveGrantByClientID(ctx, client.ID)
	if err != nil {
		return nil, nil, nil, nil, ErrMCPPlatformClientEndpointDenied
	}
	release, err := s.repo.GetActiveCatalogRelease(ctx, grant.CatalogID)
	if err != nil || release.ServerRevisionID == nil {
		return nil, nil, nil, nil, ErrMCPPlatformClientEndpointDenied
	}
	revision, err := s.repo.GetServerRevision(ctx, *release.ServerRevisionID)
	if err != nil {
		return nil, nil, nil, nil, ErrMCPPlatformClientEndpointDenied
	}
	server, err := s.repo.GetServer(ctx, revision.ServerID)
	if err != nil || server.LifecycleStatus != model.MCPPlatformServerPublished || server.ActiveRevisionID == nil || *server.ActiveRevisionID != revision.ID {
		return nil, nil, nil, nil, ErrMCPPlatformClientEndpointDenied
	}
	_ = s.repo.TouchClientCredential(ctx, credential.ID, time.Now().UTC())
	tools, err := s.repo.ListToolRevisions(ctx, &revision.ID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return client, grant, server, tools, nil
}

func (s *MCPPlatformService) RuntimeTools(ctx context.Context, token, clientKey string) ([]MCPRuntimeTool, error) {
	_, grant, _, tools, err := s.resolveRuntime(ctx, token, clientKey)
	if err != nil {
		return nil, err
	}
	var allowlist []string
	if err := json.Unmarshal(grant.ToolAllowlist, &allowlist); err != nil {
		return nil, ErrMCPPlatformClientEndpointDenied
	}
	allowed := make(map[string]struct{}, len(allowlist))
	for _, alias := range allowlist {
		allowed[alias] = struct{}{}
	}
	result := make([]MCPRuntimeTool, 0, len(tools))
	for _, tool := range tools {
		if _, ok := allowed[tool.Alias]; !ok || tool.Status != "approved" {
			continue
		}
		result = append(result, MCPRuntimeTool{Name: tool.Alias, Title: tool.Title, Description: tool.Description, RiskTier: tool.RiskTier, InputSchema: json.RawMessage(tool.InputSchema), OutputSchema: json.RawMessage(tool.OutputSchema)})
	}
	return result, nil
}

func (s *MCPPlatformService) RuntimeCall(ctx context.Context, token, clientKey, alias string, arguments json.RawMessage) (map[string]interface{}, error) {
	return s.RuntimeCallAs(ctx, token, clientKey, alias, arguments, "")
}

// RuntimeCallAs keeps the Client credential as the authorization identity and
// optionally records a validated Assistant operator in the invocation audit.
// Existing direct Client callers retain the historical client-key identity.
func (s *MCPPlatformService) RuntimeCallAs(ctx context.Context, token, clientKey, alias string, arguments json.RawMessage, actor string) (map[string]interface{}, error) {
	client, grant, server, tools, err := s.resolveRuntime(ctx, token, clientKey)
	if err != nil {
		return nil, err
	}
	var allowlist []string
	if err := json.Unmarshal(grant.ToolAllowlist, &allowlist); err != nil {
		return nil, ErrMCPPlatformClientEndpointDenied
	}
	allowed := make(map[string]struct{}, len(allowlist))
	for _, item := range allowlist {
		allowed[item] = struct{}{}
	}
	if _, ok := allowed[alias]; !ok {
		return nil, ErrMCPPlatformToolNotAllowed
	}
	var selected *model.MCPToolRevision
	for idx := range tools {
		if tools[idx].Alias == alias && tools[idx].Status == "approved" {
			selected = &tools[idx]
			break
		}
	}
	if selected == nil {
		return nil, ErrMCPPlatformToolNotAllowed
	}
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	if !json.Valid(arguments) || len(arguments) > 1<<20 {
		return nil, errors.New("invalid tool arguments")
	}
	var argumentObject map[string]interface{}
	if err := json.Unmarshal(arguments, &argumentObject); err != nil || argumentObject == nil {
		return nil, errors.New("tool arguments must be a JSON object")
	}
	now := time.Now().UTC()
	userID := strings.TrimSpace(actor)
	if userID == "" {
		userID = client.ClientKey
	}
	invocation := &model.MCPInvocation{ID: uuid.New(), ClientID: &client.ID, ToolRevisionID: &selected.ID, UserID: userID, ToolAlias: alias, Status: "started", PolicyDecision: "allow", RequestDigest: digestJSON(arguments), CreatedAt: now}
	if err := s.repo.CreateInvocation(ctx, invocation); err != nil {
		return nil, err
	}
	preEvaluation, err := s.evaluateMCPSecurity(ctx, invocation.ID, "pre", selected, argumentObject, nil, nil)
	if err != nil {
		_ = s.repo.UpdateInvocation(ctx, invocation.ID, "failed", "", mcpPtrTime(time.Now().UTC()))
		return nil, err
	}
	if preEvaluation.blocked {
		completedAt := time.Now().UTC()
		if err := s.persistMCPSecurityEvaluation(ctx, invocation.ID, "blocked", "", completedAt, preEvaluation); err != nil {
			return nil, err
		}
		s.logger.Warn("mcp_invocation_blocked_by_security_rule", zap.String("invocation_id", invocation.ID.String()), zap.String("client_id", client.ID.String()), zap.String("server_id", server.ID.String()), zap.String("tool_alias", alias), zap.Strings("rule_keys", preEvaluation.ruleKeys))
		return nil, ErrMCPPlatformSecurityBlocked
	}
	params := map[string]interface{}{"name": selected.UpstreamName, "arguments": json.RawMessage(arguments)}
	result, callErr := s.rpc(ctx, mustURL(server.EndpointURL), "tools/call", params, server.AuthType, server.CredentialRef)
	if callErr != nil {
		completedAt := time.Now().UTC()
		postEvaluation, evalErr := s.evaluateMCPSecurity(ctx, invocation.ID, "post", selected, argumentObject, nil, callErr)
		if evalErr != nil {
			_ = s.repo.UpdateInvocation(ctx, invocation.ID, "failed", "", &completedAt)
			return nil, callErr
		}
		if err := s.persistMCPSecurityEvaluation(ctx, invocation.ID, "failed", "", completedAt, mergeMCPSecurityEvaluations(preEvaluation, postEvaluation)); err != nil {
			s.logger.Error("mcp_security_evaluation_persist_failed", zap.String("invocation_id", invocation.ID.String()), zap.Error(err))
		}
		return nil, callErr
	}
	completedAt := time.Now().UTC()
	postEvaluation, err := s.evaluateMCPSecurity(ctx, invocation.ID, "post", selected, argumentObject, result, nil)
	if err != nil {
		_ = s.repo.UpdateInvocation(ctx, invocation.ID, "failed", "", &completedAt)
		return nil, err
	}
	evaluation := mergeMCPSecurityEvaluations(preEvaluation, postEvaluation)
	status := "succeeded"
	if evaluation.blocked {
		status = "blocked"
	}
	if err := s.persistMCPSecurityEvaluation(ctx, invocation.ID, status, digestJSON(result), completedAt, evaluation); err != nil {
		return nil, err
	}
	if evaluation.blocked {
		s.logger.Warn("mcp_result_blocked_by_security_rule", zap.String("invocation_id", invocation.ID.String()), zap.String("client_id", client.ID.String()), zap.String("server_id", server.ID.String()), zap.String("tool_alias", alias), zap.Strings("rule_keys", evaluation.ruleKeys))
		return nil, ErrMCPPlatformSecurityBlocked
	}
	return result, nil
}

type mcpSecurityEvaluation struct {
	severity string
	blocked  bool
	hits     []model.MCPRuleHit
	evidence []map[string]interface{}
	ruleKeys []string
}

type mcpSecurityRuleDefinition struct {
	Matcher   string      `json:"matcher"`
	Threshold interface{} `json:"threshold"`
	Keys      []string    `json:"keys"`
	Patterns  []string    `json:"patterns"`
	Action    string      `json:"action"`
}

func (s *MCPPlatformService) evaluateMCPSecurity(ctx context.Context, invocationID uuid.UUID, phase string, tool *model.MCPToolRevision, arguments, result map[string]interface{}, callErr error) (mcpSecurityEvaluation, error) {
	evaluation := mcpSecurityEvaluation{severity: "low", evidence: []map[string]interface{}{}}
	rules, err := s.repo.ListEnabledSecurityRules(ctx, phase)
	if err != nil {
		return evaluation, err
	}
	resultJSON, _ := json.Marshal(result)
	for _, rule := range rules {
		var definition mcpSecurityRuleDefinition
		if err := json.Unmarshal(rule.Definition, &definition); err != nil {
			s.logger.Warn("mcp_security_rule_definition_invalid", zap.String("rule_id", rule.ID.String()), zap.String("rule_key", rule.RuleKey), zap.Error(err))
			continue
		}
		matched := false
		evidence := map[string]interface{}{"rule_key": rule.RuleKey, "phase": phase, "action": definition.Action}
		switch definition.Matcher {
		case "tool_risk_at_least":
			threshold, _ := definition.Threshold.(string)
			matched = mcpRiskRank(tool.RiskTier) >= mcpRiskRank(threshold)
			if matched {
				evidence["tool_risk"] = tool.RiskTier
				evidence["threshold"] = threshold
			}
		case "sensitive_output_keys":
			paths := findMCPSensitiveKeyPaths(result, definition.Keys, "$")
			matched = len(paths) > 0
			if matched {
				evidence["matched_paths"] = paths
			}
		case "sensitive_input_keys":
			paths := findMCPSensitiveKeyPaths(arguments, definition.Keys, "$")
			matched = len(paths) > 0
			if matched {
				evidence["matched_paths"] = paths
			}
		case "input_patterns":
			paths, patterns := findMCPSuspiciousText(arguments, definition.Patterns)
			matched = len(paths) > 0
			if matched {
				evidence["matched_paths"] = paths
				evidence["matched_patterns"] = patterns
			}
		case "output_patterns":
			paths, patterns := findMCPSuspiciousText(result, definition.Patterns)
			matched = len(paths) > 0
			if matched {
				evidence["matched_paths"] = paths
				evidence["matched_patterns"] = patterns
			}
		case "response_size_bytes":
			threshold := 0
			var raw map[string]interface{}
			if json.Unmarshal(rule.Definition, &raw) == nil {
				if value, ok := raw["threshold"].(float64); ok {
					threshold = int(value)
				}
			}
			matched = threshold > 0 && len(resultJSON) > threshold
			if matched {
				evidence["response_size_bytes"] = len(resultJSON)
				evidence["threshold"] = threshold
			}
		case "call_failed":
			matched = callErr != nil
			if matched {
				evidence["failure_class"] = "upstream_call_failed"
			}
		}
		if !matched {
			continue
		}
		evidenceJSON, _ := json.Marshal(evidence)
		evaluation.hits = append(evaluation.hits, model.MCPRuleHit{ID: uuid.New(), InvocationID: invocationID, RuleDefinitionID: rule.ID, Severity: rule.Severity, Phase: phase, Evidence: datatypes.JSON(evidenceJSON), CreatedAt: time.Now().UTC()})
		evaluation.evidence = append(evaluation.evidence, evidence)
		evaluation.ruleKeys = append(evaluation.ruleKeys, rule.RuleKey)
		if mcpSeverityRank(rule.Severity) > mcpSeverityRank(evaluation.severity) {
			evaluation.severity = rule.Severity
		}
		if definition.Action == "block" {
			evaluation.blocked = true
		}
	}
	return evaluation, nil
}

func (s *MCPPlatformService) persistMCPSecurityEvaluation(ctx context.Context, invocationID uuid.UUID, invocationStatus, resultDigest string, completedAt time.Time, evaluation mcpSecurityEvaluation) error {
	ruleStatus := "safe"
	if len(evaluation.hits) > 0 {
		ruleStatus = "matched"
	}
	if evaluation.blocked {
		ruleStatus = "blocked"
	}
	evidence := evaluation.evidence
	if len(evidence) == 0 {
		evidence = []map[string]interface{}{{"type": "deterministic_evaluation", "result": "no_rule_matched"}}
	}
	evidenceJSON, _ := json.Marshal(evidence)
	verdict := &model.MCPSecurityVerdict{ID: uuid.New(), InvocationID: invocationID, DeterministicSeverity: evaluation.severity, AIVerdict: "not_run", OverallRisk: evaluation.severity, Evidence: datatypes.JSON(evidenceJSON), UpdatedAt: completedAt}
	return s.repo.SaveSecurityEvaluation(ctx, invocationID, invocationStatus, ruleStatus, "not_run", resultDigest, completedAt, evaluation.hits, verdict)
}

func mergeMCPSecurityEvaluations(left, right mcpSecurityEvaluation) mcpSecurityEvaluation {
	result := mcpSecurityEvaluation{
		severity: left.severity, blocked: left.blocked || right.blocked,
		hits:     append(append([]model.MCPRuleHit{}, left.hits...), right.hits...),
		evidence: append(append([]map[string]interface{}{}, left.evidence...), right.evidence...),
		ruleKeys: append(append([]string{}, left.ruleKeys...), right.ruleKeys...),
	}
	if mcpSeverityRank(right.severity) > mcpSeverityRank(result.severity) {
		result.severity = right.severity
	}
	return result
}

func mcpRiskRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "l4":
		return 4
	case "l3":
		return 3
	case "l2":
		return 2
	case "l1":
		return 1
	default:
		return 0
	}
}

func mcpSeverityRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func findMCPSensitiveKeyPaths(value interface{}, keys []string, path string) []string {
	sensitive := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		sensitive[strings.ToLower(strings.TrimSpace(key))] = struct{}{}
	}
	paths := make([]string, 0)
	var walk func(interface{}, string)
	walk = func(current interface{}, currentPath string) {
		switch typed := current.(type) {
		case map[string]interface{}:
			for key, child := range typed {
				childPath := currentPath + "." + key
				normalized := strings.ToLower(strings.TrimSpace(key))
				for candidate := range sensitive {
					if normalized == candidate || strings.Contains(normalized, candidate) {
						paths = append(paths, childPath)
						break
					}
				}
				walk(child, childPath)
			}
		case []interface{}:
			for index, child := range typed {
				walk(child, fmt.Sprintf("%s[%d]", currentPath, index))
			}
		}
	}
	walk(value, path)
	if len(paths) > 20 {
		paths = paths[:20]
	}
	return paths
}

func findMCPSuspiciousText(value interface{}, patterns []string) ([]string, []string) {
	normalizedPatterns := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		if normalized := strings.ToLower(strings.TrimSpace(pattern)); normalized != "" {
			normalizedPatterns = append(normalizedPatterns, normalized)
		}
	}
	paths := make([]string, 0)
	matchedPatterns := make([]string, 0)
	seenPattern := make(map[string]struct{})
	var walk func(interface{}, string)
	walk = func(current interface{}, path string) {
		if len(paths) >= 20 {
			return
		}
		switch typed := current.(type) {
		case map[string]interface{}:
			for key, child := range typed {
				walk(child, path+"."+key)
			}
		case []interface{}:
			for index, child := range typed {
				walk(child, fmt.Sprintf("%s[%d]", path, index))
			}
		case string:
			lower := strings.ToLower(typed)
			pathMatched := false
			for _, pattern := range normalizedPatterns {
				if !strings.Contains(lower, pattern) {
					continue
				}
				pathMatched = true
				if _, exists := seenPattern[pattern]; !exists {
					seenPattern[pattern] = struct{}{}
					matchedPatterns = append(matchedPatterns, pattern)
				}
			}
			if pathMatched {
				paths = append(paths, path)
			}
		}
	}
	walk(value, "$")
	if len(matchedPatterns) > 20 {
		matchedPatterns = matchedPatterns[:20]
	}
	return paths, matchedPatterns
}

func mustURL(raw string) *url.URL {
	parsed, _ := url.Parse(raw)
	return parsed
}
