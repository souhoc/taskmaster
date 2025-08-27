package util

import (
	"fmt"
	"os"
	"path/filepath"
)

func GetLogfile() (*os.File, error) {
	dir, err := os.UserCacheDir()
	if err == nil {
		dir = filepath.Join(dir, "taskmaster")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create user cache dir: %w", err)
	}

	filePath := filepath.Join(dir, "taskmaster.log")

	logFile, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to create user log file: %w", err)
	}

	return logFile, nil
}
