package grpc_server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	pb "server/pkg/api/v1"

	"github.com/google/uuid"
	"google.golang.org/grpc"
)

type captureConfigAgentClient struct {
	pb.AgentServiceClient
	requests []*pb.ConfigSyncRequest
	response *pb.ConfigSyncResponse
	err      error
}

func (c *captureConfigAgentClient) SyncConfig(
	_ context.Context,
	request *pb.ConfigSyncRequest,
	_ ...grpc.CallOption,
) (*pb.ConfigSyncResponse, error) {
	c.requests = append(c.requests, request)
	if c.err != nil {
		return nil, c.err
	}
	if c.response != nil {
		return c.response, nil
	}
	return &pb.ConfigSyncResponse{
		Success: true,
		Applied: map[string]bool{"agent_guard_bundle": true},
	}, nil
}

func TestSyncAgentGuardBundleCachesAndForwardsThroughStream(t *testing.T) {
	hostID := uuid.New()
	stream := &captureAgentStream{}
	server := &GRPCServer{}
	server.agentConnections.Store(hostID, &AgentConnection{HostID: hostID, Stream: stream})
	impl := &APIServerToServerImpl{grpcServer: server}
	digest := agentGuardTestDigest(7)
	payload := agentGuardBundlePayload(hostID, 7, digest)

	response, err := impl.SyncAgentConfig(context.Background(), &pb.SyncAgentConfigRequest{
		HostId: hostID.String(),
		Configs: []*pb.AgentConfig{{
			ConfigType: "agent_guard_bundle",
			ConfigJson: payload,
		}},
	})
	if err != nil {
		t.Fatalf("SyncAgentConfig: %v", err)
	}
	if !response.Success || response.AffectedAgents != 1 {
		t.Fatalf("response = %#v", response)
	}
	if len(stream.sent) != 1 {
		t.Fatalf("stream sends = %d, want 1", len(stream.sent))
	}
	config := stream.sent[0].GetConfigSync()
	if config == nil || config.ConfigType != "agent_guard_bundle" ||
		config.Action != "full_sync" || config.Payload != payload {
		t.Fatalf("bundle changed during stream forwarding: %#v", config)
	}
	cached, ok := server.loadAgentGuardBundle(hostID)
	if !ok || cached.Version != 7 || cached.Digest != digest ||
		cached.Config.Payload != payload {
		t.Fatalf("unexpected cached bundle: %#v, ok=%v", cached, ok)
	}
}

func TestSyncAgentGuardBundleUsesCallbackWhenStreamUnavailable(t *testing.T) {
	hostID := uuid.New()
	client := &captureConfigAgentClient{}
	server := &GRPCServer{}
	server.agentConnections.Store(hostID, &AgentConnection{HostID: hostID, CallbackClient: client})
	impl := &APIServerToServerImpl{grpcServer: server}

	response, err := impl.SyncAgentConfig(context.Background(), &pb.SyncAgentConfigRequest{
		HostId: hostID.String(),
		Configs: []*pb.AgentConfig{{
			ConfigType: "agent_guard_bundle",
			ConfigJson: agentGuardBundlePayload(hostID, 8, agentGuardTestDigest(8)),
		}},
	})
	if err != nil {
		t.Fatalf("SyncAgentConfig: %v", err)
	}
	if !response.Success || response.AffectedAgents != 1 || len(client.requests) != 1 {
		t.Fatalf("callback response=%#v requests=%d", response, len(client.requests))
	}
}

func TestSyncAgentGuardBundlePropagatesCallbackRejectionAndRetainsReconnectCache(t *testing.T) {
	hostID := uuid.New()
	client := &captureConfigAgentClient{
		response: &pb.ConfigSyncResponse{
			Success: false,
			Message: "bundle rejected by agent",
			Applied: map[string]bool{"agent_guard_bundle": false},
		},
	}
	server := &GRPCServer{}
	server.agentConnections.Store(hostID, &AgentConnection{HostID: hostID, CallbackClient: client})
	impl := &APIServerToServerImpl{grpcServer: server}
	payload := agentGuardBundlePayload(hostID, 9, agentGuardTestDigest(9))

	response, err := impl.SyncAgentConfig(context.Background(), &pb.SyncAgentConfigRequest{
		HostId: hostID.String(),
		Configs: []*pb.AgentConfig{{
			ConfigType: "agent_guard_bundle",
			ConfigJson: payload,
		}},
	})
	if err != nil {
		t.Fatalf("SyncAgentConfig: %v", err)
	}
	if response.Success || response.Message != "bundle rejected by agent" {
		t.Fatalf("callback rejection was not preserved: %#v", response)
	}
	cached, ok := server.loadAgentGuardBundle(hostID)
	if !ok || cached.Config.Payload != payload {
		t.Fatalf("rejected delivery was not retained for reconnect: %#v, ok=%v", cached, ok)
	}
}

