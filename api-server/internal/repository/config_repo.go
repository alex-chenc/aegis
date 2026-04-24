package repository

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"

	"api-server/internal/model"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ConfigRepository struct {
	db            *gorm.DB
	encryptionKey []byte
}

func NewConfigRepository(db *gorm.DB, encryptionKey string) *ConfigRepository {
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
	var cfg model.LLMConfig
	result := r.db.Where("is_active = ?", true).First(&cfg)
	if result.Error != nil {
		logger.Error("failed to get active LLM config", zap.Error(result.Error))
		return nil, result.Error
	}

	logger.Debug("active LLM config retrieved", zap.String("id", cfg.ID.String()))
	return &cfg, nil
}

func (r *ConfigRepository) Upsert(cfg *model.LLMConfig, apiKey string) error {
	encryptedKey, err := r.encrypt(apiKey)
	if err != nil {
		logger.Error("failed to encrypt API key", zap.Error(err))
		return err
	}

	err = r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.LLMConfig{}).Where("1 = 1").Update("is_active", false).Error; err != nil {
			return err
		}

		cfg.APIKeyEncrypted = encryptedKey
		if err := tx.Create(cfg).Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		logger.Error("failed to upsert LLM config", zap.Error(err))
		return err
	}

	logger.Info("LLM config upserted successfully",
		zap.String("id", cfg.ID.String()),
		zap.String("provider", cfg.Provider),
		zap.String("base_url", cfg.BaseURL),
		zap.String("model_name", cfg.ModelName),
	)
	return nil
}

func (r *ConfigRepository) UpdateTestStatus(id uuid.UUID, status string) error {
	result := r.db.Model(&model.LLMConfig{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"last_test_status": status,
			"last_test_at":     gorm.Expr("NOW()"),
		})

	if result.Error != nil {
		logger.Error("failed to update test status",
			zap.Error(result.Error),
			zap.String("id", id.String()),
		)
		return result.Error
	}

	logger.Info("LLM config test status updated",
		zap.String("id", id.String()),
		zap.String("status", status),
	)
	return nil
}

func (r *ConfigRepository) DecryptAPIKey(encrypted string) (string, error) {
	return r.decrypt(encrypted)
}
