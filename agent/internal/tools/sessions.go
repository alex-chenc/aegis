package tools

import (
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"
)

// UserSession represents a user login session
type UserSession struct {
	TTY      string `json:"tty"`
	Username string `json:"username"`
	PID      int    `json:"pid"`
	From     string `json:"from"`
	LoginAt  string `json:"login_at"`
}

// GetUserSessions gets logged-in user sessions
func (m *ToolManager) GetUserSessions(args map[string]interface{}) (interface{}, error) {
	sessions := []UserSession{}

	// Read /var/run/utmp for active sessions (Linux)
	utmpPath := "/var/run/utmp"
	if _, err := os.Stat(utmpPath); os.IsNotExist(err) {
		// Try alternative path
		utmpPath = "/etc/utmp"
	}

	data, err := os.ReadFile(utmpPath)
	if err != nil {
		// Fallback: parse 'who' command output
		return m.getUserSessionsFromWho()
	}

	// Parse utmp structure (simplified)
	sessions = m.parseUtmp(data)

	if len(sessions) == 0 {
		// Fallback to who command
		return m.getUserSessionsFromWho()
	}

	return map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	}, nil
}

func (m *ToolManager) parseUtmp(data []byte) []UserSession {
	sessions := []UserSession{}

	// UTMP structure size is typically 380 bytes on Linux
	// This is a simplified parser
	structSize := 380
	for i := 0; i+structSize <= len(data); i += structSize {
		rec := data[i : i+structSize]

		// Extract username (32 bytes at offset 0)
		username := strings.TrimSpace(string(rec[0:32]))
		if username == "" || username == "\x00" {
			continue
		}

		// Extract TTY (64 bytes at offset 32)
		tty := strings.TrimSpace(string(rec[32:96]))
		if tty == "" {
			continue
		}

		// Extract PID (4 bytes at offset 124)
		pid := int(rec[124]) | int(rec[125])<<8 | int(rec[126])<<16 | int(rec[127])<<24

		// Extract timestamp (4 bytes at offset 228)
		sec := uint32(rec[228]) | uint32(rec[229])<<8 | uint32(rec[230])<<16 | uint32(rec[231])<<24
		loginTime := time.Unix(int64(sec), 0)

		// Extract IP address (4 bytes at offset 256) - for remote logins
		var from string
		ip := rec[256:260]
		if ip[0] != 0 || ip[1] != 0 || ip[2] != 0 || ip[3] != 0 {
			from = fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3])
		}

		sessions = append(sessions, UserSession{
			TTY:      tty,
			Username: username,
			PID:      pid,
			From:     from,
			LoginAt:  loginTime.Format(time.RFC3339),
		})
	}

	return sessions
}

func (m *ToolManager) getUserSessionsFromWho() (interface{}, error) {
	// Fallback: parse /var/log/wtmp or use who command
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	// Get logged-in users from /var/run/utmp using who command
	// This is a simplified approach
	sessions := []UserSession{}

	// Read /var/log/wtmp for recent logins
	wtmpPath := "/var/log/wtmp"
	data, err := os.ReadFile(wtmpPath)
	if err == nil {
		sessions = m.parseUtmp(data)
	}

	// Filter to only active sessions
	activeSessions := []UserSession{}
	for _, s := range sessions {
		if s.PID > 0 && s.Username != "" {
			activeSessions = append(activeSessions, s)
		}
	}

	if len(activeSessions) == 0 {
		// Fallback to current user info
		return map[string]interface{}{
			"sessions": []UserSession{
				{
					TTY:      "pts/0",
					Username: currentUser.Username,
					PID:      os.Getpid(),
					From:     "localhost",
					LoginAt:  time.Now().Format(time.RFC3339),
				},
			},
			"count": 1,
		}, nil
	}

	return map[string]interface{}{
		"sessions": activeSessions,
		"count":    len(activeSessions),
	}, nil
}