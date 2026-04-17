# Golang Refine Monitor

A background daemon written in Go (Golang) designed to monitor game log files for "refine" events.

## Features

- **Real-time Log Tailing**: Continuously reads the game server log file to detect new refine activities.
- **Data Integration**: Communicates with the Game API to look up detailed player information (role bases).
- **Discord Integration**: Sends notifications for successful/failed refines to a Discord server and provides hourly event summaries.
- **Search Capability**: Use the CLI to search past refine entries.

## Project Structure

- `cmd/refine-monitor/main.go`: The main entry point of the daemon.
- `internal/`: Contains core business logic (`config`, `discord`, `game`, `monitor`, `refine`, `search`, `tail`).
- `configs/`: Sample/default configuration files.
- `scripts/`: Shell scripts (like `monitor.sh`) to manage background processes.

## Requirements

- Go 1.19 or higher
- Access to the target Game Server Log File (configured in `configs/config.json`)

## Building and Running

To build the executable:

```bash
go build -o refine-monitor cmd/refine-monitor/main.go
```

To run the daemon:

```bash
./refine-monitor
```

Make sure `configs/config.json` is correctly set up with the correct log file and webhook/API settings before running.

## CLI Options

To run a search for past refines directly:

```bash
./refine-monitor search-refine <search_args>
```
