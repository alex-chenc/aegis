package tools

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"time"
)

type FileInfo struct {
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	Permissions string `json:"permissions"`
	Owner       string `json:"owner"`
	Group       string `json:"group"`
	ModifiedAt  string `json:"modified_at"`
	MD5         string `json:"md5"`
	IsSUID      bool   `json:"is_suid"`
	IsSGID      bool   `json:"is_sgid"`
}

func (m *ToolManager) GetFileInfo(filePath string) (*FileInfo, error) {
	stat, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	owner := ""
	group := ""
	if sys, ok := stat.Sys().(*syscall.Stat_t); ok {
		if u, err := user.LookupId(strconv.Itoa(int(sys.Uid))); err == nil {
			owner = u.Username
		}
		if g, err := user.LookupGroupId(strconv.Itoa(int(sys.Gid))); err == nil {
			group = g.Name
		}
	}

	mode := stat.Mode()
	return &FileInfo{
		Path:        filePath,
		Size:        stat.Size(),
		Permissions: mode.String(),
		Owner:       owner,
		Group:       group,
		ModifiedAt:  stat.ModTime().Format(time.RFC3339),
		MD5:         fileMD5(filePath),
		IsSUID:      mode&os.ModeSetuid != 0,
		IsSGID:      mode&os.ModeSetgid != 0,
	}, nil
}

func fileMD5(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}
