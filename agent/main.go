package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	pb "ai-benchmark/agent/proto/agent_comm"
)

var (
	serverAddr    = flag.String("server_addr", "127.0.0.1:9090", "The server address in the format of host:port")
	authToken     = flag.String("token", "", "Authentication token for server connection")
	configDir     = flag.String("config_dir", "/etc/agent", "Directory to store agent configuration")
	version       = "1.0.0"
	hostID        string
	agentConfig   *Config
	maxRetryDelay = 5 * time.Minute
)

type Config struct {
	HostID    string `json:"host_id"`
	CreatedAt string `json:"created_at"`
}

func main() {
	flag.Parse()

	hostID = loadOrGenerateHostID()

	log.Printf("Agent starting. HostID: %s, Connecting to %s", hostID, *serverAddr)

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	wg.Add(1)
	go func() {
		defer wg.Done()
		runWithReconnect(ctx)
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("Agent shutting down...")
	cancel()
	wg.Wait()
	log.Println("Agent stopped")
}

func runWithReconnect(ctx context.Context) {
	retryDelay := 1 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := connectAndRun(ctx)
		if err != nil {
			log.Printf("Connection error: %v", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(retryDelay):
		}

		retryDelay = retryDelay * 2
		if retryDelay > maxRetryDelay {
			retryDelay = maxRetryDelay
		}
		log.Printf("Reconnecting in %v...", retryDelay)
	}
}

func connectAndRun(ctx context.Context) error {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	if *authToken != "" {
		opts = append(opts, grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+*authToken)
			return invoker(ctx, method, req, reply, cc, opts...)
		}))
		opts = append(opts, grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+*authToken)
			return streamer(ctx, desc, cc, method, opts...)
		}))
	}

	conn, err := grpc.Dial(*serverAddr, opts...)
	if err != nil {
		return fmt.Errorf("fail to dial: %w", err)
	}
	defer conn.Close()

	client := pb.NewAgentServiceClient(conn)

	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	stream, err := client.Register(streamCtx)
	if err != nil {
		return fmt.Errorf("failed to register: %w", err)
	}

	log.Printf("Connected to server %s", *serverAddr)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		runHeartbeat(streamCtx, stream)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		runAssetCollection(streamCtx, stream)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		receiveCommands(streamCtx, stream)
	}()

	wg.Wait()

	return nil
}

func loadOrGenerateHostID() string {
	configPath := filepath.Join(*configDir, "agent_config.json")

	data, err := os.ReadFile(configPath)
	if err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err == nil && cfg.HostID != "" {
			agentConfig = &cfg
			return cfg.HostID
		}
	}

	newHostID := generateHostID()
	cfg := Config{
		HostID:    newHostID,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	agentConfig = &cfg

	if err := os.MkdirAll(*configDir, 0755); err != nil {
		log.Printf("Warning: failed to create config directory: %v", err)
	} else {
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			log.Printf("Warning: failed to marshal config: %v", err)
		} else if err := os.WriteFile(configPath, data, 0644); err != nil {
			log.Printf("Warning: failed to write config file: %v", err)
		}
	}

	return newHostID
}

func generateHostID() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	timestamp := time.Now().Unix()
	return fmt.Sprintf("%s-%d", hostname, timestamp)
}
