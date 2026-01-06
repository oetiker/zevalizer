package cache

import (
	"time"

	"zevalizer/internal/models"
)

// StoreSensorData adds battery sensor data to the cache, excluding today
func (c *Cache) StoreSensorData(sensorID string, data []models.SensorData) {
	today := Today()

	if c.SensorData.Data[sensorID] == nil {
		c.SensorData.Data[sensorID] = make(map[string][]models.SensorData)
	}

	for _, point := range data {
		pointDate := NormalizeDate(point.Date)

		// Skip today's data
		if !pointDate.Before(today) {
			continue
		}

		dateKey := DateToKey(pointDate)
		c.SensorData.Data[sensorID][dateKey] = append(
			c.SensorData.Data[sensorID][dateKey],
			point,
		)
	}
}

// UpdateSensorCachedRanges marks a date range as cached for a specific sensor
func (c *Cache) UpdateSensorCachedRanges(sensorID string, from, to time.Time) {
	from = NormalizeDate(from)
	to = NormalizeDate(to)
	today := Today()

	if !to.Before(today) {
		to = today.AddDate(0, 0, -1)
	}
	if from.After(to) {
		return
	}

	if c.SensorData.CachedRanges == nil {
		c.SensorData.CachedRanges = make(map[string][]DateRange)
	}

	newRange := DateRange{Start: from, End: to}
	c.SensorData.CachedRanges[sensorID] = append(c.SensorData.CachedRanges[sensorID], newRange)
	c.SensorData.CachedRanges[sensorID] = MergeRanges(c.SensorData.CachedRanges[sensorID])
}

// GetSensorData retrieves cached sensor data for a date range
func (c *Cache) GetSensorData(sensorID string, from, to time.Time) []models.SensorData {
	from = NormalizeDate(from)
	to = NormalizeDate(to)

	var result []models.SensorData

	sensorCache, ok := c.SensorData.Data[sensorID]
	if !ok {
		return result
	}

	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		dateKey := DateToKey(d)
		if points, ok := sensorCache[dateKey]; ok {
			result = append(result, points...)
		}
	}

	return result
}

// GetSensorCacheGaps returns date ranges needing fetch for a specific sensor
func (c *Cache) GetSensorCacheGaps(sensorID string, from, to time.Time) []DateRange {
	ranges := c.SensorData.CachedRanges[sensorID]
	return FindGaps(ranges, from, to)
}
