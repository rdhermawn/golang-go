package main

import (
	"fmt"
	"sync/atomic"
	"time"

	"golang-refine/internal/config"
	"golang-refine/internal/craft"
	"golang-refine/internal/discord"
	"golang-refine/internal/monitor"
	"golang-refine/webui"
)

func initCraftLogger(cfg *config.Config) (*monitor.Logger, error) {
	if !cfg.Discord.CraftEnabled {
		return nil, nil
	}

	craftLogPath := cfg.GetCraftLogPath()
	craftLogger := monitor.NewLogger(craftLogPath)
	if err := craftLogger.Open(time.Now()); err != nil {
		return nil, err
	}
	fmt.Printf("Craft log: %s\n", craftLogger.Path())
	return craftLogger, nil
}

func processCraftEvent(
	parsed craft.ParsedCraftLine,
	sequence int64,
	observedAt time.Time,
	api *apiClient,
	craftLogger *monitor.Logger,
	hub *webui.Hub,
	cfg *config.Config,
	craftCh chan<- discord.CraftEvent,
) {
	event := discord.BuildCraftEvent(
		sequence,
		observedAt,
		parsed.RoleID,
		parsed.CraftedItemID,
		parsed.MaterialItemID,
		parsed.CraftCount,
		parsed.MaterialCount,
		api.LookupRoleBase,
	)
	if craftLogger != nil {
		craftLogger.LogAt(event.ObservedAt, "[CRAFT] %s x%d manufactured by %s",
			event.CraftedItemName, event.CraftCount, event.PlayerName)
	}
	hub.Publish(webui.NewCraftEvent(event))
	discord.ProcessCraftEvent(event, cfg, craftCh)
}

func handleCraftLine(
	rawLine []byte,
	sequence *int64,
	api *apiClient,
	craftLogger *monitor.Logger,
	hub *webui.Hub,
	cfg *config.Config,
	craftCh chan<- discord.CraftEvent,
) bool {
	if !cfg.Discord.CraftEnabled {
		return false
	}

	parsed, match := craft.ParseCraftLine(rawLine)
	if !match {
		return false
	}

	currentSequence := atomic.AddInt64(sequence, 1)
	observedAt := time.Now()
	if parsedTimestamp, ok := craft.ParseLineTimestamp(rawLine); ok {
		observedAt = parsedTimestamp
	}

	processCraftEvent(parsed, currentSequence, observedAt, api, craftLogger, hub, cfg, craftCh)
	return true
}
