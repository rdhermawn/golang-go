package main

import (
	"fmt"
	"sync/atomic"
	"time"

	"golang-refine/internal/config"
	"golang-refine/internal/discord"
	"golang-refine/internal/monitor"
	"golang-refine/internal/sendmail"
	"golang-refine/webui"
)

func initSendmailLogger(cfg *config.Config) (*monitor.Logger, error) {
	if !cfg.Discord.SendmailEnabled {
		return nil, nil
	}

	sendmailLogPath := cfg.GetSendmailLogPath()
	sendmailLogger := monitor.NewLogger(sendmailLogPath)
	if err := sendmailLogger.Open(time.Now()); err != nil {
		return nil, err
	}
	fmt.Printf("Sendmail log: %s\n", sendmailLogger.Path())
	return sendmailLogger, nil
}

func processSendmailEvent(
	parsed sendmail.ParsedSendmailLine,
	sequence int64,
	observedAt time.Time,
	api *apiClient,
	sendmailLogger *monitor.Logger,
	hub *webui.Hub,
	sendmailCh chan<- discord.SendmailEvent,
) {
	event := discord.BuildSendmailEvent(
		sequence,
		observedAt,
		parsed.SrcRoleID,
		parsed.DstRoleID,
		parsed.Money,
		parsed.ItemID,
		parsed.ItemCount,
		api.LookupRoleBase,
	)

	message := sendmail.BuildMonitorMessage(
		event.SrcPlayerName, event.DstPlayerName,
		event.SrcRoleID, event.DstRoleID,
		event.Money, event.ItemID, event.ItemCount,
	)

	if sendmailLogger != nil {
		sendmailLogger.LogAt(event.ObservedAt, "[SENDMAIL] %s", message)
	}
	if event.Money > 0 {
		hub.Publish(webui.NewSendmailEvent(event))
	}
	discord.ProcessSendmailEvent(event, sendmailCh)
}

func handleSendmailLine(
	rawLine []byte,
	sequence *int64,
	api *apiClient,
	sendmailLogger *monitor.Logger,
	hub *webui.Hub,
	sendmailCh chan<- discord.SendmailEvent,
) bool {
	if !config.Current().Discord.SendmailEnabled {
		return false
	}

	parsed, match := sendmail.ParseSendmailLine(rawLine)
	if !match {
		return false
	}

	currentSequence := atomic.AddInt64(sequence, 1)
	observedAt := time.Now()
	if parsedTimestamp, ok := sendmail.ParseLineTimestamp(rawLine); ok {
		observedAt = parsedTimestamp
	}

	processSendmailEvent(parsed, currentSequence, observedAt, api, sendmailLogger, hub, sendmailCh)
	return true
}
