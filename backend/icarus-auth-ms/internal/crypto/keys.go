package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LoadOrGenerateKeys loads private and public RSA keys from the given directory.
// If the keys do not exist, they will be generated and saved.
func LoadOrGenerateKeys(keysDir string) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	if err := os.MkdirAll(keysDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("failed to create keys directory: %w", err)
	}

	privPath := filepath.Join(keysDir, "app.rsa")
	pubPath := filepath.Join(keysDir, "app.rsa.pub")

	_, errPriv := os.Stat(privPath)
	_, errPub := os.Stat(pubPath)

	if os.IsNotExist(errPriv) || os.IsNotExist(errPub) {
		// Generate new key pair
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate RSA key: %w", err)
		}

		// Save private key
		privBytes := x509.MarshalPKCS1PrivateKey(privateKey)
		privPem := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: privBytes,
		})
		if err := os.WriteFile(privPath, privPem, 0600); err != nil {
			return nil, nil, fmt.Errorf("failed to write private key: %w", err)
		}

		// Save public key
		pubBytes := x509.MarshalPKCS1PublicKey(&privateKey.PublicKey)
		pubPem := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PUBLIC KEY",
			Bytes: pubBytes,
		})
		if err := os.WriteFile(pubPath, pubPem, 0644); err != nil {
			return nil, nil, fmt.Errorf("failed to write public key: %w", err)
		}
	}

	// Load private key
	privData, err := os.ReadFile(privPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read private key file: %w", err)
	}
	blockPriv, _ := pem.Decode(privData)
	if blockPriv == nil || blockPriv.Type != "RSA PRIVATE KEY" {
		return nil, nil, errors.New("invalid private key PEM")
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(blockPriv.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Load public key
	pubData, err := os.ReadFile(pubPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read public key file: %w", err)
	}
	blockPub, _ := pem.Decode(pubData)
	if blockPub == nil || blockPub.Type != "RSA PUBLIC KEY" {
		return nil, nil, errors.New("invalid public key PEM")
	}
	publicKey, err := x509.ParsePKCS1PublicKey(blockPub.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	return privateKey, publicKey, nil
}
