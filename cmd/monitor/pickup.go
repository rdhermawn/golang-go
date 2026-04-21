package main

import (
	"fmt"
	"sync/atomic"
	"time"

	"golang-refine/internal/config"
	"golang-refine/internal/discord"
	"golang-refine/internal/monitor"
	"golang-refine/internal/pickup"
	"golang-refine/webui"
)

func initPickupLogger(cfg *config.Config) (*monitor.Logger, error) {
	if !cfg.Discord.PickupEnabled {
		return nil, nil
	}

	pickupLogPath := cfg.GetPickupLogPath()
	pickupLogger := monitor.NewLogger(pickupLogPath)
	if err := pickupLogger.Open(time.Now()); err != nil {
		return nil, err
	}
	fmt.Printf("Pickup log: %s\n", pickupLogger.Path())
	return pickupLogger, nil
}

func processPickupEvent(
	parsed pickup.ParsedPickupLine,
	sequence int64,
	observedAt time.Time,
	api *apiClient,
	pickupLogger *monitor.Logger,
	hub *webui.Hub,
	pickupCh chan<- discord.PickupEvent,
) {
	event := discord.BuildPickupEvent(
		sequence,
		observedAt,
		parsed.RoleID,
		parsed.ItemID,
		parsed.Count,
		api.LookupRoleBase,
	)
	if pickupLogger != nil {
		pickupLogger.LogAt(event.ObservedAt, "[PICKUP] %s picked up %d x %s",
			event.PlayerName, event.Count, event.ItemName)
	}
	hub.Publish(webui.NewPickupEvent(event))
	discord.ProcessPickupEvent(event, pickupCh)
}

func handlePickupLine(
	rawLine []byte,
	sequence *int64,
	api *apiClient,
	pickupLogger *monitor.Logger,
	hub *webui.Hub,
	pickupCh chan<- discord.PickupEvent,
) bool {
	if !config.Current().Discord.PickupEnabled {
		return false
	}

	parsed, match := pickup.ParsePickupLine(rawLine)
	if !match {
		return false
	}

	currentSequence := atomic.AddInt64(sequence, 1)
	observedAt := time.Now()
	if parsedTimestamp, ok := pickup.ParseLineTimestamp(rawLine); ok {
		observedAt = parsedTimestamp
	}

	processPickupEvent(parsed, currentSequence, observedAt, api, pickupLogger, hub, pickupCh)
	return true
}
