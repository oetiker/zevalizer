// internal/setup/analyzer.go
package setup

import (
	"fmt"
	"zevalizer/internal/api"
	"zevalizer/internal/config"
)

type Analyzer struct {
	client *api.Client
}

func NewAnalyzer(client *api.Client) *Analyzer {
	return &Analyzer{client: client}
}

func (sa *Analyzer) AnalyzeSetup(smId string) (*config.ZEVConfig, error) {
	// Get all sensors
	sensors, err := sa.client.GetSensors(smId)
	if err != nil {
		return nil, fmt.Errorf("failed to get sensors: %v", err)
	}

	zevConfig := &config.ZEVConfig{}

	// Find main grid meter
	for _, sensor := range sensors {
		if sensor.Type == "Smart Meter" &&
			sensor.DeviceType == "sub-meter" &&
			sensor.Data.SubMeterCostTypes == 1 {
			zevConfig.GridMeterID = sensor.ID + "  # " + sensor.Tag.Name
			break
		}
	}

	// Find production meters (inverters and their measurements)
	// Find production meters - only use the inverter devices
	for _, sensor := range sensors {
		if sensor.Type == "Smart Meter" &&
			sensor.DeviceType == "sub-meter" &&
			sensor.Data.SubMeterCostTypes == 2 {
			zevConfig.ProductionIDs = append(zevConfig.ProductionIDs, sensor.ID+"  # "+sensor.Tag.Name)
		}
	}

	// Find battery system meter
	for _, sensor := range sensors {
		if sensor.Type == "Battery" &&
			sensor.DeviceType == "device" {
			zevConfig.BatterySystemIDs = append(zevConfig.BatterySystemIDs, sensor.ID+"  # "+sensor.Tag.Name)
		}
	}

	// Find consumer meters
	for _, sensor := range sensors {
		if sensor.Type == "Smart Meter" &&
			sensor.DeviceType == "sub-meter" &&
			sensor.Data.SubMeterCostTypes == 0 {
			zevConfig.ConsumerIDs = append(zevConfig.ConsumerIDs, sensor.ID+"  # "+sensor.Tag.Name)

		}
	}

	return zevConfig, nil
}
