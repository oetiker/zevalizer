package config

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

type APIConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	BaseURL  string `yaml:"baseUrl"`
}

type LowTariffConfig struct {
	StartHour int `yaml:"startHour"`
	EndHour   int `yaml:"endHour"`
}

type ZEVConfig struct {
	GridMeterID         string   `yaml:"gridMeterId"`
	ProductionIDs       []string `yaml:"productionIds"`
	ConsumerIDs         []string `yaml:"consumerIds"`
	BatterySystemIDs    []string `yaml:"batterySystemId"`    // IDs of the battery smart meter
	InverterEfficiency  float64  `yaml:"inverterEfficiency"` // Battery-to-AC efficiency (0.0-1.0), default 0.93
}
type Config struct {
	API       APIConfig       `yaml:"api"`
	LowTariff LowTariffConfig `yaml:"lowTariff"`
	ZEV       ZEVConfig       `yaml:"zev,omitempty"`
	Debug     bool
}

func Load(filename string) (*Config, error) {
	buf, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %v", err)
	}

	c := &Config{}
	err = yaml.Unmarshal(buf, c)
	if err != nil {
		return nil, fmt.Errorf("parsing yaml: %v", err)
	}

	return c, nil
}
