package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"
	"zevalizer/internal/analyzer"
	"zevalizer/internal/api"
	"zevalizer/internal/config"
	"zevalizer/internal/setup"
)

func printSetupHint(zevConfig *config.ZEVConfig) {
	fmt.Printf("\nZEV Setup Hint:\n")
	fmt.Printf("Grid Meter: %s\n", zevConfig.GridMeterID)
	fmt.Printf("Production Meters: %v\n", zevConfig.ProductionIDs)
	fmt.Printf("Battery System: %v\n", zevConfig.BatterySystemIDs)
	fmt.Printf("Consumer Meters: %v\n", zevConfig.ConsumerIDs)

	// Verify completeness
	if zevConfig.GridMeterID == "" {
		fmt.Printf("\nWarning: No grid meter identified\n")
	}
	if len(zevConfig.ProductionIDs) == 0 {
		fmt.Printf("\nWarning: No production meters identified\n")
	}
	if len(zevConfig.BatterySystemIDs) == 0 {
		fmt.Printf("\nWarning: No battery system meter identified\n")
	}
	if len(zevConfig.ConsumerIDs) == 0 {
		fmt.Printf("\nWarning: No consumer meters identified\n")
	}

	// Print YAML suggestion
	fmt.Printf("\nSuggested config.yaml ZEV section:\n")
	fmt.Printf("zev:\n")
	fmt.Printf("  gridMeterId: %q\n", zevConfig.GridMeterID)
	fmt.Printf("  productionIds:\n")
	for _, id := range zevConfig.ProductionIDs {
		fmt.Printf("    - %q\n", id)
	}
	fmt.Printf("  batterySystemId:\n")
	for _, id := range zevConfig.BatterySystemIDs {
		fmt.Printf("    - %q\n", id)
	}
	fmt.Printf("  consumerIds:\n")
	for _, id := range zevConfig.ConsumerIDs {
		fmt.Printf("    - %q\n", id)
	}
}

// Update the analyzeEnergy function in main.go

func analyzeEnergy(client *api.Client, cfg *config.Config, smId string, from, to time.Time) error {
	energyAnalyzer := analyzer.NewEnergyAnalyzer(client, &cfg.ZEV, cfg.Debug)
	stats, err := energyAnalyzer.Analyze(smId, from, to)
	if err != nil {
		return fmt.Errorf("analyzing energy data: %v", err)
	}

	// Print summary
	fmt.Printf("\nEnergy Analysis for period: %s to %s\n\n",
		from.Format("2006-01-02 15:04"),
		to.Format("2006-01-02 15:04"))

	fmt.Printf("System Overview:\n")
	fmt.Printf("---------------\n")
	fmt.Printf("Grid Import:       %.1f kWh\n", stats.GridImport/1000)
	fmt.Printf("Grid Export:       %.1f kWh\n", stats.GridExport/1000)
	fmt.Printf("Production:        %.1f kWh\n", stats.Production/1000)
	fmt.Printf("Battery Charge:    %.1f kWh\n", stats.BatteryCharge/1000)
	fmt.Printf("Battery Discharge: %.1f kWh\n", stats.BatteryDischarge/1000)
	fmt.Printf("Self Consumption:  %.1f%%\n", stats.SelfConsumptionRate())
	fmt.Printf("Autarchy:         %.1f%%\n", stats.AutarchyRate())

	fmt.Printf("\nEnergy Balance:\n")
	fmt.Printf("--------------\n")
	totalInput := stats.GridImport + stats.Production + stats.BatteryDischarge
	totalOutput := stats.GridExport + stats.BatteryCharge
	for _, consumer := range stats.Consumers {
		if consumer.Sensor.Tag.Name != "Unaccounted Energy" {
			totalOutput += consumer.Total
		}
	}
	fmt.Printf("Total Input:       %.1f kWh\n", totalInput/1000)
	fmt.Printf("Total Output:      %.1f kWh\n", totalOutput/1000)
	fmt.Printf("Difference:        %.1f kWh\n", (totalInput-totalOutput)/1000)

	fmt.Printf("\nConsumer Details:\n")
	fmt.Printf("----------------\n")
	fmt.Printf("%-15s %13s %13s %13s %13s\n",
		"Name", "Total", "Solar", "Battery", "Grid")
	fmt.Printf("%s\n", strings.Repeat("-", 71))

	// First print regular consumers
	for _, consumer := range stats.Consumers {

		fmt.Printf("%-15s %9.1f kWh %9.1f kWh %9.1f kWh %9.1f kWh\n",
			consumer.Sensor.Tag.Name,
			consumer.Total/1000,
			consumer.Sources.FromSolar/1000,
			consumer.Sources.FromBattery/1000,
			consumer.Sources.FromGrid/1000)

	}

	return nil
}

func parseDate(dateStr string) (time.Time, error) {
	// Try different date formats
	formats := []string{
		"2006-01-02",
		"02.01.2006",
		"02.01.06",
	}

	var parseErr error
	for _, format := range formats {
		t, err := time.ParseInLocation(format, dateStr, time.Local)
		if err == nil {
			return t, nil
		}
		parseErr = err
	}
	return time.Time{}, fmt.Errorf("invalid date format, please use YYYY-MM-DD or DD.MM.YYYY: %v", parseErr)
}

func main() {
	var (
		startDate string
		endDate   string
		days      int
	)

	flag.StringVar(&startDate, "from", "", "Start date (format: YYYY-MM-DD or DD.MM.YYYY)")
	flag.StringVar(&endDate, "to", "", "End date (format: YYYY-MM-DD or DD.MM.YYYY)")
	flag.IntVar(&days, "days", 0, "Number of days to analyze (ignored if from/to are specified)")
	analyze := flag.Bool("analyze", false, "Analyze setup and suggest configuration")
	energy := flag.Bool("energy", false, "Show energy analysis")
	debug := flag.Bool("debug", false, "Enable debug output")
	flag.Parse()

	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	cfg.Debug = *debug

	client := api.NewClient(cfg)

	users, err := client.GetUsers()
	if err != nil {
		log.Fatalf("Failed to get users: %v", err)
	}

	if len(users) == 0 {
		log.Fatal("No users found")
	}

	smId := users[0].SmID

	if *analyze {
		setupAnalyzer := setup.NewAnalyzer(client)
		zevConfig, err := setupAnalyzer.AnalyzeSetup(smId)
		if err != nil {
			log.Fatalf("Setup analysis failed: %v", err)
		}
		printSetupHint(zevConfig)
		return
	}

	if *energy {
		// Handle time range
		var from, to time.Time
		now := time.Now()

		if startDate != "" && endDate != "" {
			// Parse dates
			from, err = parseDate(startDate)
			if err != nil {
				log.Fatalf("Invalid start date: %v", err)
			}
			to, err = parseDate(endDate)
			if err != nil {
				log.Fatalf("Invalid end date: %v", err)
			}
			// Set to start and end of days
			from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.Local)
			to = time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, 999999999, time.Local)
		} else if days > 0 {
			// Use days parameter
			to = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, time.Local)
			from = time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.Local).
				AddDate(0, 0, -days+1)
		} else {
			// Default to current day
			to = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, time.Local)
			from = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		}

		if cfg.Debug {
			fmt.Printf("Analyzing period from %s to %s\n",
				from.Format("2006-01-02 15:04:05 MST"),
				to.Format("2006-01-02 15:04:05 MST"))
		}

		if err := analyzeEnergy(client, cfg, smId, from, to); err != nil {
			log.Fatalf("Energy analysis failed: %v", err)
		}
		return
	}
}
