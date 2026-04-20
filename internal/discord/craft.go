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
	"golang-refine/internal/craft"
	"golang-refine/internal/monitor"
	"golang-refine/internal/refine"
)

type CraftEvent struct {
	Sequence       int64
	ObservedAt     time.Time
	RoleID         string
	CraftCount     int
	CraftedItemID  string
	MaterialItemID string
	MaterialCount  int
	PlayerName     string
	CraftedItemName string
	MaterialName   string
	IconPath       string
}

func SendCraftEmbed(webhookURL, footer, roleID, playerName, craftedItemID, craftedItemName string, craftCount int, materialItemID, materialName string, materialCount int, iconPath string) (int, float64, error) {
	color := 0x9B59B6

	embed := map[string]interface{}{
		"color": color,
		"author": map[string]interface{}{
			"name": fmt.Sprintf("%s x%d manufactured by %s",
				craft.GetItemDisplayName(craftedItemID), craftCount, playerName),
		},
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

func BuildCraftEvent(sequence int64, observedAt time.Time, roleID, craftedItemID, materialItemID string, craftCount, materialCount int, lookupRole func(int) (string, error)) CraftEvent {
	playerName := strings.TrimSpace(roleID)
	if playerName == "" {
		playerName = "Unknown Player"
	}
	if name, err := lookupRole(refine.MustAtoi(roleID)); err == nil && name != "" {
		playerName = name
	}

	craftedItemName := craft.GetItemDisplayName(craftedItemID)
	materialName := craft.GetItemDisplayName(materialItemID)
	iconPath := craft.GetItemIconPath(craftedItemID)

	return CraftEvent{
		Sequence:        sequence,
		ObservedAt:      observedAt,
		RoleID:          roleID,
		CraftCount:      craftCount,
		CraftedItemID:   craftedItemID,
		MaterialItemID:  materialItemID,
		MaterialCount:   materialCount,
		PlayerName:      playerName,
		CraftedItemName: craftedItemName,
		MaterialName:    materialName,
		IconPath:        iconPath,
	}
}

func ProcessCraftEvent(event CraftEvent, cfg *config.Config, msgCh chan<- CraftEvent) bool {
	if !cfg.Discord.CraftEnabled {
		return false
	}

	fmt.Printf("[CRAFT] 🛠️ **%s** crafted **%d** x %s (used %d x %s)\n",
		event.PlayerName, event.CraftCount, event.CraftedItemName, event.MaterialCount, event.MaterialName)
	msgCh <- event
	return true
}

func StartCraftSender(cfg *config.Config, msgCh <-chan CraftEvent) {
	const maxRateLimitWait = 60 * time.Second

	webhook := cfg.Discord.GetCraftWebhook()
	if webhook == "" {
		webhook = cfg.Discord.GetWebhook("SUCCESS")
	}
	if webhook == "" {
		fmt.Println("[CRAFT] Warning: no webhook configured for craft events")
	}

	for event := range msgCh {
		attempt := 1
		for {
			statusCode, retryAfter, err := SendCraftEmbed(
				webhook, cfg.Discord.Footer,
				event.RoleID, event.PlayerName, event.CraftedItemID, event.CraftedItemName,
				event.CraftCount, event.MaterialItemID, event.MaterialName,
				event.MaterialCount, event.IconPath,
			)
			if err == nil {
				if attempt > 1 {
					fmt.Printf("[OK] Craft sent to Discord after %d attempts\n", attempt)
				} else {
					fmt.Printf("[OK] Craft sent to Discord\n")
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
