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
	"golang-refine/internal/refine"
	"golang-refine/internal/sendmail"
)

type SendmailEvent struct {
	Sequence      int64
	ObservedAt    time.Time
	SrcRoleID     string
	DstRoleID     string
	SrcPlayerName string
	DstPlayerName string
	Money         int64
	ItemID        string
	ItemCount     int
	ItemName      string
	IconPath      string
}

func SendSendmailEmbed(webhookURL, footer, srcPlayerName, dstPlayerName, itemID, itemName string, money int64, itemCount int, iconPath string) (int, float64, error) {
	color := 0x00CED1

	var description string
	if money > 0 && itemID != "0" && itemCount > 0 {
		description = fmt.Sprintf("**%s** sent **%d gold** + **%d x %s** to **%s**",
			srcPlayerName, money, itemCount, itemName, dstPlayerName)
	} else if money > 0 {
		description = fmt.Sprintf("**%s** sent **%d gold** to **%s**",
			srcPlayerName, money, dstPlayerName)
	} else if itemID != "0" && itemCount > 0 {
		description = fmt.Sprintf("**%s** sent **%d x %s** to **%s**",
			srcPlayerName, itemCount, itemName, dstPlayerName)
	} else {
		description = fmt.Sprintf("**%s** sent mail to **%s**",
			srcPlayerName, dstPlayerName)
	}

	embed := map[string]interface{}{
		"color":       color,
		"title":       "📧 Mail Sent",
		"description": description,
		"footer":      map[string]interface{}{"text": footer},
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
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
	embed["thumbnail"] = map[string]interface{}{"url": "attachment://" + filename}
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

func BuildSendmailEvent(sequence int64, observedAt time.Time, srcRoleID, dstRoleID string, money int64, itemID string, itemCount int, lookupRole func(int) (string, error)) SendmailEvent {
	srcPlayerName := strings.TrimSpace(srcRoleID)
	if srcPlayerName == "" {
		srcPlayerName = "Unknown Player"
	}
	if name, err := lookupRole(refine.MustAtoi(srcRoleID)); err == nil && name != "" {
		srcPlayerName = name
	}

	dstPlayerName := strings.TrimSpace(dstRoleID)
	if dstPlayerName == "" {
		dstPlayerName = "Unknown Player"
	}
	if name, err := lookupRole(refine.MustAtoi(dstRoleID)); err == nil && name != "" {
		dstPlayerName = name
	}

	itemName := ""
	iconPath := ""
	if itemID != "0" && itemCount > 0 {
		itemName = sendmail.GetItemDisplayName(itemID)
		iconPath = sendmail.GetItemIconPath(itemID)
	}

	return SendmailEvent{
		Sequence:      sequence,
		ObservedAt:    observedAt,
		SrcRoleID:     srcRoleID,
		DstRoleID:     dstRoleID,
		SrcPlayerName: srcPlayerName,
		DstPlayerName: dstPlayerName,
		Money:         money,
		ItemID:        itemID,
		ItemCount:     itemCount,
		ItemName:      itemName,
		IconPath:      iconPath,
	}
}

func ProcessSendmailEvent(event SendmailEvent, cfg *config.Config, msgCh chan<- SendmailEvent) bool {
	if !cfg.Discord.SendmailEnabled {
		return false
	}

	fmt.Printf("[SENDMAIL] 📧 **%s** sent mail to **%s**\n",
		event.SrcPlayerName, event.DstPlayerName)
	msgCh <- event
	return true
}

func StartSendmailSender(cfg *config.Config, msgCh <-chan SendmailEvent) {
	const maxRateLimitWait = 60 * time.Second

	webhook := cfg.Discord.GetSendmailWebhook()
	if webhook == "" {
		webhook = cfg.Discord.GetWebhook("SUCCESS")
	}
	if webhook == "" {
		fmt.Println("[SENDMAIL] Warning: no webhook configured for sendmail events")
	}

	for event := range msgCh {
		attempt := 1
		for {
			statusCode, retryAfter, err := SendSendmailEmbed(
				webhook, cfg.Discord.Footer,
				event.SrcPlayerName, event.DstPlayerName,
				event.ItemID, event.ItemName,
				event.Money, event.ItemCount, event.IconPath,
			)
			if err == nil {
				if attempt > 1 {
					fmt.Printf("[OK] Sendmail sent to Discord after %d attempts\n", attempt)
				} else {
					fmt.Printf("[OK] Sendmail sent to Discord\n")
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
