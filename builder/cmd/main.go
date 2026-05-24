package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	buildergrpc "builder/internal/grpc"
	"builder/internal/minio"
	"builder/internal/service"
	"builder/internal/signer"
	pb "builder/pkg/api/v1"

	"google.golang.org/grpc"
)

func main() {
	port := flag.Int("port", 19096, "gRPC port")
	workDir := flag.String("work-dir", "/tmp/aegis-builder", "Working directory")
	minioEndpoint := flag.String("minio-endpoint", envOr("MINIO_ENDPOINT", "minio:9000"), "MinIO endpoint")
	minioAccessKey := flag.String("minio-access-key", envOr("MINIO_ACCESS_KEY", "minio_admin"), "MinIO access key")
	minioSecretKey := flag.String("minio-secret-key", envOr("MINIO_SECRET_KEY", "a_third_strong_secret_password"), "MinIO secret key")
	flag.Parse()

	publicKey, privateKey, err := signer.GenerateKeyPair()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate key pair: %v\n", err)
		os.Exit(1)
	}

	signerInst := signer.NewSigner(privateKey)

	minioClient, err := minio.NewClient(minio.Config{
		Endpoint:  *minioEndpoint,
		AccessKey: *minioAccessKey,
		SecretKey: *minioSecretKey,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create MinIO client: %v\n", err)
		os.Exit(1)
	}

	builderService := service.NewBuilderService(signerInst, *workDir, minioClient)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen: %v\n", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	builderGRPCServer := buildergrpc.NewBuilderGRPCServer(builderService)
	pb.RegisterBuilderServiceServer(grpcServer, builderGRPCServer)

	fmt.Printf("Builder server starting on port %d\n", *port)
	fmt.Printf("Public key fingerprint: %s\n", signerInst.GetPublicKeyFingerprint())
	fmt.Printf("Public key (hex for agent config): %x\n", publicKey)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to serve: %v\n", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("Shutting down...")
	grpcServer.GracefulStop()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
