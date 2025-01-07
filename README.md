# Zevalizer

Zevalizer is a tool for gathering and analyzing energy usage data. It:
- Fetches sensor and ZEV data from Solar Manager.
- Identifies grid, production, battery, and consumer components.
- Analyzes energy flows to calculate consumption, production, and battery usage metrics.

## Usage

1. Adjust the config.yaml file with your credentials and meter IDs.
2. Run “go build” or “go run cmd/zevalizer/main.go” with appropriate flags.
   - “-analyze” to identify available sensors.
   - “-energy” to perform energy usage analysis.

## Command Options

- **-analyze**  
  Analyze all detected sensors on a Solar Manager instance and suggest configuration values.
- **-energy**  
  Perform energy usage analysis for a specified time range.
- **-debug**  
  Enable debug logging output for detailed tracing.
- **-from**  
  Specify a start date (format: YYYY-MM-DD or DD.MM.YYYY).
- **-to**  
  Specify an end date (format: YYYY-MM-DD or DD.MM.YYYY).
- **-days**  
  Number of days to analyze, used if -from and -to are not set.

## Compiling from Source

1. Make sure Go is installed (1.20 or later).
2. Clone this repository:  
   git clone https://github.com/oetiker/zevalizer.git
3. Enter the project directory:  
   cd zevalizer
4. Compile the Zevalizer executable:  
   go build -o zevalizer cmd/zevalizer/main.go
5. Optionally, run tests (if any):  
   go test ./...