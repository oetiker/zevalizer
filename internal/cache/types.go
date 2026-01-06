package cache

import (
	"time"

	"zevalizer/internal/models"
)

// CacheMetadata stores information about the cache itself
type CacheMetadata struct {
	Version     int
	CreatedAt   time.Time
	LastUpdated time.Time
	SmID        string
}

// DateRange represents a contiguous range of cached dates
type DateRange struct {
	Start time.Time // Inclusive, normalized to start of day (00:00:00)
	End   time.Time // Inclusive, normalized to start of day (00:00:00)
}

// ZevDataCache stores ZEV data indexed by date and sensor
type ZevDataCache struct {
	// Data maps date (YYYY-MM-DD) -> sensorID -> data points
	Data map[string]map[string][]models.ZevSensorData

	// CachedRanges tracks which date ranges have been fetched
	CachedRanges []DateRange
}

// SensorDataCache stores battery sensor data
type SensorDataCache struct {
	// Data maps sensorID -> date (YYYY-MM-DD) -> data points
	Data map[string]map[string][]models.SensorData

	// CachedRanges tracks per-sensor cached date ranges
	CachedRanges map[string][]DateRange
}

// Cache is the top-level cache structure persisted to disk
type Cache struct {
	Metadata   CacheMetadata
	ZevData    ZevDataCache
	SensorData SensorDataCache
}
