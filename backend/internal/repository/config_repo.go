package repository

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"io"

	"baseline-system/internal/model"
)

type ConfigRepository struct {
	db            *sql.DB
	encryptionKey []byte
}

func NewConfigRepository(db *sql.DB, encryptionKey string) *ConfigRepository {
	key := []byte(encryptionKey)
	if len(key) < 32 {
		padded := make([]byte, 32)
		copy(padded, key)
		key = padded
	}
	return &ConfigRepository{db: db, encryptionKey: key[:32]}
}

func (r *ConfigRepository) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(r.encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (r *ConfigRepository) decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(r.encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, cipherData := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (r *ConfigRepository) GetActive() (*model.LLMConfig, error) {
	query := `
		SELECT id, api_key_encrypted, api_key_masked, base_url, model_name, is_active,
		       last_test_status, last_test_at, created_at, updated_at
		FROM llm_configs WHERE is_active = true LIMIT 1
	`
	var cfg model.LLMConfig
	err := r.db.QueryRow(query).Scan(
		&cfg.ID, &cfg.APIKeyEncrypted, &cfg.APIKeyMasked, &cfg.BaseURL, &cfg.ModelName, &cfg.IsActive,
		&cfg.LastTestStatus, &cfg.LastTestAt, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *ConfigRepository) Upsert(cfg *model.LLMConfig, apiKey string) error {
	encryptedKey, err := r.encrypt(apiKey)
	if err != nil {
		return err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`UPDATE llm_configs SET is_active = false`)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO llm_configs (api_key_encrypted, api_key_masked, base_url, model_name, is_active)
		VALUES ($1, $2, $3, $4, true)
		RETURNING id, created_at, updated_at
	`
	err = tx.QueryRow(
		query,
		encryptedKey, cfg.APIKeyMasked, cfg.BaseURL, cfg.ModelName,
	).Scan(&cfg.ID, &cfg.CreatedAt, &cfg.UpdatedAt)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ConfigRepository) UpdateTestStatus(id string, status string) error {
	query := `UPDATE llm_configs SET last_test_status = $1, last_test_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(query, status, id)
	return err
}

func (r *ConfigRepository) DecryptAPIKey(encrypted string) (string, error) {
	return r.decrypt(encrypted)
}
