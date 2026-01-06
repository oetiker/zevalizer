# Zevalizer

Energy analysis tool for ZEV (Zusammenschluss zum Eigenverbrauch) billing. Fetches data from Solar Manager API and calculates consumption metrics split by tariff periods and energy sources.

## Quick Start

```bash
# Build
go build -o zevalizer cmd/zevalizer/main.go

# Discover sensors and suggest config
./zevalizer -analyze

# Analyze energy for last 7 days
./zevalizer -energy -days 7

# Analyze specific date range
./zevalizer -energy -from 2025-01-01 -to 2025-01-31

# Debug mode
./zevalizer -debug -energy -days 7
```

## Configuration

Create `config.yaml`:

```yaml
api:
  username: "your@email.com"
  password: "your-password"
  baseUrl: "https://cloud.solar-manager.ch"

lowTariff:
  startHour: 21   # Low tariff starts at 9 PM
  endHour: 6      # Low tariff ends at 6 AM

zev:
  gridMeterId: "..."        # Main grid meter
  productionIds:
    - "..."                 # Inverter production meters
  batterySystemId:
    - "..."                 # Battery system(s)
  consumerIds:
    - "..."                 # Consumer meters (flats, offices, etc.)
```

Run `./zevalizer -analyze` to discover sensor IDs for your installation.

## Command Options

| Flag | Description |
|------|-------------|
| `-analyze` | Discover sensors and suggest config values |
| `-energy` | Perform energy usage analysis |
| `-debug` | Enable detailed debug output |
| `-from` | Start date (YYYY-MM-DD or DD.MM.YYYY) |
| `-to` | End date (YYYY-MM-DD or DD.MM.YYYY) |
| `-days` | Number of days to analyze (if -from/-to not set) |
| `-no-cache` | Disable caching, fetch fresh data |
| `-clear-cache` | Delete cache before running |
| `-dump-cache` | Print cache contents and exit |

## Energy Calculation Method

### The NET Formula

For hybrid inverters that perform cross-phase power balancing, the tool uses a NET formula to calculate real production:

```
NET Production = Delivery - Purchase
```

**Why is this needed?**

Hybrid inverters with batteries often perform cross-phase power circulation for grid stability. At night (no solar), you might see on a 3-phase meter:

| Phase | Power |
|-------|-------|
| Phase 1 | -1.0 kW (export) |
| Phase 2 | +0.5 kW (import) |
| Phase 3 | -0.1 kW (export) |

The inverter draws power from one phase and pushes it to others. The meter correctly registers:
- **Purchase**: 0.5 kW (import on Phase 2)
- **Delivery**: 1.1 kW (export on Phases 1+3)

But this is just circulating power, not real production! The NET formula removes this phantom:
- **NET**: Delivery - Purchase = actual energy contribution

During the day with real solar production, NET correctly shows the actual contribution.

### Energy Balance

The tool calculates energy balance per 15-minute interval:

```
Total Input  = Grid Import + NET Production + Battery Discharge
Total Output = Consumer Usage + Grid Export
```

When Input > Output, the difference is assigned to "Shared Usage" (common areas, unmeasured loads).

### Inverter Power Flow

The NET formula can result in negative values when the inverter consumes more than it produces:

- **Positive NET**: Inverter contributing energy (solar production)
- **Negative NET**: Inverter consuming energy (standby losses, internal consumption)

This correctly models the inverter as both a producer (daytime) and consumer (nighttime standby).

### Battery Losses

Battery round-trip efficiency is typically 83-90%. The tool measures:

| Metric | Description |
|--------|-------------|
| Battery Charge | Energy going INTO the battery |
| Battery Discharge | Energy coming OUT of the battery |
| Implicit Loss | Charge - Discharge (~10-17%) |

The NET production value shows energy that **reached the house** (after losses). If comparing with apps that show energy **generated** (before losses), expect a difference roughly equal to battery losses.

### Consumer Attribution

For each consumer, the tool calculates energy source breakdown:

| Source | Description |
|--------|-------------|
| Inverter | Direct solar consumption |
| Battery | Energy from battery discharge |
| Grid | Energy imported from grid |

Attribution is proportional to available energy in each interval:

```
Consumer's Grid Share    = Consumer Usage * (Grid Import / Total Input)
Consumer's Solar Share   = Consumer Usage * (NET Production / Total Input)
Consumer's Battery Share = Consumer Usage * (Battery Discharge / Total Input)
```

## Caching

The tool caches API data locally to avoid repeated fetches:

```bash
# Normal use (caching enabled)
./zevalizer -energy -days 30

# Force fresh fetch
./zevalizer -energy -days 30 -no-cache

# Inspect cache contents
./zevalizer -dump-cache

# Clear cache and refetch
./zevalizer -clear-cache -energy -days 30
```

Cache location: `config.data-cache` (next to config file)

## Output Interpretation

### System Overview

```
Grid Import:         2477.2 kWh    # Total from grid
Grid Export:          145.6 kWh    # Total to grid
Production:          1149.2 kWh    # NET solar (after phantom removal)
Battery Discharge:    416.8 kWh    # From battery to house
Self Consumption:      86.8 %      # Production used (not exported)
Autarchy:              43.7 %      # Self-sufficiency rate
```

### Energy Balance

```
Total Input:       2334.9 kWh
Total Output:      2334.9 kWh
Difference:           0.0 kWh      # Should be ~0 if balanced
```

A non-zero difference indicates measurement errors or unaccounted flows.

### Consumer Details

```
Name              Total      Inverter    Battery      Grid
----------------------------------------------------------
WG 1            168.2 kWh    40.4 kWh   28.4 kWh   99.3 kWh
Shared Usage    563.5 kWh   169.1 kWh   69.8 kWh  324.6 kWh
```

- **Shared Usage**: Energy not attributed to any consumer (common areas, losses, unmeasured loads)

## Troubleshooting

### High "Shared Usage"

If shared usage seems too high:
1. Check for unmeasured consumers
2. Verify all consumer meters are in config
3. Check for meter measurement errors

### Negative Inverter Values at Night

This is normal! At night, the inverter consumes standby power. Negative values correctly show the inverter is taking from the system, not contributing.

### Production Doesn't Match App

Common reasons:
1. **Battery losses**: Our NET shows energy after losses; apps may show before
2. **Time boundaries**: Check lowTariff hours match your utility
3. **Different calculation methods**: Apps may use different formulas

## Compiling from Source

1. Make sure Go is installed (1.20 or later)
2. Clone this repository:
   ```bash
   git clone https://github.com/oetiker/zevalizer.git
   ```
3. Enter the project directory:
   ```bash
   cd zevalizer
   ```
4. Compile:
   ```bash
   go build -o zevalizer cmd/zevalizer/main.go
   ```
5. Run tests:
   ```bash
   go test ./...
   ```

## License

MIT
