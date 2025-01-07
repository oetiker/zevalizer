package models

import "time"

type SensorTag struct {
	ID           string `json:"_id"`
	Name         string `json:"name"`
	SensorsCount int    `json:"sensorsCount"`
	Color        string `json:"color"`
}

type SensorMetaData struct {
	Device_ID          string `json:"Device_ID,omitempty"`
	InvertMeasurement  bool   `json:"invertMeasurement,omitempty"`
	SmartMeterPosition string `json:"SmartMeterPosition,omitempty"`
	SubMeterCostTypes  int    `json:"subMeterCostTypes,omitempty"`
	Notes              string `json:"notes,omitempty"`
}
type Sensor struct {
	ID             string         `json:"_id"`
	Signal         string         `json:"signal"`
	Type           string         `json:"type"`
	DeviceType     string         `json:"device_type"`
	DeviceGroup    string         `json:"device_group"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	Priority       int            `json:"priority"`
	Tag            SensorTag      `json:"tag"`
	Mac            string         `json:"mac,omitempty"`
	IP             string         `json:"ip,omitempty"`
	Data           SensorMetaData `json:"data"`
	DeviceActivity int            `json:"deviceActivity"`
}

type User struct {
	UserID               string `json:"user_id"`
	SmID                 string `json:"sm_id"`
	FirstName            string `json:"first_name"`
	LastName             string `json:"last_name"`
	DeviceCount          int    `json:"device_count"`
	InstallationFinished bool   `json:"installation_finished"`
}

type SensorData struct {
	Date               time.Time `json:"date"`
	PurchaseCounter    int       `json:"CurrentEnergyPurchaseTariff1"`
	DeliveryCounter    int       `json:"CurrentEnergyDeliveryTariff1"`
	BatteryDischargeWh float64   `json:"bdWh"`
	BatteryChargeWh    float64   `json:"bcWh"`
}

type ZevData struct {
	SensorID          string          `json:"sensorId"`
	DeviceType        string          `json:"device_type"`
	SubMeterCostTypes int             `json:"subMeterCostTypes"`
	Data              []ZevSensorData `json:"data"`
}

type ZevSensorData struct {
	CreatedAt                    time.Time `json:"createdAt"`
	CurrentEnergyPurchaseTariff1 float64   `json:"CurrentEnergyPurchaseTariff1"`
	CurrentEnergyDeliveryTariff1 float64   `json:"CurrentEnergyDeliveryTariff1,omitempty"`
}
