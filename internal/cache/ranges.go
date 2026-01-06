package cache

import (
	"sort"
	"time"
)

// NormalizeDate returns the date at 00:00:00 in local timezone
func NormalizeDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// Today returns today's date normalized to 00:00:00
func Today() time.Time {
	return NormalizeDate(time.Now())
}

// DateToKey converts a time to a cache key string (YYYY-MM-DD)
func DateToKey(t time.Time) string {
	return t.Format("2006-01-02")
}

// KeyToDate parses a cache key string back to time
func KeyToDate(key string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", key, time.Local)
}

// Contains checks if a date is within a DateRange
func (r DateRange) Contains(date time.Time) bool {
	d := NormalizeDate(date)
	return !d.Before(r.Start) && !d.After(r.End)
}

// Overlaps checks if two ranges overlap
func (r DateRange) Overlaps(other DateRange) bool {
	return !r.End.Before(other.Start) && !other.End.Before(r.Start)
}

// MergeRanges consolidates overlapping/adjacent ranges into minimal set
func MergeRanges(ranges []DateRange) []DateRange {
	if len(ranges) == 0 {
		return ranges
	}

	// Sort by start date
	sorted := make([]DateRange, len(ranges))
	copy(sorted, ranges)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Start.Before(sorted[j].Start)
	})

	result := []DateRange{sorted[0]}
	for i := 1; i < len(sorted); i++ {
		last := &result[len(result)-1]
		current := sorted[i]

		// Check if adjacent (end + 1 day == start) or overlapping
		nextDay := last.End.AddDate(0, 0, 1)
		if !current.Start.After(nextDay) {
			// Merge: extend last range if current extends further
			if current.End.After(last.End) {
				last.End = current.End
			}
		} else {
			// Gap between ranges, add as new range
			result = append(result, current)
		}
	}

	return result
}

// FindGaps returns date ranges NOT covered by the cached ranges within [from, to]
// Excludes today (always needs fresh fetch)
func FindGaps(cached []DateRange, from, to time.Time) []DateRange {
	from = NormalizeDate(from)
	to = NormalizeDate(to)
	today := Today()

	// Exclude today from the range we're checking
	if !to.Before(today) {
		to = today.AddDate(0, 0, -1)
	}
	if from.After(to) {
		return nil // Entire range is today or future
	}

	// Start with the full range as a gap
	gaps := []DateRange{{Start: from, End: to}}

	// Subtract each cached range
	for _, c := range cached {
		var newGaps []DateRange
		for _, gap := range gaps {
			subtracted := subtractRange(gap, c)
			newGaps = append(newGaps, subtracted...)
		}
		gaps = newGaps
	}

	return gaps
}

// subtractRange removes the 'subtract' range from 'base', returning remaining pieces
func subtractRange(base, subtract DateRange) []DateRange {
	// No overlap
	if !base.Overlaps(subtract) {
		return []DateRange{base}
	}

	var result []DateRange

	// Left piece (before subtract starts)
	if base.Start.Before(subtract.Start) {
		result = append(result, DateRange{
			Start: base.Start,
			End:   subtract.Start.AddDate(0, 0, -1),
		})
	}

	// Right piece (after subtract ends)
	if base.End.After(subtract.End) {
		result = append(result, DateRange{
			Start: subtract.End.AddDate(0, 0, 1),
			End:   base.End,
		})
	}

	return result
}
