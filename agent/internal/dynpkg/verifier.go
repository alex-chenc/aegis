package dynpkg

import (
	"crypto/ed25519"
	"fmt"
	"os"
)

// VerifySignature verifies the Ed25519 signature of a package
func VerifySignature(publicKey ed25519.PublicKey, packagePath, signaturePath string) error {
	if len(publicKey) == 0 {
		return fmt.Errorf("signing public key not configured, cannot verify signature")
	}

	packageBytes, err := os.ReadFile(packagePath)
	if err != nil {
		return fmt.Errorf("read package: %w", err)
	}

	sigBytes, err := os.ReadFile(signaturePath)
	if err != nil {
		return fmt.Errorf("read signature: %w", err)
	}

	if !ed25519.Verify(publicKey, packageBytes, sigBytes) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}
