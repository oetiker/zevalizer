// energy.go

package analyzer

import (
	"fmt"
	"time"
	"zevalizer/internal/api"
	"zevalizer/internal/config"
	"zevalizer/internal/models"
)

// EnergyStats represents energy data for a time period
type EnergyStats struct {
	Period struct {
		Start time.Time
		End   time.Time
	}
	GridImport       float64
	GridExport       float64
	Production       float64
	BatteryCharge    float64
	BatteryDischarge float64
	Consumers        []ConsumerStats
}

// ConsumerStats represents energy usage for a single consumer
type ConsumerStats struct {
	Sensor  *models.Sensor
	Sources struct {
		FromSolar   float64
		FromBattery float64
		FromGrid    float64
	}
	Total float64
}

// SelfConsumptionRate calculates the percentage of produced energy that was consumed locally
func (stats *EnergyStats) SelfConsumptionRate() float64 {
	if stats.Production <= 0 {
		return 0
	}
	directConsumption := stats.Production - stats.GridExport
	return (directConsumption / stats.Production) * 100
}

// AutarchyRate calculates the percentage of consumption covered by local production
func (stats *EnergyStats) AutarchyRate() float64 {
	totalConsumption := stats.GridImport + stats.Production - stats.GridExport
	if totalConsumption <= 0 {
		return 0
	}
	return ((totalConsumption - stats.GridImport) / totalConsumption) * 100
}

// IntervalData holds all energy data for a single 900-second interval
type IntervalData struct {
	Start            time.Time
	End              time.Time
	GridImport       float64
	GridExport       float64
	SolarProduction  float64
	BatteryCharge    float64
	BatteryDischarge float64
	ConsumerUsage    map[string]float64 // key: consumer ID
}

type EnergyAnalyzer struct {
	client    *api.Client
	config    *config.ZEVConfig
	debug     bool
	sensorMap map[string]*models.Sensor
	intervals []*IntervalData
}

func (ea *EnergyAnalyzer) debugf(format string, args ...interface{}) {
	if ea.debug {
		fmt.Printf("DEBUG: "+format+"\n", args...)
	}
}

func NewEnergyAnalyzer(client *api.Client, config *config.ZEVConfig, debug bool) *EnergyAnalyzer {
	return &EnergyAnalyzer{
		client:    client,
		config:    config,
		debug:     debug,
		sensorMap: make(map[string]*models.Sensor),
	}
}

// loadSensors initializes the sensor map
func (ea *EnergyAnalyzer) loadSensors(smId string) error {
	sensors, err := ea.client.GetSensors(smId)
	if err != nil {
		return fmt.Errorf("getting sensors: %w", err)
	}

	// Build sensor map and debug inverter info
	for i := range sensors {
		ea.sensorMap[sensors[i].ID] = &sensors[i]
		if sensors[i].DeviceType == "inverter" {
			ea.debugf("Found inverter: %s (ID: %s)", sensors[i].DeviceGroup, sensors[i].ID)
		}
	}

	// Log configured production IDs
	ea.debugf("\nConfigured Production IDs:")
	for _, id := range ea.config.ProductionIDs {
		if sensor, ok := ea.sensorMap[id]; ok {
			ea.debugf("  %s: %s", id, sensor.DeviceGroup)
		} else {
			ea.debugf("  %s: NOT FOUND", id)
		}
	}

	return nil
}

func (ea *EnergyAnalyzer) Analyze(smId string, from, to time.Time) (*EnergyStats, error) {
	// Initialize data structures
	if err := ea.loadSensors(smId); err != nil {
		return nil, fmt.Errorf("loading sensors: %w", err)
	}

	// Create intervals array covering the entire period
	ea.createIntervals(from, to)
	ea.debugf("Created %d intervals for analysis", len(ea.intervals))

	// Collect data for each source
	if err := ea.collectGridData(smId, from, to); err != nil {
		return nil, fmt.Errorf("collecting grid data: %w", err)
	}

	if err := ea.collectSolarData(smId, from, to); err != nil {
		return nil, fmt.Errorf("collecting solar data: %w", err)
	}

	if err := ea.collectBatteryData(smId, from, to); err != nil {
		return nil, fmt.Errorf("collecting battery data: %w", err)
	}

	if err := ea.collectConsumerData(smId, from, to); err != nil {
		return nil, fmt.Errorf("collecting consumer data: %w", err)
	}

	// Process intervals and create final statistics
	return ea.calculateStats(), nil
}

