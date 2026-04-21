package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang-refine/internal/config"
	"golang-refine/internal/monitor"
	"golang-refine/internal/pickup"
	"golang-refine/internal/refine"
)

type PickupEvent struct {
	Sequence   int64
	ObservedAt time.Time
	RoleID     string
	ItemID     string
	Count      int
	PlayerName string
	ItemName   string
	IconPath   string
}

func SendPickupEmbed(webhookURL, footer, playerName, itemID, itemName string, count int, iconPath string) (int, float64, error) {
	color := 0x00BFFF

	embed := map[string]interface{}{
		"color": color,
		"author": map[string]interface{}{
			"name": pickup.GetItemDisplayName(itemID),
		},
		"description": fmt.Sprintf("**%s** picked up **%d** x %s",
			playerName, count, itemName),
		"footer":    map[string]interface{}{"text": footer},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	payload := map[string]interface{}{"embeds": []map[string]interface{}{embed}}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return 0, 0, fmt.Errorf("marshal embed payload: %w", err)
	}

	if iconPath == "" {
		resp, err := Client.Post(webhookURL, "application/json", bytes.NewReader(jsonData))
		if err != nil {
			return 0, 0, fmt.Errorf("error sending embed to Discord: %w", err)
		}
		defer DrainAndClose(resp)
		if resp.StatusCode == 429 {
			retryAfter, _ := strconv.ParseFloat(resp.Header.Get("Retry-After"), 64)
			return 429, retryAfter, fmt.Errorf("discord rate limited 429")
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return resp.StatusCode, 0, fmt.Errorf("discord webhook returned status %d", resp.StatusCode)
		}
		return resp.StatusCode, 0, nil
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	filename := filepath.Base(iconPath)
	embed["author"].(map[string]interface{})["icon_url"] = "attachment://" + filename
	jsonData, err = json.Marshal(payload)
	if err != nil {
		return 0, 0, fmt.Errorf("marshal embed payload with icon: %w", err)
	}

	jsonPart, err := writer.CreateFormField("payload_json")
	if err != nil {
		return 0, 0, fmt.Errorf("create json form field: %w", err)
	}
	if _, err := jsonPart.Write(jsonData); err != nil {
		return 0, 0, fmt.Errorf("write json form field: %w", err)
	}

	filePart, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return 0, 0, fmt.Errorf("error creating form file: %w", err)
	}

	f, err := os.Open(iconPath)
	if err != nil {
		return 0, 0, fmt.Errorf("error opening icon file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(filePart, f); err != nil {
		return 0, 0, fmt.Errorf("error copying icon file: %w", err)
	}
	writer.Close()

	resp, err := Client.Post(webhookURL, writer.FormDataContentType(), &buf)
	if err != nil {
		return 0, 0, fmt.Errorf("error sending embed to Discord: %w", err)
	}
	defer DrainAndClose(resp)
	if resp.StatusCode == 429 {
		retryAfter, _ := strconv.ParseFloat(resp.Header.Get("Retry-After"), 64)
		return 429, retryAfter, fmt.Errorf("discord rate limited 429")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, 0, fmt.Errorf("discord webhook returned status %d", resp.StatusCode)
	}
	return resp.StatusCode, 0, nil
}

func BuildPickupEvent(sequence int64, observedAt time.Time, roleID, itemID string, count int, lookupRole func(int) (string, error)) PickupEvent {
	playerName := strings.TrimSpace(roleID)
	if playerName == "" {
		playerName = "Unknown Player"
	}
	if name, err := lookupRole(refine.MustAtoi(roleID)); err == nil && name != "" {
		playerName = name
	}

	itemName := pickup.GetItemDisplayName(itemID)
	iconPath := pickup.GetItemIconPath(itemID)

	return PickupEvent{
		Sequence:   sequence,
		ObservedAt: observedAt,
		RoleID:     roleID,
		ItemID:     itemID,
		Count:      count,
		PlayerName: playerName,
		ItemName:   itemName,
		IconPath:   iconPath,
	}
}

func ProcessPickupEvent(event PickupEvent, msgCh chan<- PickupEvent) bool {
	if !config.Current().Discord.PickupEnabled {
		return false
	}

	fmt.Printf("[PICKUP] 🎒 **%s** picked up **%d** x %s\n",
		event.PlayerName, event.Count, event.ItemName)
	msgCh <- event
	return true
}

func StartPickupSender(msgCh <-chan PickupEvent) {
	const maxRateLimitWait = 60 * time.Second

	for event := range msgCh {
		cfg := config.Current()
		webhook := cfg.Discord.GetPickupWebhook()
		if webhook == "" {
			webhook = cfg.Discord.GetWebhook("SUCCESS")
		}
		if webhook == "" {
			fmt.Println("[PICKUP] Warning: no webhook configured for pickup events")
			continue
		}

		attempt := 1
		for {
			statusCode, retryAfter, err := SendPickupEmbed(
				webhook, cfg.Discord.Footer,
				event.PlayerName, event.ItemID, event.ItemName,
				event.Count, event.IconPath,
			)
			if err == nil {
				if attempt > 1 {
					fmt.Printf("[OK] Pickup sent to Discord after %d attempts\n", attempt)
				} else {
					fmt.Printf("[OK] Pickup sent to Discord\n")
				}
				break
			}

			if statusCode == 429 {
				wait := 2 * time.Second
				if retryAfter > 0 {
					wait = time.Duration(retryAfter * float64(time.Second))
				}
				if wait > maxRateLimitWait {
					wait = maxRateLimitWait
				}
				fmt.Printf("Rate limited, waiting %.1fs (attempt %d)\n", wait.Seconds(), attempt)
				time.Sleep(wait)
				attempt++
				continue
			}

			fmt.Printf("[ERROR] %v\n", err)
			monitor.LogToFileAt(event.ObservedAt, "[ERROR] %v", err)
			break
		}
	}
}
