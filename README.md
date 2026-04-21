# Golang Refine Monitor

A background daemon written in Go (Golang) designed to monitor game log files for refine, pickup, craft, and sendmail events.

## Features

- **Real-time Log Tailing**: Continuously reads the game server log file to detect new activities.
- **Refine Monitor**: Tracks equipment refinement events (success, failure, reset, downgraded) with level tracking.
- **Pickup Monitor**: Tracks rare item pickups by players.
- **Craft Monitor**: Tracks manufacturing/crafting events.
- **Sendmail Monitor**: Tracks mail transactions between players. Sendmail events with `money > 0` are displayed in WebUI; item-only mails (`money == 0`) are logged and sent to Discord but hidden from WebUI.
- **Data Integration**: Communicates with the Game API to look up detailed player information (role bases).
- **Discord Integration**: Sends notifications for all event types to dedicated Discord webhooks, plus hourly event summaries.
- **Search Capability**: Use the CLI to search past refine entries.
- **Web UI**: Serves a live dashboard with recent events, item icons, streaming event feed, tabbed views (Refine / Craft / Pickup / Sendmail), and a **Reload Config** button.
- **Config Hot-Reload**: Reload `configs/config.json` without restarting via Web UI or `SIGHUP` signal.
- **Log Rotation**: Automatic daily rotation for all log files with configurable retention.
- **SSL / Apache Proxy**: Optional full-stack installer sets up Apache reverse proxy with Let's Encrypt SSL.

## Project Structure

- `cmd/monitor/`: The main entry point and event handlers (`main.go`, `refine.go`, `pickup.go`, `craft.go`, `sendmail.go`).
- `internal/`: Core business logic (`config`, `discord`, `game`, `monitor`, `refine`, `pickup`, `craft`, `sendmail`, `search`, `tail`, `backfill`).
- `configs/`: Tracked sample config (`config.example.json`) plus your local runtime config (`config.json`, ignored by git).
- `scripts/`: Shell scripts for build, start, stop, and install.
- `webui/`: Embedded HTTP server and frontend assets for the live dashboard.
- `data/`: Item name tables (`RAE_Exported_Table.tab`) and item icons.
- `install.sh`: Full-stack interactive installer (Apache + SSL + systemd service).

## Requirements

- Go 1.19 or higher
- Access to the target Game Server Log File (`world2.log`)
- Access to the target Format Log File (`world2.formatlog`) — required for sendmail

## Quick Start

### 1. Clone & Build

```bash
cd /home/golang-go
go build -o monitor ./cmd/monitor/
```

### 2. Configure

Copy the example config and edit it:

```bash
cp configs/config.example.json configs/config.json
nano configs/config.json
```

Key fields to set:

| Field | Description |
|-------|-------------|
| `log_file` | Path to `world2.log` |
| `format_log_path` | Path to `world2.formatlog` (required for sendmail) |
| `discord.webhook_url` | Default Discord webhook |
| `discord.webhook_success` | Refine success webhook (optional) |
| `discord.webhook_failure` | Refine failure webhook (optional) |
| `discord.webhook_reset` | Refine reset webhook (optional) |
| `discord.webhook_downgraded` | Refine downgrade webhook (optional) |
| `discord.webhook_pickup` | Pickup webhook (optional) |
| `discord.webhook_craft` | Craft webhook (optional) |
| `discord.webhook_sendmail` | Sendmail webhook (optional) |
| `discord.pickup_enabled` | Enable pickup monitoring |
| `discord.craft_enabled` | Enable craft monitoring |
| `discord.sendmail_enabled` | Enable sendmail monitoring |
| `web_enabled` | Enable Web UI (`true`/`false`) |
| `web_addr` | Web UI bind address (default `127.0.0.1:9090`) |

### 3. Run

```bash
./monitor
```

The Web UI will be available at `http://127.0.0.1:9090` (or your configured `web_addr`).

## Full Server Installation (Apache + SSL + Systemd)

For production deployment with Apache reverse proxy and Let's Encrypt SSL:

```bash
sudo bash install.sh
```

This interactive installer will:

1. Update system packages
2. Install Apache and required modules
3. Install Go (if missing)
4. Install Certbot
5. Deploy Apache Virtual Host with reverse proxy
6. Build the Go binary
7. Create `configs/config.json` from example
8. Install systemd service (`monitor.service`)
9. Configure firewall (optional)
10. Obtain SSL certificate via Certbot
11. Start Apache and monitor service

After installation:

```bash
# Check service status
systemctl status monitor

# View logs
journalctl -u monitor -f

# Restart monitor
systemctl restart monitor

# Reload config without restart
systemctl kill -s HUP monitor
# or click "Reload Config" in Web UI
```

## CLI Options

### Search Past Refines

```bash
./monitor search-refine <player_name_or_item>
```

### Backfill Discord History

```bash
./monitor backfill-discord
```

## Web UI

The Web UI provides:

- **Live Event Stream**: Real-time SSE feed of all events
- **Tabbed Views**: Refine / Craft / Pickup / Sendmail
- **Search & Filter**: Search by player name, item name, or result type
- **Pagination**: Browse large event histories
- **Item Icons**: Displays item icons for recognized items
- **Reload Config**: Button to hot-reload `configs/config.json`

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/health` | GET | Health check |
| `/api/events/recent` | GET | Recent events (JSON) |
| `/api/events/all` | GET | All events (JSON) |
| `/api/events/stream` | GET | Server-Sent Events stream |
| `/api/reload` | POST | Hot-reload configuration |
| `/api/icons/{id}.png` | GET | Item icon image |

## Sendmail WebUI Behavior

Sendmail events are handled with special WebUI filtering:

- **`money > 0`**: Displayed in WebUI, sent to Discord, and logged to file.
- **`money == 0` (item-only)**: **Hidden from WebUI**, but still sent to Discord and logged to file.

This keeps the WebUI focused on gold transactions while ensuring all events are tracked.

## Log Files

Logs are automatically rotated daily and stored in:

- `./logs/refine/monitor-log.txt`
- `./logs/pickup/pickup-log.txt`
- `./logs/craft/craft-log.txt`
- `./logs/sendmail/sendmail-log.txt`

Retention is controlled by `log_retention_days` in `config.json` (default: 60 days).

## Config Hot-Reload

You can reload configuration without restarting the daemon:

1. **Web UI**: Click the **"Reload Config"** button in the top-right corner of the dashboard.
2. **Signal**: Send `SIGHUP` to the process:
   ```bash
   systemctl kill -s HUP monitor
   ```

## Development

### Build Frontend

```bash
cd webui/frontend
npm install
npm run build
```

The built assets in `webui/frontend/dist/` are embedded into the Go binary.

### Run Tests

```bash
go test ./...
```

## License

MIT
