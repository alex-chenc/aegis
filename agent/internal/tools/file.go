package tools

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
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

type FileContent struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	ReadSize  int64  `json:"read_size"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

var sensitivePaths = []string{
	"/etc/shadow",
	"/etc/gshadow",
	"/etc/ssh",
	"/root/.ssh",
	"/var/lib/postgresql",
	"/var/lib/mysql",
	"/var/lib/redis",
	"/var/lib/mongodb",
	"/home",
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

func (m *ToolManager) ReadFileContent(filePath string, maxSize int64) (*FileContent, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}

	if isSensitivePath(absPath) {
		return nil, errors.New("access denied: sensitive path")
	}

	stat, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	if stat.IsDir() {
		return nil, errors.New("cannot read directory")
	}

	file, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	readSize := stat.Size()
	truncated := false
	if readSize > maxSize {
		readSize = maxSize
		truncated = true
	}

	buf := make([]byte, readSize)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return nil, err
	}

	return &FileContent{
		Path:      filePath,
		Size:      stat.Size(),
		ReadSize:  int64(n),
		Content:   string(buf[:n]),
		Truncated: truncated,
	}, nil
}

func isSensitivePath(path string) bool {
	for _, sp := range sensitivePaths {
		if strings.HasPrefix(path, sp) {
			return true
		}
	}
	return false
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