func TestOfflineAgentGuardBundleIsCachedAndResentOnReconnect(t *testing.T) {
	hostID := uuid.New()
	server := &GRPCServer{}
	impl := &APIServerToServerImpl{grpcServer: server}
	payload := agentGuardBundlePayload(hostID, 9, agentGuardTestDigest(9))

	response, err := impl.SyncAgentConfig(context.Background(), &pb.SyncAgentConfigRequest{
		HostId: hostID.String(),
		Configs: []*pb.AgentConfig{{
			ConfigType: "agent_guard_bundle",
			ConfigJson: payload,
		}},
	})
	if err != nil {
		t.Fatalf("SyncAgentConfig: %v", err)
	}
	if !response.Success || response.AffectedAgents != 0 {
		t.Fatalf("offline bundle was not accepted as pending reconnect: %#v", response)
	}
	reconnectConfigs := server.configsForAgent(hostID)
	if len(reconnectConfigs) != 1 ||
		reconnectConfigs[0].ConfigType != "agent_guard_bundle" ||
		reconnectConfigs[0].Payload != payload {
		t.Fatalf("startup config set does not include cached bundle: %#v", reconnectConfigs)
	}

	stream := &captureAgentStream{}
	sent, err := server.dispatchCachedAgentGuardBundle(
		context.Background(),
		hostID,
		&AgentConnection{HostID: hostID, Stream: stream},
	)
	if err != nil || !sent {
		t.Fatalf("dispatchCachedAgentGuardBundle sent=%v err=%v", sent, err)
	}
	if len(stream.sent) != 1 || stream.sent[0].GetConfigSync().Payload != payload {
		t.Fatalf("cached bundle was not resent unchanged: %#v", stream.sent)
	}
}

func TestReconnectNeverSendsAnEmptyAgentGuardBundle(t *testing.T) {
	server := &GRPCServer{}
	stream := &captureAgentStream{}
	sent, err := server.dispatchCachedAgentGuardBundle(
		context.Background(),
		uuid.New(),
		&AgentConnection{Stream: stream},
	)
	if err != nil || sent || len(stream.sent) != 0 {
		t.Fatalf("empty cache dispatch sent=%v err=%v requests=%d", sent, err, len(stream.sent))
	}
}

func TestStaleAgentGuardBundleCannotReplaceReconnectCache(t *testing.T) {
	hostID := uuid.New()
	server := &GRPCServer{}
	impl := &APIServerToServerImpl{grpcServer: server}
	syncBundle := func(version int64, digest string) *pb.SyncAgentConfigResponse {
		t.Helper()
		response, err := impl.SyncAgentConfig(context.Background(), &pb.SyncAgentConfigRequest{
			HostId: hostID.String(),
			Configs: []*pb.AgentConfig{{
				ConfigType: "agent_guard_bundle",
				ConfigJson: agentGuardBundlePayload(hostID, version, digest),
			}},
		})
		if err != nil {
			t.Fatalf("SyncAgentConfig version %d: %v", version, err)
		}
		return response
	}

	digest11 := agentGuardTestDigest(11)
	if response := syncBundle(11, digest11); !response.Success {
		t.Fatalf("initial bundle rejected: %#v", response)
	}
	if response := syncBundle(10, agentGuardTestDigest(10)); response.Success {
		t.Fatalf("stale bundle accepted: %#v", response)
	}
	mutatedPayload := strings.TrimSuffix(
		agentGuardBundlePayload(hostID, 11, digest11),
		"}",
	) + `,"mutated":true}`
	response, err := impl.SyncAgentConfig(context.Background(), &pb.SyncAgentConfigRequest{
		HostId: hostID.String(),
		Configs: []*pb.AgentConfig{{
			ConfigType: "agent_guard_bundle",
			ConfigJson: mutatedPayload,
		}},
	})
	if err != nil {
		t.Fatalf("SyncAgentConfig conflicting version: %v", err)
	}
	if response.Success {
		t.Fatalf("same version/digest with different payload accepted: %#v", response)
	}
	cached, ok := server.loadAgentGuardBundle(hostID)
	if !ok || cached.Version != 11 || cached.Digest != digest11 {
		t.Fatalf("stale bundle replaced cache: %#v, ok=%v", cached, ok)
	}
}

