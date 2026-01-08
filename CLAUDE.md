# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run Commands

```bash
# Build the executable
go build -o zevalizer cmd/zevalizer/main.go

# Run directly (requires config.yaml)
go run cmd/zevalizer/main.go -analyze     # Discover sensors and suggest config
go run cmd/zevalizer/main.go -energy      # Analyze energy for current day
go run cmd/zevalizer/main.go -energy -days 7    # Analyze last 7 days
go run cmd/zevalizer/main.go -energy -from 2024-01-01 -to 2024-01-31
go run cmd/zevalizer/main.go -debug -energy    # Enable debug output

# Run tests
go test ./...
```

## Architecture

Zevalizer fetches energy data from Solar Manager API and calculates consumption metrics split by tariff periods and energy sources (grid, solar inverter, battery).

### Package Structure

- **cmd/zevalizer/main.go** - CLI entry point, flag parsing, output formatting
- **internal/api** - HTTP client for Solar Manager API (users, sensors, ZEV data, sensor data). Handles chunked requests for large date ranges (30-day chunks).
- **internal/config** - YAML config loading. Config structure: API credentials, low tariff hours, ZEV meter IDs
- **internal/models** - Data types for API responses: Sensor, User, SensorData, ZevData
- **internal/setup** - Auto-discovers sensors by type to suggest config values
- **internal/analyzer** - Core energy analysis logic. Creates 15-minute intervals, collects data from all sources, calculates energy distribution per consumer

### Key Data Flow

1. Config loads from `config.yaml` (gitignored, contains credentials)
2. API client fetches user's `smId` (Solar Manager ID)
3. For `-analyze`: Setup analyzer scans sensors and outputs suggested ZEV config
4. For `-energy`: Energy analyzer creates time intervals, fetches grid/inverter/battery/consumer data, then calculates per-consumer energy attribution from each source (inverter, battery, grid) based on interval-level proportions

### Configuration

The `config.yaml` file requires:
- API credentials (username, password, baseUrl)
- Low tariff hours (startHour, endHour)
- ZEV section with meter IDs (gridMeterId, productionIds, batterySystemId, consumerIds)

Run with `-analyze` first to discover available sensor IDs for your installation.
