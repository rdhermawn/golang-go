package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

var current atomic.Pointer[Config]

type Config struct {
	ServerVersion       string        `json:"serverversion"`
	Discord             DiscordConfig `json:"discord"`
	Logs                LogsConfig    `json:"logs"`
	IP                  string        `json:"ip"`
	Ports               Ports         `json:"ports"`
	SBlock              bool          `json:"s_block"`
	SReadType           int           `json:"s_readtype"`
	MaxBuffer           int           `json:"maxbuffer"`
	Version             string        `json:"version"`
	LogFile             string        `json:"log_file"`
	FormatLogPath       string        `json:"format_log_path"`
	WebEnabled          *bool         `json:"web_enabled"`
	WebAddr             string        `json:"web_addr"`
	WebRecentBufferSize int           `json:"web_recent_buffer_size"`
	LogRetentionDays    int           `json:"log_retention_days"`
}

type LogsConfig struct {
	RefineLogPath   string `json:"refine_log_path"`
	PickupLogPath   string `json:"pickup_log_path"`
	CraftLogPath    string `json:"craft_log_path"`
	SendmailLogPath string `json:"sendmail_log_path"`
}

type DiscordConfig struct {
	WebhookURL        string              `json:"webhook_url"`
	WebhookSummary    string              `json:"webhook_summary"`
	WebhookSuccess    string              `json:"webhook_success"`
	WebhookFailure    string              `json:"webhook_failure"`
	WebhookReset      string              `json:"webhook_reset"`
	WebhookDowngraded string              `json:"webhook_downgraded"`
	WebhookPickup     string              `json:"webhook_pickup"`
	WebhookCraft      string              `json:"webhook_craft"`
	WebhookSendmail   string              `json:"webhook_sendmail"`
	PickupEnabled     bool                `json:"pickup_enabled"`
	CraftEnabled      bool                `json:"craft_enabled"`
	SendmailEnabled   bool                `json:"sendmail_enabled"`
	Footer            string              `json:"footer"`
	LevelFilters      DiscordLevelFilters `json:"level_filters"`
}

type DiscordLevelFilters struct {
	Success    RefineLevelFilter `json:"success"`
	Failure    RefineLevelFilter `json:"failure"`
	Reset      RefineLevelFilter `json:"reset"`
	Downgraded RefineLevelFilter `json:"downgraded"`
}

type RefineLevelFilter struct {
	Enable   *bool `json:"enable"`
	MinLevel *int  `json:"min_level"`
	MaxLevel *int  `json:"max_level"`
}

func (f RefineLevelFilter) Enabled() bool {
	if f.Enable == nil {
		return true
	}
	return *f.Enable
}

type Ports struct {
	Client     int `json:"client"`
	Gamedbd    int `json:"gamedbd"`
	Gdeliveryd int `json:"gdeliveryd"`
	Gacd       int `json:"gacd"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	current.Store(&cfg)
	return &cfg, nil
}

func Reload(path string) error {
	_, err := Load(path)
	return err
}

func Current() *Config {
	if cfg := current.Load(); cfg != nil {
		return cfg
	}
	return &Config{}
}

func (d *DiscordConfig) GetWebhook(result string) string {
	switch result {
	case "SUCCESS":
		if d.WebhookSuccess != "" {
			return d.WebhookSuccess
		}
	case "FAILURE":
		if d.WebhookFailure != "" {
			return d.WebhookFailure
		}
	case "RESET":
		if d.WebhookReset != "" {
			return d.WebhookReset
		}
	case "DOWNGRADED":
		if d.WebhookDowngraded != "" {
			return d.WebhookDowngraded
		}
	}
	if d.WebhookURL != "" {
		return d.WebhookURL
	}
	return ""
}

func (d *DiscordConfig) GetSummaryWebhook() string {
	if d.WebhookSummary != "" {
		return d.WebhookSummary
	}
	if d.WebhookURL != "" {
		return d.WebhookURL
	}
	if d.WebhookFailure != "" {
		return d.WebhookFailure
	}
	if d.WebhookReset != "" {
		return d.WebhookReset
	}
	if d.WebhookDowngraded != "" {
		return d.WebhookDowngraded
	}
	if d.WebhookSuccess != "" {
		return d.WebhookSuccess
	}
	return ""
}

func (f RefineLevelFilter) Allows(level int) bool {
	if !f.Enabled() {
		return false
	}
	if f.MinLevel != nil && level < *f.MinLevel {
		return false
	}
	if f.MaxLevel != nil && level > *f.MaxLevel {
		return false
	}
	return true
}

func (d *DiscordConfig) ShouldSend(result string, levelBefore, levelAfter int) bool {
	switch result {
	case "SUCCESS":
		return d.LevelFilters.Success.Allows(levelAfter)
	case "FAILURE":
		return d.LevelFilters.Failure.Allows(levelBefore)
	case "RESET":
		return d.LevelFilters.Reset.Allows(levelBefore)
	case "DOWNGRADED":
		return d.LevelFilters.Downgraded.Allows(levelBefore)
	default:
		return true
	}
}

func (c *Config) IsWebEnabled() bool {
	if c == nil || c.WebEnabled == nil {
		return true
	}
	return *c.WebEnabled
}

func (c *Config) GetWebAddr() string {
	if c == nil {
		return "127.0.0.1:8080"
	}
	addr := strings.TrimSpace(c.WebAddr)
	if addr == "" {
		return "127.0.0.1:8080"
	}
	return addr
}

func (c *Config) GetWebRecentBufferSize() int {
	if c == nil || c.WebRecentBufferSize <= 0 {
		return 200
	}
	return c.WebRecentBufferSize
}

func (d *DiscordConfig) GetPickupWebhook() string {
	if d.WebhookPickup != "" {
		return d.WebhookPickup
	}
	if d.WebhookURL != "" {
		return d.WebhookURL
	}
	return ""
}

func (c *Config) GetRefineLogPath() string {
	if c.Logs.RefineLogPath != "" {
		return c.Logs.RefineLogPath
	}
	return "./logs/refine/monitor-log.txt"
}

func (c *Config) GetPickupLogPath() string {
	if c.Logs.PickupLogPath != "" {
		return c.Logs.PickupLogPath
	}
	return "./logs/pickup/pickup-log.txt"
}

func (c *Config) GetCraftLogPath() string {
	if c.Logs.CraftLogPath != "" {
		return c.Logs.CraftLogPath
	}
	return "./logs/craft/craft-log.txt"
}

func (c *Config) GetFormatLogPath() string {
	if c.FormatLogPath != "" {
		return c.FormatLogPath
	}
	return ""
}

func (c *Config) GetSendmailLogPath() string {
	if c.Logs.SendmailLogPath != "" {
		return c.Logs.SendmailLogPath
	}
	return "./logs/sendmail/sendmail-log.txt"
}

func (d *DiscordConfig) GetCraftWebhook() string {
	if d.WebhookCraft != "" {
		return d.WebhookCraft
	}
	if d.WebhookURL != "" {
		return d.WebhookURL
	}
	return ""
}

func (d *DiscordConfig) GetSendmailWebhook() string {
	if d.WebhookSendmail != "" {
		return d.WebhookSendmail
	}
	if d.WebhookURL != "" {
		return d.WebhookURL
	}
	return ""
}
