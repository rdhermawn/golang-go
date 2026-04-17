package config

import (
	"encoding/json"
	"os"
	"strings"
)

type Config struct {
	ServerVersion       string        `json:"serverversion"`
	Discord             DiscordConfig `json:"discord"`
	IP                  string        `json:"ip"`
	Ports               Ports         `json:"ports"`
	SBlock              bool          `json:"s_block"`
	SReadType           int           `json:"s_readtype"`
	MaxBuffer           int           `json:"maxbuffer"`
	Version             string        `json:"version"`
	LogFile             string        `json:"log_file"`
	WebEnabled          *bool         `json:"web_enabled"`
	WebAddr             string        `json:"web_addr"`
	WebRecentBufferSize int           `json:"web_recent_buffer_size"`
}

type DiscordConfig struct {
	WebhookURL        string              `json:"webhook_url"`
	WebhookSummary    string              `json:"webhook_summary"`
	WebhookSuccess    string              `json:"webhook_success"`
	WebhookFailure    string              `json:"webhook_failure"`
	WebhookReset      string              `json:"webhook_reset"`
	WebhookDowngraded string              `json:"webhook_downgraded"`
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
	MinLevel *int `json:"min_level"`
	MaxLevel *int `json:"max_level"`
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
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
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
