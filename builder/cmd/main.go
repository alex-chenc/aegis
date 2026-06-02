package main

import (
	"crypto/ed25519"
	"encoding/hex"
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

const embeddedSigningPrivateKeyHex = "9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a"

func main() {
	port := flag.Int("port", 19096, "gRPC port")
	workDir := flag.String("work-dir", "/tmp/aegis-builder", "Working directory")
	keyFile := flag.String("key-file", envOr("BUILDER_KEY_FILE", "/data/builder.key"), "Ed25519 key file path (hex encoded)")
	useKeyFile := flag.Bool("use-key-file", false, "Load signing key from --key-file instead of the embedded V5.8 key")
	minioEndpoint := flag.String("minio-endpoint", envOr("MINIO_ENDPOINT", "minio:9000"), "MinIO endpoint")
	minioAccessKey := flag.String("minio-access-key", envOr("MINIO_ACCESS_KEY", "minio_admin"), "MinIO access key")
	minioSecretKey := flag.String("minio-secret-key", envOr("MINIO_SECRET_KEY", "a_third_strong_secret_password"), "MinIO secret key")
	flag.Parse()

	publicKey, privateKey, err := loadSigningKey(*keyFile, *useKeyFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load signing key pair: %v\n", err)
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

func loadSigningKey(keyFile string, useKeyFile bool) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	if useKeyFile {
		return loadOrGenerateKey(keyFile)
	}

	keyBytes, err := hex.DecodeString(embeddedSigningPrivateKeyHex)
	if err != nil {
		return nil, nil, fmt.Errorf("decode embedded private key: %w", err)
	}
	if len(keyBytes) != ed25519.PrivateKeySize {
		return nil, nil, fmt.Errorf("embedded private key has length %d, expected %d", len(keyBytes), ed25519.PrivateKeySize)
	}

	privateKey := ed25519.PrivateKey(keyBytes)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	fmt.Println("Loaded embedded V5.8 signing key")
	return publicKey, privateKey, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadOrGenerateKey(keyFile string) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	data, err := os.ReadFile(keyFile)
	if err == nil {
		// Try to load existing key (hex encoded private key, 64 bytes)
		keyBytes, err := hex.DecodeString(string(data))
		if err == nil && len(keyBytes) == ed25519.PrivateKeySize {
			privateKey := ed25519.PrivateKey(keyBytes)
			publicKey := privateKey.Public().(ed25519.PublicKey)
			fmt.Printf("Loaded existing key from %s\n", keyFile)
			return publicKey, privateKey, nil
		}
	}

	// Generate new key pair
	publicKey, privateKey, err := signer.GenerateKeyPair()
	if err != nil {
		return nil, nil, fmt.Errorf("generate key pair: %w", err)
	}

	// Save to file
	if err := os.MkdirAll("/data", 0755); err == nil {
		if err := os.WriteFile(keyFile, []byte(hex.EncodeToString(privateKey)), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save key to %s: %v\n", keyFile, err)
		} else {
			fmt.Printf("Generated and saved new key to %s\n", keyFile)
		}
	}

	return publicKey, privateKey, nil
}
