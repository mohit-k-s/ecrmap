package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ecrmap/pkg/model"
)

// FileCache manages local caching of ECS discovery snapshots.
type FileCache struct {
	baseDir string
}

// New creates a FileCache using the user's cache directory (or ~/.cache/ecrmap).
func New() (*FileCache, error) {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		userCacheDir = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	dir := filepath.Join(userCacheDir, "ecrmap")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory %s: %w", dir, err)
	}
	return &FileCache{baseDir: dir}, nil
}

func (c *FileCache) getFilePath(profile, region string) string {
	if profile == "" {
		profile = "default"
	}
	if region == "" {
		region = "default"
	}
	filename := fmt.Sprintf("snapshot_%s_%s.json", profile, region)
	return filepath.Join(c.baseDir, filename)
}

// Get retrieves a snapshot from cache if it exists and is within maxAge.
func (c *FileCache) Get(profile, region string, maxAge time.Duration) (*model.Snapshot, bool, error) {
	path := c.getFilePath(profile, region)
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	if maxAge > 0 && time.Since(info.ModTime()) > maxAge {
		// Cache expired
		return nil, false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}

	var snapshot model.Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, false, nil // Corrupt cache, treat as cache miss
	}

	return &snapshot, true, nil
}

// Put writes a snapshot to the cache file.
func (c *FileCache) Put(profile, region string, snapshot *model.Snapshot) error {
	path := c.getFilePath(profile, region)
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Clear removes all cached files.
func (c *FileCache) Clear() error {
	return os.RemoveAll(c.baseDir)
}

// Dir returns the cache directory path.
func (c *FileCache) Dir() string {
	return c.baseDir
}
