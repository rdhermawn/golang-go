package webui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"golang-refine/internal/discord"
	"golang-refine/internal/refine"
)

type FeedEvent struct {
	ID          string `json:"id"`
	Sequence    int64  `json:"sequence"`
	Timestamp   string `json:"timestamp"`
	Status      string `json:"status"`
	RoleID      string `json:"roleId,omitempty"`
	PlayerName  string `json:"playerName"`
	ItemID      string `json:"itemId,omitempty"`
	ItemName    string `json:"itemName"`
	IconURL     string `json:"iconUrl,omitempty"`
	Result      string `json:"result"`
	LevelBefore int    `json:"levelBefore"`
	LevelAfter  int    `json:"levelAfter"`
	StoneID     string `json:"stoneId,omitempty"`
	StoneName   string `json:"stoneName,omitempty"`
	Message     string `json:"message"`
	StreakKey   string `json:"streakKey"`
}

type recentResponse struct {
	Events []FeedEvent `json:"events"`
}

type Hub struct {
	mu          sync.RWMutex
	recent      []FeedEvent
	subscribers map[chan FeedEvent]struct{}
	size        int
}

func NewHub(size int) *Hub {
	if size <= 0 {
		size = 200
	}
	return &Hub{
		size:        size,
		subscribers: make(map[chan FeedEvent]struct{}),
	}
}

func NewFeedEvent(event discord.RefineEvent) FeedEvent {
	timestamp := event.ObservedAt.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	itemName := refine.NormalizeItemDisplayName(event.ItemName)
	iconURL := ""
	if strings.TrimSpace(event.ItemID) != "" && strings.TrimSpace(event.IconPath) != "" {
		iconURL = "/api/icons/" + strings.TrimSpace(event.ItemID) + ".png"
	}

	return FeedEvent{
		ID:          fmt.Sprintf("%d-%s-%s", event.Sequence, event.RoleID, event.ItemID),
		Sequence:    event.Sequence,
		Timestamp:   timestamp.Format(time.RFC3339),
		Status:      "OK",
		RoleID:      event.RoleID,
		PlayerName:  event.PlayerName,
		ItemID:      event.ItemID,
		ItemName:    itemName,
		IconURL:     iconURL,
		Result:      event.Result,
		LevelBefore: event.LevelBefore,
		LevelAfter:  event.LevelAfter,
		StoneID:     event.StoneID,
		StoneName:   refine.GetStoneName(event.StoneID),
		Message: refine.BuildRefineMonitorLogMessage(
			event.RoleID,
			event.PlayerName,
			event.Result,
			itemName,
			event.LevelBefore,
			event.LevelAfter,
			event.StoneID,
		),
		StreakKey: buildStreakKey(event.RoleID, event.ItemID, event.PlayerName, itemName),
	}
}

func (h *Hub) Publish(event FeedEvent) {
	h.mu.Lock()
	h.recent = append(h.recent, event)
	sort.Slice(h.recent, func(i, j int) bool {
		return h.recent[i].Sequence < h.recent[j].Sequence
	})
	if overflow := len(h.recent) - h.size; overflow > 0 {
		h.recent = append([]FeedEvent(nil), h.recent[overflow:]...)
	}

	subscribers := make([]chan FeedEvent, 0, len(h.subscribers))
	for ch := range h.subscribers {
		subscribers = append(subscribers, ch)
	}
	h.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

func (h *Hub) SetRecent(events []FeedEvent) {
	if len(events) > h.size {
		events = events[len(events)-h.size:]
	}

	cloned := append([]FeedEvent(nil), events...)
	sort.Slice(cloned, func(i, j int) bool {
		return cloned[i].Sequence < cloned[j].Sequence
	})

	h.mu.Lock()
	h.recent = cloned
	h.mu.Unlock()
}

func (h *Hub) Recent() []FeedEvent {
	h.mu.RLock()
	events := append([]FeedEvent(nil), h.recent...)
	h.mu.RUnlock()

	sort.Slice(events, func(i, j int) bool {
		return events[i].Sequence < events[j].Sequence
	})
	return events
}

func (h *Hub) RecentJSON() ([]byte, error) {
	return json.Marshal(recentResponse{Events: h.Recent()})
}

func (h *Hub) Subscribe() (<-chan FeedEvent, func()) {
	ch := make(chan FeedEvent, 32)

	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		if _, ok := h.subscribers[ch]; ok {
			delete(h.subscribers, ch)
			close(ch)
		}
		h.mu.Unlock()
	}

	return ch, cancel
}

func buildStreakKey(roleID, itemID, playerName, itemName string) string {
	if strings.TrimSpace(roleID) != "" && strings.TrimSpace(itemID) != "" {
		return strings.TrimSpace(roleID) + ":" + strings.TrimSpace(itemID)
	}
	return normalizeKey(playerName) + ":" + normalizeKey(itemName)
}

func normalizeKey(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}
