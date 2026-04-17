# Golang Refine Monitor

A background daemon written in Go (Golang) designed to monitor game log files for "refine" events.

## Features

- **Real-time Log Tailing**: Continuously reads the game server log file to detect new refine activities.
- **Data Integration**: Communicates with the Game API to look up detailed player information (role bases).
- **Discord Integration**: Sends notifications for successful/failed refines to a Discord server and provides hourly event summaries.
- **Search Capability**: Use the CLI to search past refine entries.
- **Web UI**: Serves a live dashboard with recent refine events, item icons, and a streaming event feed.

## Project Structure
- `cmd/refine-monitor/main.go`: The main entry point of the daemon.
- `internal/`: Contains core business logic (`config`, `discord`, `game`, `monitor`, `refine`, `search`, `tail`).
- `configs/`: Sample/default configuration files.
- `scripts/`: Shell scripts (like `monitor.sh`) to manage background processes.
- `webui/`: Embedded HTTP server and frontend assets for the live dashboard.

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

Make sure `configs/config.json` is correctly set up with the correct log file and webhook/API settings before running. When Web UI is enabled, the dashboard is served from `http://127.0.0.1:8080` by default.

Optional Web UI settings in `configs/config.json`:

```json
{
  "web_enabled": true,
  "web_addr": "127.0.0.1:8080",
  "web_recent_buffer_size": 200
}
```

Web UI config notes:

- `web_enabled`: Optional. Defaults to `true`. Set to `false` to disable the HTTP server entirely.
- `web_addr`: Optional. Defaults to `127.0.0.1:8080`. Change this if you want a different listen host or port.
- `web_recent_buffer_size`: Optional. Defaults to `200`. Controls how many recent refine events are kept in memory for the dashboard and `/api/events/recent`.

If `web_addr` is already occupied, pick a different local port such as `127.0.0.1:8081`.

## CLI Options

To run a search for past refines directly:

```bash
./refine-monitor search-refine <search_args>
```