func TestInvalidOrBroadcastAgentGuardBundleIsRejectedBeforeDispatch(t *testing.T) {
	hostID := uuid.New()
	tests := []struct {
		name    string
		hostID  string
		payload string
	}{
		{
			name:    "broadcast",
			hostID:  "",
			payload: agentGuardBundlePayload(hostID, 1, agentGuardTestDigest(1)),
		},
		{
			name:   "invalid schema",
			hostID: hostID.String(),
			payload: strings.Replace(
				agentGuardBundlePayload(hostID, 1, agentGuardTestDigest(1)),
				"aegis.agent_guard.bundle.v1",
				"aegis.agent_guard.bundle.v2",
				1,
			),
		},
		{
			name:    "invalid version",
			hostID:  hostID.String(),
			payload: agentGuardBundlePayload(hostID, 0, agentGuardTestDigest(0)),
		},
		{
			name:    "invalid digest",
			hostID:  hostID.String(),
			payload: agentGuardBundlePayload(hostID, 1, "sha256:not-a-digest"),
		},
		{
			name:    "host mismatch",
			hostID:  hostID.String(),
			payload: agentGuardBundlePayload(uuid.New(), 1, agentGuardTestDigest(1)),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := &GRPCServer{}
			impl := &APIServerToServerImpl{grpcServer: server}
			response, err := impl.SyncAgentConfig(context.Background(), &pb.SyncAgentConfigRequest{
				HostId: test.hostID,
				Configs: []*pb.AgentConfig{{
					ConfigType: "agent_guard_bundle",
					ConfigJson: test.payload,
				}},
			})
			if err != nil {
				t.Fatalf("SyncAgentConfig: %v", err)
			}
			if response.Success {
				t.Fatalf("invalid bundle accepted: %#v", response)
			}
			if _, ok := server.loadAgentGuardBundle(hostID); ok {
				t.Fatal("invalid bundle entered reconnect cache")
			}
		})
	}
}

func TestConcurrentAgentGuardBundleCacheNeverRegressesVersion(t *testing.T) {
	hostID := uuid.New()
	server := &GRPCServer{}
	const maxVersion = 32
	start := make(chan struct{})
	errs := make(chan error, maxVersion)
	var wait sync.WaitGroup
	for version := int64(1); version <= maxVersion; version++ {
		version := version
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := server.cacheAgentGuardBundle(hostID, &pb.ConfigSync{
				ConfigType: "agent_guard_bundle",
				Action:     "full_sync",
				Payload: agentGuardBundlePayload(
					hostID,
					version,
					agentGuardTestDigest(version),
				),
			})
			if err != nil && !errors.Is(err, errAgentGuardBundleStale) {
				errs <- err
			}
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("cache update: %v", err)
	}
	cached, ok := server.loadAgentGuardBundle(hostID)
	if !ok || cached.Version != maxVersion {
		t.Fatalf("concurrent cache regressed: %#v, ok=%v", cached, ok)
	}
}

func agentGuardBundlePayload(hostID uuid.UUID, version int64, digest string) string {
	return fmt.Sprintf(
		`{"schema":"aegis.agent_guard.bundle.v1","bundle_version":%d,"host_id":%q,"digest":%q,"profiles":[],"policies":[]}`,
		version,
		hostID.String(),
		digest,
	)
}

func agentGuardTestDigest(version int64) string {
	return fmt.Sprintf("sha256:%064x", version)
}
