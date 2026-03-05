package repository

import (
	"database/sql"

	"baseline-system/internal/model"
)

type HostRepository struct {
	db *sql.DB
}

func NewHostRepository(db *sql.DB) *HostRepository {
	return &HostRepository{db: db}
}

func (r *HostRepository) Upsert(host *model.Host) error {
	query := `
		INSERT INTO hosts (ip_address, hostname, os_type, agent_version, last_heartbeat_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (ip_address) DO UPDATE SET
			hostname = EXCLUDED.hostname,
			os_type = EXCLUDED.os_type,
			agent_version = EXCLUDED.agent_version,
			last_heartbeat_at = EXCLUDED.last_heartbeat_at,
			updated_at = NOW()
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRow(
		query,
		host.IPAddress, host.Hostname, host.OSType, host.AgentVersion, host.LastHeartbeatAt,
	).Scan(&host.ID, &host.CreatedAt, &host.UpdatedAt)
}

func (r *HostRepository) UpdateHeartbeat(hostID string) error {
	query := `UPDATE hosts SET last_heartbeat_at = NOW(), updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(query, hostID)
	return err
}

func (r *HostRepository) FindAll(page, pageSize int, query string) ([]model.Host, error) {
	offset := (page - 1) * pageSize
	sqlQuery := `
		SELECT id, ip_address, hostname, os_type, agent_version, last_heartbeat_at, created_at, updated_at
		FROM hosts
		WHERE ($1 = '' OR ip_address LIKE $1 OR hostname LIKE $1)
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	searchPattern := "%" + query + "%"
	rows, err := r.db.Query(sqlQuery, searchPattern, pageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hosts []model.Host
	for rows.Next() {
		var h model.Host
		if err := rows.Scan(
			&h.ID, &h.IPAddress, &h.Hostname, &h.OSType, &h.AgentVersion,
			&h.LastHeartbeatAt, &h.CreatedAt, &h.UpdatedAt,
		); err != nil {
			return nil, err
		}
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

func (r *HostRepository) FindByID(id string) (*model.Host, error) {
	query := `
		SELECT id, ip_address, hostname, os_type, agent_version, last_heartbeat_at, created_at, updated_at
		FROM hosts WHERE id = $1
	`
	var h model.Host
	err := r.db.QueryRow(query, id).Scan(
		&h.ID, &h.IPAddress, &h.Hostname, &h.OSType, &h.AgentVersion,
		&h.LastHeartbeatAt, &h.CreatedAt, &h.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (r *HostRepository) Count(query string) (int, error) {
	sqlQuery := `SELECT COUNT(*) FROM hosts WHERE ($1 = '' OR ip_address LIKE $1 OR hostname LIKE $1)`
	searchPattern := "%" + query + "%"
	var count int
	err := r.db.QueryRow(sqlQuery, searchPattern).Scan(&count)
	return count, err
}