func (ea *EnergyAnalyzer) createIntervals(from, to time.Time) {
	interval := time.Duration(900) * time.Second
	current := from

	for current.Before(to) {
		intervalEnd := current.Add(interval)
		if intervalEnd.After(to) {
			intervalEnd = to
		}

		ea.intervals = append(ea.intervals, &IntervalData{
			Start:         current,
			End:           intervalEnd,
			ConsumerUsage: make(map[string]float64),
		})

		current = intervalEnd
	}
}

// findInterval returns the interval containing the given time
func (ea *EnergyAnalyzer) findInterval(t time.Time) *IntervalData {
	for _, interval := range ea.intervals {
		if (t.Equal(interval.Start) || t.After(interval.Start)) && t.Before(interval.End) {
			return interval
		}
	}
	return nil
}

func (ea *EnergyAnalyzer) collectGridData(smId string, from, to time.Time) error {
	if ea.config.GridMeterID == "" {
		return nil
	}

	data, err := ea.client.GetZevData(smId, from, to)
	if err != nil {
		return err
	}

	for _, sensorData := range data {
		if sensorData.SensorID != ea.config.GridMeterID {
			continue
		}

		// Process each data point
		for i := 1; i < len(sensorData.Data); i++ {
			current := sensorData.Data[i]
			previous := sensorData.Data[i-1]

			interval := ea.findInterval(current.CreatedAt)
			if interval == nil {
				continue
			}

			purchaseDiff := current.CurrentEnergyPurchaseTariff1 - previous.CurrentEnergyPurchaseTariff1
			deliveryDiff := current.CurrentEnergyDeliveryTariff1 - previous.CurrentEnergyDeliveryTariff1

			if purchaseDiff > 30000 || deliveryDiff > 30000 {
				ea.debugf("Skipping abnormal grid reading: purchase=%.1f delivery=%.1f",
					purchaseDiff, deliveryDiff)
				continue
			}

			interval.GridImport += purchaseDiff
			interval.GridExport += deliveryDiff
		}
	}
	return nil
}

func (ea *EnergyAnalyzer) collectSolarData(smId string, from, to time.Time) error {
	for _, prodId := range ea.config.ProductionIDs {
		data, err := ea.client.GetZevData(smId, from, to)
		if err != nil {
			return err
		}

		for _, sensorData := range data {
			if sensorData.SensorID != prodId {
				continue
			}

			for i := 1; i < len(sensorData.Data); i++ {
				current := sensorData.Data[i]
				previous := sensorData.Data[i-1]

				interval := ea.findInterval(current.CreatedAt)
				if interval == nil {
					continue
				}

				production := current.CurrentEnergyDeliveryTariff1 - previous.CurrentEnergyDeliveryTariff1
				if production > 10000 || production < 0 {
					ea.debugf("Skipping abnormal production reading: %.1f", production)
					continue
				}

				interval.SolarProduction += production
			}
		}
	}
	return nil
}

func (ea *EnergyAnalyzer) collectBatteryData(smId string, from, to time.Time) error {
	for _, batteryId := range ea.config.BatterySystemIDs {
		data, err := ea.client.GetSensorData(smId, batteryId, from, to)
		if err != nil {
			return err
		}

		sensor := ea.sensorMap[batteryId]
		for i := 1; i < len(data); i++ {
			current := data[i]

			interval := ea.findInterval(current.Date)
			if interval == nil {
				continue
			}

			charge := current.BatteryChargeWh
			discharge := current.BatteryDischargeWh

			if sensor.Data.InvertMeasurement {
				charge, discharge = discharge, charge
			}

			interval.BatteryCharge += charge
			interval.BatteryDischarge += discharge
		}
	}
	return nil
}

