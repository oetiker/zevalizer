// energy.go

package analyzer

import (
	"fmt"
	"time"

	"zevalizer/internal/config"
	"zevalizer/internal/models"
)

const (
	// IntervalSeconds is the duration of each analysis interval (15 minutes)
	IntervalSeconds = 900

	// MaxGridReadingDiffWh is the maximum reasonable grid import/export per interval (30 kWh).
	// Readings above this are considered anomalies and skipped.
	MaxGridReadingDiffWh = 30000

	// MaxProductionReadingWh is the maximum reasonable inverter production per interval (10 kWh).
	// Readings above this are considered anomalies and skipped.
	MaxProductionReadingWh = 10000

	// MaxConsumerReadingWh is the maximum reasonable consumer usage per interval (10 kWh).
	// Readings above this are considered anomalies and skipped.
	MaxConsumerReadingWh = 10000
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
	Consumption      float64
	BatteryCharge    float64
	BatteryDischarge float64
	Consumers        []ConsumerStats
}

// ConsumerStats represents energy usage for a single consumer
type ConsumerStats struct {
	Sensor  *models.Sensor
	Sources struct {
		FromInverter float64
		FromBattery  float64
		FromGrid     float64
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
	Start                    time.Time
	End                      time.Time
	GridImport               float64
	GridExport               float64
	InverterGeneratedPower   float64
	InverterPowerConsumption float64
	BatteryCharge            float64
	BatteryDischarge         float64
	ConsumerUsage            map[string]float64 // key: consumer ID
}

// DataFetcher is an interface for fetching data from the API
// Both api.Client and cache.CachedClient implement this interface
type DataFetcher interface {
	GetZevData(smId string, from, to time.Time) ([]models.ZevData, error)
	GetSensorData(smId string, sensorID string, from, to time.Time) ([]models.SensorData, error)
	GetSensors(smID string) ([]models.Sensor, error)
}

type EnergyAnalyzer struct {
	client    DataFetcher
	config    *config.Config
	sensorMap map[string]*models.Sensor
	intervals []*IntervalData
}

func (ea *EnergyAnalyzer) debugf(format string, args ...interface{}) {
	if ea.config.Debug {
		fmt.Printf("DEBUG: "+format+"\n", args...)
	}
}

func NewEnergyAnalyzer(client DataFetcher, config *config.Config) *EnergyAnalyzer {
	return &EnergyAnalyzer{
		client:    client,
		config:    config,
		sensorMap: make(map[string]*models.Sensor),
	}
}

// isLowTariffHour checks if a given hour falls within the low tariff period.
// Handles both overnight periods (e.g., 22:00-06:00) and daytime periods (e.g., 06:00-22:00).
func (ea *EnergyAnalyzer) isLowTariffHour(hour int) bool {
	start := ea.config.LowTariff.StartHour
	end := ea.config.LowTariff.EndHour

	if start > end {
		// Overnight period (e.g., 22:00 - 06:00)
		return hour >= start || hour < end
	}
	// Daytime period (e.g., 06:00 - 22:00)
	return hour >= start && hour < end
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
	for _, id := range ea.config.ZEV.ProductionIDs {
		if sensor, ok := ea.sensorMap[id]; ok {
			ea.debugf("  %s: %s", id, sensor.DeviceGroup)
		} else {
			ea.debugf("  %s: NOT FOUND", id)
		}
	}

	return nil
}

func (ea *EnergyAnalyzer) Analyze(smId string, from, to time.Time) (*EnergyStats, *EnergyStats, error) {
	// Validate inverter efficiency config
	eff := ea.config.ZEV.InverterEfficiency
	if eff != 0 && (eff < 0 || eff > 1) {
		return nil, nil, fmt.Errorf("invalid inverterEfficiency %.2f: must be between 0 and 1", eff)
	}

	// Initialize data structures
	if err := ea.loadSensors(smId); err != nil {
		return nil, nil, fmt.Errorf("loading sensors: %w", err)
	}

	// Create intervals array covering the entire period
	ea.createIntervals(from, to)
	ea.debugf("Created %d intervals for analysis", len(ea.intervals))

	// Collect data for each source
	data, err := ea.client.GetZevData(smId, from, to)
	if err != nil {
		return nil, nil, err
	}

	if err := ea.collectGridData(data); err != nil {
		return nil, nil, fmt.Errorf("collecting grid data: %w", err)
	}

	if err := ea.collectInverterData(data); err != nil {
		return nil, nil, fmt.Errorf("collecting inverter data: %w", err)
	}

	if err := ea.collectConsumerData(data); err != nil {
		return nil, nil, fmt.Errorf("collecting consumer data: %w", err)
	}

	if err := ea.collectBatteryData(smId, from, to); err != nil {
		return nil, nil, fmt.Errorf("collecting battery data: %w", err)
	}

	// Process intervals and create final statistics
	statLowTariff, err := ea.calculateStats(true)
	if err != nil {
		return nil, nil, fmt.Errorf("calculating low tariff stats: %w", err)
	}
	statHighTariff, err := ea.calculateStats(false)
	if err != nil {
		return nil, nil, fmt.Errorf("calculating high tariff stats: %w", err)
	}
	return statLowTariff, statHighTariff, nil
}

func (ea *EnergyAnalyzer) createIntervals(from, to time.Time) {
	interval := time.Duration(IntervalSeconds) * time.Second
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

func (ea *EnergyAnalyzer) collectGridData(data []models.ZevData) error {
	if ea.config.ZEV.GridMeterID == "" {
		return nil
	}

	for _, sensorData := range data {
		if sensorData.SensorID != ea.config.ZEV.GridMeterID {
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

			if previous.CurrentEnergyDeliveryTariff1 == 0 && current.CurrentEnergyDeliveryTariff1 != 0 {
				continue
			}

			purchaseDiff := current.CurrentEnergyPurchaseTariff1 - previous.CurrentEnergyPurchaseTariff1
			deliveryDiff := current.CurrentEnergyDeliveryTariff1 - previous.CurrentEnergyDeliveryTariff1

			if purchaseDiff > MaxGridReadingDiffWh || deliveryDiff > MaxGridReadingDiffWh {
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

func (ea *EnergyAnalyzer) collectInverterData(data []models.ZevData) error {
	for _, prodId := range ea.config.ZEV.ProductionIDs {

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

				delivery := current.CurrentEnergyDeliveryTariff1 - previous.CurrentEnergyDeliveryTariff1
				if delivery > MaxProductionReadingWh || delivery < 0 {
					ea.debugf("Skipping abnormal delivery reading: %.1f", delivery)
					continue
				}
				purchase := current.CurrentEnergyPurchaseTariff1 - previous.CurrentEnergyPurchaseTariff1
				if purchase > MaxProductionReadingWh || purchase < 0 {
					ea.debugf("Skipping abnormal purchase reading: %.1f", purchase)
					continue
				}

				// Use NET formula: production = delivery - purchase
				// This removes phantom power circulation from hybrid inverters
				// Positive = inverter contributing energy (solar/battery)
				// Negative = inverter consuming energy (standby, losses)
				interval.InverterGeneratedPower += delivery - purchase
				// Don't add purchase to InverterPowerConsumption separately -
				// it's already accounted for in the NET calculation
			}
		}
	}
	return nil
}

func (ea *EnergyAnalyzer) collectBatteryData(smId string, from, to time.Time) error {
	for _, batteryId := range ea.config.ZEV.BatterySystemIDs {
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
			ea.debugf("%s, Battery: %s, Charge: %.1f kWh, Discharge: %.1f kWh", current.Date.Format("2006-01-02 15:04:05 MST"), batteryId, charge/1000, discharge/1000)
			interval.BatteryCharge += charge
			interval.BatteryDischarge += discharge
		}
	}
	return nil
}

func (ea *EnergyAnalyzer) collectConsumerData(data []models.ZevData) error {
	for _, consumerId := range ea.config.ZEV.ConsumerIDs {

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

				if usage > MaxConsumerReadingWh {
					ea.debugf("Skipping abnormal consumer usage: %.1f", usage)
					continue
				}

				interval.ConsumerUsage[consumerId] += usage
			}
		}
	}
	return nil
}

func (ea *EnergyAnalyzer) calculateStats(lowTariff bool) (*EnergyStats, error) {
	stats := &EnergyStats{}

	if len(ea.intervals) > 0 {
		stats.Period.Start = ea.intervals[0].Start
		stats.Period.End = ea.intervals[len(ea.intervals)-1].End
	}

	// Initialize consumer stats
	consumerStats := make(map[string]*ConsumerStats)
	for _, consumerId := range ea.config.ZEV.ConsumerIDs {
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
		// Filter intervals based on tariff period
		intervalIsLowTariff := ea.isLowTariffHour(interval.Start.Hour())
		if intervalIsLowTariff != lowTariff {
			continue
		}

		ea.debugf("\nProcessing %s interval: %s to %s",
			func() string {
				if lowTariff {
					return "Low-Tariff"
				}
				return "High-Tariff"
			}(),
			interval.Start.Format("15:04:05"),
			interval.End.Format("15:04:05"))

		ea.debugf("Grid Import: %.1f kWh", interval.GridImport/1000)
		ea.debugf("Grid Export: %.1f kWh", interval.GridExport/1000)
		ea.debugf("Inverter Production: %.1f kWh", interval.InverterGeneratedPower/1000)
		ea.debugf("Inverter Consumption: %.1f kWh", interval.InverterPowerConsumption/1000)
		ea.debugf("Battery Charge: %.1f kWh", interval.BatteryCharge/1000)
		ea.debugf("Battery Discharge: %.1f kWh", interval.BatteryDischarge/1000)

		// Accumulate totals

		stats.GridImport += interval.GridImport
		stats.GridExport += interval.GridExport
		stats.Production += interval.InverterGeneratedPower
		stats.Consumption += interval.InverterPowerConsumption
		stats.BatteryCharge += interval.BatteryCharge
		stats.BatteryDischarge += interval.BatteryDischarge

		// Calculate total energy input and consumption for this interval
		totalInput := interval.GridImport + interval.InverterGeneratedPower

		// Sum up all consumer usage for this interval
		var totalEnergyConsumption float64
		for _, usage := range interval.ConsumerUsage {
			totalEnergyConsumption += usage
		}

		// Calculate energy outputs
		totalOutput := totalEnergyConsumption + interval.GridExport + interval.InverterPowerConsumption

		// Calculate sharedUseEnergy (shared) energy
		sharedUseEnergy := totalInput - totalOutput
		if sharedUseEnergy > 0 {
			ea.debugf("Shared energy in interval: %.1f Wh (Input: %.1f, Output: %.1f)",
				sharedUseEnergy, totalInput, totalOutput)
			// Add shared usage as a special consumer
			interval.ConsumerUsage["shared"] = sharedUseEnergy
		} else if sharedUseEnergy < -1 { // use -1 to account for small floating point differences
			ea.debugf("Warning: Negative energy balance in interval: %.1f Wh (Input: %.1f, Output: %.1f)",
				sharedUseEnergy, totalInput, totalOutput)
		}

		// Use totalInput as available energy for distribution
		if totalInput <= 0 {
			continue
		}

		// Calculate source percentages for this interval
		// Battery discharge is measured at DC side, but inverter output is AC
		// Apply inverter efficiency to convert battery DC to AC contribution
		inverterEfficiency := ea.config.ZEV.InverterEfficiency
		if inverterEfficiency == 0 {
			inverterEfficiency = 0.93 // Default 93% efficiency if not configured
		}
		batteryACContribution := interval.BatteryDischarge * inverterEfficiency
		solarContribution := interval.InverterGeneratedPower - batteryACContribution

		// Handle two cases of negative solarContribution:
		// 1. InverterGeneratedPower < 0: True consumption (grid charging battery)
		//    -> This is "common power", attributed to Shared Usage
		// 2. InverterGeneratedPower >= 0 but solarContribution < 0: Efficiency mismatch
		//    -> Cap battery contribution to inverter output, solar = 0
		inverterConsuming := interval.InverterGeneratedPower < 0
		if !inverterConsuming && solarContribution < 0 {
			// Efficiency mismatch: battery can't contribute more than inverter output
			batteryACContribution = interval.InverterGeneratedPower
			solarContribution = 0
		}

		inverterShare := solarContribution / totalInput
		batteryShare := batteryACContribution / totalInput
		gridShare := interval.GridImport / totalInput

		ea.debugf("Interval energy shares: Inverter=%.1f%% Battery=%.1f%% Grid=%.1f%% (consuming=%v)",
			inverterShare*100, batteryShare*100, gridShare*100, inverterConsuming)

		// Distribute each consumer's usage according to source percentages
		for consumerId, usage := range interval.ConsumerUsage {
			if usage <= 0 {
				continue
			}

			consumer := consumerStats[consumerId]
			consumer.Total += usage

			if inverterConsuming && consumerId != "shared" {
				// Regular consumers: don't show negative inverter, adjust grid share
				// The inverter consumption is common power, attributed to Shared Usage
				consumer.Sources.FromInverter += 0
				consumer.Sources.FromBattery += usage * batteryShare
				consumer.Sources.FromGrid += usage * (gridShare + inverterShare) // grid covers inverter consumption
			} else {
				// Normal case OR Shared Usage (which gets the inverter consumption)
				consumer.Sources.FromInverter += usage * inverterShare
				consumer.Sources.FromBattery += usage * batteryShare
				consumer.Sources.FromGrid += usage * gridShare
			}

			ea.debugf("Consumer %s interval usage: %.1f (Inverter: %.1f, Battery: %.1f, Grid: %.1f)",
				consumer.Sensor.Tag.Name, usage,
				usage*inverterShare,
				usage*batteryShare,
				usage*gridShare)
		}
	}

	// Convert consumer stats map to slice
	for _, consumerStat := range consumerStats {
		stats.Consumers = append(stats.Consumers, *consumerStat)
	}

	return stats, nil
}
