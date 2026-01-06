package cache

import (
	"time"

	"zevalizer/internal/models"
)

// StoreZevData adds ZEV data to the cache, excluding today's data
func (c *Cache) StoreZevData(data []models.ZevData) {
	today := Today()

	for _, zevData := range data {
		sensorID := zevData.SensorID

		for _, point := range zevData.Data {
			pointDate := NormalizeDate(point.CreatedAt)

			// Skip today's data - never cache it
			if !pointDate.Before(today) {
				continue
			}

			dateKey := DateToKey(pointDate)

			// Initialize nested maps if needed
			if c.ZevData.Data[dateKey] == nil {
				c.ZevData.Data[dateKey] = make(map[string][]models.ZevSensorData)
			}

			// Append data point
			c.ZevData.Data[dateKey][sensorID] = append(
				c.ZevData.Data[dateKey][sensorID],
				point,
			)
		}
	}
}

// UpdateZevCachedRanges updates the cached ranges after storing new data
func (c *Cache) UpdateZevCachedRanges(from, to time.Time) {
	from = NormalizeDate(from)
	to = NormalizeDate(to)
	today := Today()

	// Exclude today
	if !to.Before(today) {
		to = today.AddDate(0, 0, -1)
	}
	if from.After(to) {
		return // Nothing to mark as cached
	}

	newRange := DateRange{Start: from, End: to}
	c.ZevData.CachedRanges = append(c.ZevData.CachedRanges, newRange)
	c.ZevData.CachedRanges = MergeRanges(c.ZevData.CachedRanges)
}

// GetZevData retrieves cached ZEV data for a date range
// Returns data in the same format as the API: []models.ZevData
func (c *Cache) GetZevData(from, to time.Time) []models.ZevData {
	from = NormalizeDate(from)
	to = NormalizeDate(to)

	// Collect all data points organized by sensor
	sensorData := make(map[string][]models.ZevSensorData)

	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		dateKey := DateToKey(d)
		if dayData, ok := c.ZevData.Data[dateKey]; ok {
			for sensorID, points := range dayData {
				sensorData[sensorID] = append(sensorData[sensorID], points...)
			}
		}
	}

	// Convert to API format
	var result []models.ZevData
	for sensorID, points := range sensorData {
		result = append(result, models.ZevData{
			SensorID: sensorID,
			Data:     points,
		})
	}

	return result
}

// GetZevCacheGaps returns date ranges that need fetching for ZEV data
func (c *Cache) GetZevCacheGaps(from, to time.Time) []DateRange {
	return FindGaps(c.ZevData.CachedRanges, from, to)
}
