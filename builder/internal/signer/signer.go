package signer

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
)

type Signer struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

func NewSigner(privateKey ed25519.PrivateKey) *Signer {
	return &Signer{
		privateKey: privateKey,
		publicKey:  privateKey.Public().(ed25519.PublicKey),
	}
}

func GenerateKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key pair: %w", err)
	}
	return publicKey, privateKey, nil
}

func (s *Signer) SignFile(filePath string) ([]byte, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	signature := ed25519.Sign(s.privateKey, data)
	return signature, nil
}

func (s *Signer) SignBytes(data []byte) []byte {
	return ed25519.Sign(s.privateKey, data)
}

func (s *Signer) GetPublicKey() ed25519.PublicKey {
	return s.publicKey
}

func (s *Signer) GetPublicKeyFingerprint() string {
	return fmt.Sprintf("%x", s.publicKey)
}

func Verify(publicKey ed25519.PublicKey, data, signature []byte) bool {
	return ed25519.Verify(publicKey, data, signature)
}
