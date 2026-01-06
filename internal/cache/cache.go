package cache

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zevalizer/internal/models"
)

// CacheFilePath derives cache path from config path
// config.yaml -> config.data-cache
func CacheFilePath(configPath string) string {
	ext := filepath.Ext(configPath)
	base := strings.TrimSuffix(configPath, ext)
	return base + ".data-cache"
}

// NewCache creates an empty cache for a given SmID
func NewCache(smID string) *Cache {
	return &Cache{
		Metadata: CacheMetadata{
			Version:     1,
			CreatedAt:   time.Now(),
			LastUpdated: time.Now(),
			SmID:        smID,
		},
		ZevData: ZevDataCache{
			Data:         make(map[string]map[string][]models.ZevSensorData),
			CachedRanges: []DateRange{},
		},
		SensorData: SensorDataCache{
			Data:         make(map[string]map[string][]models.SensorData),
			CachedRanges: make(map[string][]DateRange),
		},
	}
}

// Load reads cache from disk, returns empty cache if file doesn't exist
func Load(path string, smID string) (*Cache, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return NewCache(smID), nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening cache file: %w", err)
	}
	defer file.Close()

	var cache Cache
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&cache); err != nil {
		return nil, fmt.Errorf("decoding cache: %w", err)
	}

	// Validate SmID matches (skip if smID is empty, e.g., for dump-cache)
	if smID != "" && cache.Metadata.SmID != smID {
		return nil, fmt.Errorf("cache SmID mismatch: got %s, expected %s",
			cache.Metadata.SmID, smID)
	}

	// Initialize maps if nil (for older cache versions)
	if cache.ZevData.Data == nil {
		cache.ZevData.Data = make(map[string]map[string][]models.ZevSensorData)
	}
	if cache.SensorData.Data == nil {
		cache.SensorData.Data = make(map[string]map[string][]models.SensorData)
	}
	if cache.SensorData.CachedRanges == nil {
		cache.SensorData.CachedRanges = make(map[string][]DateRange)
	}

	return &cache, nil
}

// Save writes cache to disk atomically (write to temp, then rename)
func (c *Cache) Save(path string) error {
	c.Metadata.LastUpdated = time.Now()

	// Write to temporary file first
	tmpPath := path + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("creating temp cache file: %w", err)
	}

	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(c); err != nil {
		file.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("encoding cache: %w", err)
	}

	if err := file.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp cache file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming cache file: %w", err)
	}

	return nil
}

// Clear removes all cached data but preserves metadata
func (c *Cache) Clear() {
	c.ZevData.Data = make(map[string]map[string][]models.ZevSensorData)
	c.ZevData.CachedRanges = []DateRange{}
	c.SensorData.Data = make(map[string]map[string][]models.SensorData)
	c.SensorData.CachedRanges = make(map[string][]DateRange)
}

// Delete removes the cache file from disk
func Delete(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil // Already deleted
	}
	return err
}
