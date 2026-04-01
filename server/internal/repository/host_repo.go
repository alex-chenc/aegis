package repository

import (
	"time"

	"server/internal/model"
	"server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type HostRepository struct {
	db *gorm.DB
}

func NewHostRepository(db *gorm.DB) *HostRepository {
	return &HostRepository{db: db}
}

func (r *HostRepository) Upsert(host *model.Host) error {
	result := r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "ip_address"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"hostname", "os_type", "agent_version", "last_heartbeat_at", "updated_at",
		}),
	}).Create(host)

	if result.Error != nil {
		logger.Error("failed to upsert host", zap.Error(result.Error), zap.String("ip", host.IPAddress))
		return result.Error
	}

	logger.Info("host upserted successfully",
		zap.String("id", host.ID.String()),
		zap.String("ip", host.IPAddress),
		zap.String("hostname", host.Hostname),
	)
	return nil
}

func (r *HostRepository) UpdateHeartbeat(hostID uuid.UUID) error {
	result := r.db.Model(&model.Host{}).
		Where("id = ?", hostID).
		Updates(map[string]interface{}{
			"last_heartbeat_at": time.Now(),
			"updated_at":        time.Now(),
		})

	if result.Error != nil {
		logger.Error("failed to update heartbeat", zap.Error(result.Error), zap.String("host_id", hostID.String()))
		return result.Error
	}

	logger.Debug("heartbeat updated", zap.String("host_id", hostID.String()))
	return nil
}

func (r *HostRepository) FindAll(page, pageSize int, query string) ([]model.Host, error) {
	var hosts []model.Host
	offset := (page - 1) * pageSize

	db := r.db.Model(&model.Host{})
	if query != "" {
		searchPattern := "%" + query + "%"
		db = db.Where("ip_address LIKE ? OR hostname LIKE ?", searchPattern, searchPattern)
	}

	result := db.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&hosts)
	if result.Error != nil {
		logger.Error("failed to find hosts", zap.Error(result.Error))
		return nil, result.Error
	}

	logger.Debug("hosts found", zap.Int("count", len(hosts)), zap.Int("page", page))
	return hosts, nil
}

func (r *HostRepository) FindByID(id uuid.UUID) (*model.Host, error) {
	var host model.Host
	result := r.db.First(&host, "id = ?", id)
	if result.Error != nil {
		logger.Error("failed to find host by id", zap.Error(result.Error), zap.String("id", id.String()))
		return nil, result.Error
	}
	return &host, nil
}

func (r *HostRepository) FindByIP(ipAddress string) (*model.Host, error) {
	var host model.Host
	result := r.db.First(&host, "ip_address = ?", ipAddress)
	if result.Error != nil {
		return nil, result.Error
	}
	return &host, nil
}

func (r *HostRepository) Count(query string) (int64, error) {
	var count int64

	db := r.db.Model(&model.Host{})
	if query != "" {
		searchPattern := "%" + query + "%"
		db = db.Where("ip_address LIKE ? OR hostname LIKE ?", searchPattern, searchPattern)
	}

	result := db.Count(&count)
	if result.Error != nil {
		logger.Error("failed to count hosts", zap.Error(result.Error))
		return 0, result.Error
	}

	return count, nil
}