func (ea *EnergyAnalyzer) collectConsumerData(smId string, from, to time.Time) error {
	for _, consumerId := range ea.config.ConsumerIDs {
		data, err := ea.client.GetZevData(smId, from, to)
		if err != nil {
			return err
		}

		sensor := ea.sensorMap[consumerId]
		for _, sensorData := range data {
			if sensorData.SensorID != consumerId {
				continue
			}

			for i := 1; i < len(sensorData.Data); i++ {
				current := sensorData.Data[i]
				previous := sensorData.Data[i-1]

				interval := ea.findInterval(current.CreatedAt)
				if interval == nil {
					continue
				}

				usage := current.CurrentEnergyPurchaseTariff1 - previous.CurrentEnergyPurchaseTariff1
				if sensor.Data.InvertMeasurement {
					usage = current.CurrentEnergyDeliveryTariff1 - previous.CurrentEnergyDeliveryTariff1
				}

				if usage > 1000 {
					ea.debugf("Skipping abnormal consumer usage: %.1f", usage)
					continue
				}

				interval.ConsumerUsage[consumerId] += usage
			}
		}
	}
	return nil
}

func (ea *EnergyAnalyzer) calculateStats() *EnergyStats {
	stats := &EnergyStats{}
	if len(ea.intervals) > 0 {
		stats.Period.Start = ea.intervals[0].Start
		stats.Period.End = ea.intervals[len(ea.intervals)-1].End
	}

	// Initialize consumer stats
	consumerStats := make(map[string]*ConsumerStats)
	for _, consumerId := range ea.config.ConsumerIDs {
		consumerStats[consumerId] = &ConsumerStats{
			Sensor: ea.sensorMap[consumerId],
		}
	}

	// Add special "shared" consumer
	consumerStats["shared"] = &ConsumerStats{
		Sensor: &models.Sensor{
			Tag: models.SensorTag{
				Name: "Shared Usage",
			},
		},
	}

	// Process each interval
	for _, interval := range ea.intervals {
		ea.debugf("\nProcessing interval: %s to %s",
			interval.Start.Format("15:04:05"),
			interval.End.Format("15:04:05"))

		// Accumulate totals
		stats.GridImport += interval.GridImport
		stats.GridExport += interval.GridExport
		stats.Production += interval.SolarProduction
		stats.BatteryCharge += interval.BatteryCharge
		stats.BatteryDischarge += interval.BatteryDischarge

		// Calculate total energy input and consumption for this interval
		totalInput := interval.GridImport + interval.SolarProduction + interval.BatteryDischarge

		// Sum up all consumer usage for this interval
		var totalConsumption float64
		for _, usage := range interval.ConsumerUsage {
			totalConsumption += usage
		}

		// Calculate energy outputs
		totalOutput := totalConsumption + interval.GridExport + interval.BatteryCharge

		// Calculate excess (shared) energy
		excess := totalInput - totalOutput
		if excess > 0 {
			ea.debugf("Shared energy in interval: %.1f Wh (Input: %.1f, Output: %.1f)",
				excess, totalInput, totalOutput)
			// Add shared usage as a special consumer
			interval.ConsumerUsage["shared"] = excess
		} else if excess < -1 { // use -1 to account for small floating point differences
			ea.debugf("Warning: Negative energy balance in interval: %.1f Wh (Input: %.1f, Output: %.1f)",
				excess, totalInput, totalOutput)
		}

		// Use totalInput as available energy for distribution
		if totalInput <= 0 {
			continue
		}

		// Calculate source percentages for this interval
		solarShare := interval.SolarProduction / totalInput
		batteryShare := interval.BatteryDischarge / totalInput
		gridShare := interval.GridImport / totalInput

		ea.debugf("Interval energy shares: Solar=%.1f%% Battery=%.1f%% Grid=%.1f%%",
			solarShare*100, batteryShare*100, gridShare*100)

		// Distribute each consumer's usage according to source percentages
		for consumerId, usage := range interval.ConsumerUsage {
			if usage <= 0 {
				continue
			}

			consumer := consumerStats[consumerId]
			consumer.Total += usage
			consumer.Sources.FromSolar += usage * solarShare
			consumer.Sources.FromBattery += usage * batteryShare
			consumer.Sources.FromGrid += usage * gridShare

			ea.debugf("Consumer %s interval usage: %.1f (Solar: %.1f, Battery: %.1f, Grid: %.1f)",
				consumer.Sensor.Tag.Name, usage,
				usage*solarShare,
				usage*batteryShare,
				usage*gridShare)
		}
	}

	// Convert consumer stats map to slice
	for _, consumerStat := range consumerStats {
		stats.Consumers = append(stats.Consumers, *consumerStat)
	}

	return stats
}
