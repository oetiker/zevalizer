package cache

import (
	"fmt"
	"io"
	"sort"
)

// Dump writes a human-readable representation of the cache
func (c *Cache) Dump(w io.Writer) {
	fmt.Fprintf(w, "=== Cache Dump ===\n\n")

	// Metadata
	fmt.Fprintf(w, "Metadata:\n")
	fmt.Fprintf(w, "  Version:      %d\n", c.Metadata.Version)
	fmt.Fprintf(w, "  SmID:         %s\n", c.Metadata.SmID)
	fmt.Fprintf(w, "  Created:      %s\n", c.Metadata.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "  Last Updated: %s\n\n", c.Metadata.LastUpdated.Format("2006-01-02 15:04:05"))

	// ZEV Data Summary
	fmt.Fprintf(w, "ZEV Data:\n")
	fmt.Fprintf(w, "  Cached Ranges:\n")
	if len(c.ZevData.CachedRanges) == 0 {
		fmt.Fprintf(w, "    (none)\n")
	}
	for _, r := range c.ZevData.CachedRanges {
		days := int(r.End.Sub(r.Start).Hours()/24) + 1
		fmt.Fprintf(w, "    %s to %s (%d days)\n",
			r.Start.Format("2006-01-02"),
			r.End.Format("2006-01-02"),
			days)
	}

	// Count data points per sensor
	sensorCounts := make(map[string]int)
	for _, dateData := range c.ZevData.Data {
		for sensorID, points := range dateData {
			sensorCounts[sensorID] += len(points)
		}
	}

	fmt.Fprintf(w, "  Data Points per Sensor:\n")
	if len(sensorCounts) == 0 {
		fmt.Fprintf(w, "    (none)\n")
	} else {
		var sensorIDs []string
		for id := range sensorCounts {
			sensorIDs = append(sensorIDs, id)
		}
		sort.Strings(sensorIDs)
		for _, id := range sensorIDs {
			fmt.Fprintf(w, "    %s: %d points\n", id, sensorCounts[id])
		}
	}

	// Sensor Data (Batteries) Summary
	fmt.Fprintf(w, "\nSensor Data (Batteries):\n")
	if len(c.SensorData.CachedRanges) == 0 {
		fmt.Fprintf(w, "  (none)\n")
	}

	var batteryIDs []string
	for sensorID := range c.SensorData.CachedRanges {
		batteryIDs = append(batteryIDs, sensorID)
	}
	sort.Strings(batteryIDs)

	for _, sensorID := range batteryIDs {
		ranges := c.SensorData.CachedRanges[sensorID]
		fmt.Fprintf(w, "  Sensor %s:\n", sensorID)
		fmt.Fprintf(w, "    Cached Ranges:\n")
		for _, r := range ranges {
			days := int(r.End.Sub(r.Start).Hours()/24) + 1
			fmt.Fprintf(w, "      %s to %s (%d days)\n",
				r.Start.Format("2006-01-02"),
				r.End.Format("2006-01-02"),
				days)
		}
		if data, ok := c.SensorData.Data[sensorID]; ok {
			total := 0
			for _, points := range data {
				total += len(points)
			}
			fmt.Fprintf(w, "    Total Data Points: %d\n", total)
		}
	}

	fmt.Fprintf(w, "\n=== End Cache Dump ===\n")
}
