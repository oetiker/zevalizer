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

type EnergyAnalyzer struct {
	client    *api.Client
	config    *config.Config
	sensorMap map[string]*models.Sensor
	intervals []*IntervalData
}

func (ea *EnergyAnalyzer) debugf(format string, args ...interface{}) {
	if ea.config.Debug {
		fmt.Printf("DEBUG: "+format+"\n", args...)
	}
}

func NewEnergyAnalyzer(client *api.Client, config *config.Config) *EnergyAnalyzer {
	return &EnergyAnalyzer{
		client:    client,
		config:    config,
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
	statHighTariff, err := ea.calculateStats(false)
	return statLowTariff, statHighTariff, err
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

				production := current.CurrentEnergyDeliveryTariff1 - previous.CurrentEnergyDeliveryTariff1
				if production > 10000 || production < 0 {
					ea.debugf("Skipping abnormal production reading: %.1f", production)
					continue
				}
				consumtion := current.CurrentEnergyPurchaseTariff1 - previous.CurrentEnergyPurchaseTariff1
				if consumtion > 10000 || consumtion < 0 {
					ea.debugf("Skipping abnormal consumtion reading: %.1f", consumtion)
					continue
				}

				interval.InverterGeneratedPower += production
				interval.InverterPowerConsumption += consumtion
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

				if usage > 10000 {
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

		// if interval.Start.Hour() >= ea.config.LowTariff.StartHour || interval.End.Hour() < ea.config.LowTariff.EndHour {
		// 	if !lowTariff {
		// 		continue
		// 	}
		// } else if lowTariff {
		// 	continue
		// }
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
		ea.debugf("Inverter Consumtion: %.1f kWh", interval.InverterPowerConsumption/1000)
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
		inverterShare := (interval.InverterGeneratedPower - interval.BatteryDischarge) / totalInput
		batteryShare := interval.BatteryDischarge / totalInput
		gridShare := interval.GridImport / totalInput

		ea.debugf("Interval energy shares: Inverter=%.1f%% Battery=%.1f%% Grid=%.1f%%",
			inverterShare*100, batteryShare*100, gridShare*100)

		// Distribute each consumer's usage according to source percentages
		for consumerId, usage := range interval.ConsumerUsage {
			if usage <= 0 {
				continue
			}

			consumer := consumerStats[consumerId]
			consumer.Total += usage
			consumer.Sources.FromInverter += usage * inverterShare
			consumer.Sources.FromBattery += usage * batteryShare
			consumer.Sources.FromGrid += usage * gridShare

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
