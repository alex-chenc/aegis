package grpc

import (
	"context"
	"errors"
	"strings"
	"testing"

	pb "api-server/pkg/api/v1"

	"google.golang.org/grpc"
)

type fakeAgentGuardConfigRPCClient struct {
	pb.APIServerToServerClient
	response *pb.SyncAgentConfigResponse
	err      error
}

func (f *fakeAgentGuardConfigRPCClient) SyncAgentConfig(
	context.Context,
	*pb.SyncAgentConfigRequest,
	...grpc.CallOption,
) (*pb.SyncAgentConfigResponse, error) {
	return f.response, f.err
}

func TestSyncAgentConfigPreservesServerAcceptanceAndRejection(t *testing.T) {
	tests := []struct {
		name         string
		response     *pb.SyncAgentConfigResponse
		rpcErr       error
		wantAffected int32
		wantError    string
	}{
		{
			name: "accepted",
			response: &pb.SyncAgentConfigResponse{
				Success: true, AffectedAgents: 1, Message: "config sync sent",
			},
			wantAffected: 1,
		},
		{
			name: "agent rejected",
			response: &pb.SyncAgentConfigResponse{
				Success: false, Message: "agent_guard_bundle_digest_mismatch",
			},
			wantError: "agent_guard_bundle_digest_mismatch",
		},
		{
			name:      "transport failure",
			rpcErr:    errors.New("server unavailable"),
			wantError: "server unavailable",
		},
		{
			name:      "nil response",
			wantError: "empty response",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &ServerClient{client: &fakeAgentGuardConfigRPCClient{
				response: test.response,
				err:      test.rpcErr,
			}}
			affected, err := client.SyncAgentConfig(context.Background(), "host-1", []*pb.AgentConfig{{
				ConfigType: "agent_guard_bundle",
				ConfigJson: `{}`,
			}})
			if affected != test.wantAffected {
				t.Fatalf("affected agents = %d, want %d", affected, test.wantAffected)
			}
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("SyncAgentConfig: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("SyncAgentConfig error = %v, want %q", err, test.wantError)
			}
		})
	}
}
