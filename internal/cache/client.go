package cache

import (
	"fmt"
	"io"
	"sort"
	"time"

	"zevalizer/internal/api"
	"zevalizer/internal/models"
)

// CachedClient wraps api.Client with caching capabilities
type CachedClient struct {
	client    *api.Client
	cache     *Cache
	cachePath string
	enabled   bool
	debug     bool
}

// NewCachedClient creates a caching wrapper around the API client
func NewCachedClient(client *api.Client, cachePath string, smID string, enabled bool, debug bool) (*CachedClient, error) {
	var c *Cache
	var err error

	if enabled {
		c, err = Load(cachePath, smID)
		if err != nil {
			return nil, fmt.Errorf("loading cache: %w", err)
		}
	} else {
		c = NewCache(smID)
	}

	return &CachedClient{
		client:    client,
		cache:     c,
		cachePath: cachePath,
		enabled:   enabled,
		debug:     debug,
	}, nil
}

func (cc *CachedClient) debugf(format string, args ...interface{}) {
	if cc.debug {
		fmt.Printf("DEBUG [cache]: "+format+"\n", args...)
	}
}

// GetSensors fetches sensor list (not cached, rarely changes)
func (cc *CachedClient) GetSensors(smID string) ([]models.Sensor, error) {
	return cc.client.GetSensors(smID)
}

// GetZevData fetches ZEV data, using cache where possible
func (cc *CachedClient) GetZevData(smId string, from, to time.Time) ([]models.ZevData, error) {
	if !cc.enabled {
		return cc.client.GetZevData(smId, from, to)
	}

	today := Today()
	var allData []models.ZevData
	cacheModified := false

	// 1. Get gaps that need fetching (excludes today automatically)
	gaps := cc.cache.GetZevCacheGaps(from, to)

	// 2. Check if request includes today
	includestoday := !NormalizeDate(to).Before(today)

	// 3. Fetch missing historical data
	for _, gap := range gaps {
		cc.debugf("Fetching ZEV data gap: %s to %s",
			gap.Start.Format("2006-01-02"),
			gap.End.Format("2006-01-02"))

		// Fetch ends at 23:59:59 of the last day
		gapEnd := time.Date(gap.End.Year(), gap.End.Month(), gap.End.Day(),
			23, 59, 59, 999999999, gap.End.Location())

		data, err := cc.client.GetZevData(smId, gap.Start, gapEnd)
		if err != nil {
			return nil, err
		}

		// Store in cache
		cc.cache.StoreZevData(data)
		cc.cache.UpdateZevCachedRanges(gap.Start, gap.End)
		cacheModified = true
	}

	// 4. Fetch today's data fresh (never cached)
	if includestoday {
		cc.debugf("Fetching today's ZEV data (not cached)")
		todayEnd := time.Date(today.Year(), today.Month(), today.Day(),
			23, 59, 59, 999999999, today.Location())
		todayData, err := cc.client.GetZevData(smId, today, todayEnd)
		if err != nil {
			return nil, err
		}
		allData = append(allData, todayData...)
	}

	// 5. Get cached historical data
	historicalEnd := NormalizeDate(to)
	if includestoday {
		historicalEnd = today.AddDate(0, 0, -1)
	}
	if !historicalEnd.Before(NormalizeDate(from)) {
		cachedData := cc.cache.GetZevData(from, historicalEnd)
		cc.debugf("Retrieved %d sensors from cache for %s to %s",
			len(cachedData),
			from.Format("2006-01-02"),
			historicalEnd.Format("2006-01-02"))
		allData = append(allData, cachedData...)
	}

	// 6. Save updated cache
	if cacheModified {
		if err := cc.cache.Save(cc.cachePath); err != nil {
			cc.debugf("Warning: failed to save cache: %v", err)
		}
	}

	// 7. Merge data by sensor (combine cached + fresh)
	return mergeZevData(allData), nil
}

// GetSensorData fetches sensor data with caching (for batteries)
func (cc *CachedClient) GetSensorData(smId string, sensorID string, from, to time.Time) ([]models.SensorData, error) {
	if !cc.enabled {
		return cc.client.GetSensorData(smId, sensorID, from, to)
	}

	today := Today()
	var allData []models.SensorData
	cacheModified := false

	// Get gaps for this specific sensor
	gaps := cc.cache.GetSensorCacheGaps(sensorID, from, to)
	includestoday := !NormalizeDate(to).Before(today)

	// Fetch missing historical data
	for _, gap := range gaps {
		cc.debugf("Fetching sensor %s data gap: %s to %s",
			sensorID,
			gap.Start.Format("2006-01-02"),
			gap.End.Format("2006-01-02"))

		gapEnd := time.Date(gap.End.Year(), gap.End.Month(), gap.End.Day(),
			23, 59, 59, 999999999, gap.End.Location())

		data, err := cc.client.GetSensorData(smId, sensorID, gap.Start, gapEnd)
		if err != nil {
			return nil, err
		}

		cc.cache.StoreSensorData(sensorID, data)
		cc.cache.UpdateSensorCachedRanges(sensorID, gap.Start, gap.End)
		cacheModified = true
	}

	// Fetch today fresh
	if includestoday {
		cc.debugf("Fetching today's sensor %s data (not cached)", sensorID)
		todayEnd := time.Date(today.Year(), today.Month(), today.Day(),
			23, 59, 59, 999999999, today.Location())
		todayData, err := cc.client.GetSensorData(smId, sensorID, today, todayEnd)
		if err != nil {
			return nil, err
		}
		allData = append(allData, todayData...)
	}

	// Get cached historical data
	historicalEnd := NormalizeDate(to)
	if includestoday {
		historicalEnd = today.AddDate(0, 0, -1)
	}
	if !historicalEnd.Before(NormalizeDate(from)) {
		cachedData := cc.cache.GetSensorData(sensorID, from, historicalEnd)
		allData = append(allData, cachedData...)
	}

	// Save updated cache
	if cacheModified {
		if err := cc.cache.Save(cc.cachePath); err != nil {
			cc.debugf("Warning: failed to save cache: %v", err)
		}
	}

	return mergeSensorData(allData), nil
}

// ClearCache removes all cached data
func (cc *CachedClient) ClearCache() error {
	cc.cache.Clear()
	return cc.cache.Save(cc.cachePath)
}

// DeleteCache removes the cache file from disk
func (cc *CachedClient) DeleteCache() error {
	return Delete(cc.cachePath)
}

// DumpCache writes cache contents to the given writer
func (cc *CachedClient) DumpCache(w io.Writer) {
	cc.cache.Dump(w)
}

// mergeZevData combines data from multiple ZevData slices by sensor
func mergeZevData(data []models.ZevData) []models.ZevData {
	merged := make(map[string]*models.ZevData)

	for _, zd := range data {
		if existing, ok := merged[zd.SensorID]; ok {
			existing.Data = append(existing.Data, zd.Data...)
		} else {
			copy := zd
			merged[zd.SensorID] = &copy
		}
	}

	// Sort data points by time within each sensor
	var result []models.ZevData
	for _, zd := range merged {
		sort.Slice(zd.Data, func(i, j int) bool {
			return zd.Data[i].CreatedAt.Before(zd.Data[j].CreatedAt)
		})
		result = append(result, *zd)
	}

	return result
}

// mergeSensorData combines and deduplicates sensor data
func mergeSensorData(data []models.SensorData) []models.SensorData {
	seen := make(map[time.Time]bool)
	var result []models.SensorData

	for _, d := range data {
		if !seen[d.Date] {
			seen[d.Date] = true
			result = append(result, d)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Date.Before(result[j].Date)
	})

	return result
}
