package webui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"golang-refine/internal/craft"
	"golang-refine/internal/discord"
	"golang-refine/internal/pickup"
	"golang-refine/internal/refine"
	"golang-refine/internal/sendmail"
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
	Count       int    `json:"count,omitempty"`
	Message     string `json:"message"`
	StreakKey   string `json:"streakKey"`
	SrcRoleID   string `json:"srcRoleId,omitempty"`
	DstRoleID   string `json:"dstRoleId,omitempty"`
	SrcPlayerName string `json:"srcPlayerName,omitempty"`
	DstPlayerName string `json:"dstPlayerName,omitempty"`
	Money       int64  `json:"money,omitempty"`
}

type recentResponse struct {
	Events []FeedEvent `json:"events"`
}

type Hub struct {
	mu          sync.RWMutex
	all         []FeedEvent
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

func NewPickupEvent(event discord.PickupEvent) FeedEvent {
	timestamp := event.ObservedAt.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	itemName := pickup.GetItemDisplayName(event.ItemID)
	iconURL := ""
	if strings.TrimSpace(event.ItemID) != "" && strings.TrimSpace(event.IconPath) != "" {
		iconURL = "/api/icons/" + strings.TrimSpace(event.ItemID) + ".png"
	}

	return FeedEvent{
		ID:         fmt.Sprintf("%d-%s-%s", event.Sequence, event.RoleID, event.ItemID),
		Sequence:   event.Sequence,
		Timestamp:  timestamp.Format(time.RFC3339),
		Status:     "PICKUP",
		RoleID:     event.RoleID,
		PlayerName: event.PlayerName,
		ItemID:     event.ItemID,
		ItemName:   itemName,
		IconURL:    iconURL,
		Result:     "PICKUP",
		Count:      event.Count,
		Message:    fmt.Sprintf("%s acquired %d x %s", event.PlayerName, event.Count, itemName),
		StreakKey:  buildStreakKey(event.RoleID, event.ItemID, event.PlayerName, itemName),
	}
}

func NewCraftEvent(event discord.CraftEvent) FeedEvent {
	timestamp := event.ObservedAt.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	itemName := craft.GetItemDisplayName(event.CraftedItemID)
	iconURL := ""
	if strings.TrimSpace(event.CraftedItemID) != "" && strings.TrimSpace(event.IconPath) != "" {
		iconURL = "/api/icons/" + strings.TrimSpace(event.CraftedItemID) + ".png"
	}

	return FeedEvent{
		ID:         fmt.Sprintf("%d-%s-%s", event.Sequence, event.RoleID, event.CraftedItemID),
		Sequence:   event.Sequence,
		Timestamp:  timestamp.Format(time.RFC3339),
		Status:     "CRAFT",
		RoleID:     event.RoleID,
		PlayerName: event.PlayerName,
		ItemID:     event.CraftedItemID,
		ItemName:   itemName,
		IconURL:    iconURL,
		Result:     "CRAFT",
		Count:      event.CraftCount,
		Message:    fmt.Sprintf("%s.png %s x%d manufactured by %s", event.CraftedItemID, itemName, event.CraftCount, event.PlayerName),
		StreakKey:  buildStreakKey(event.RoleID, event.CraftedItemID, event.PlayerName, itemName),
	}
}

func NewSendmailEvent(event discord.SendmailEvent) FeedEvent {
	timestamp := event.ObservedAt.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	itemName := sendmail.GetItemDisplayName(event.ItemID)
	iconURL := ""
	if strings.TrimSpace(event.ItemID) != "" && event.ItemID != "0" && strings.TrimSpace(event.IconPath) != "" {
		iconURL = "/api/icons/" + strings.TrimSpace(event.ItemID) + ".png"
	}

	var message string
	if event.Money > 0 && event.ItemID != "0" && event.ItemCount > 0 {
		message = fmt.Sprintf("%s -> %s: %d gold + %d x %s", event.SrcPlayerName, event.DstPlayerName, event.Money, event.ItemCount, itemName)
	} else if event.Money > 0 {
		message = fmt.Sprintf("%s -> %s: %d silver", event.SrcPlayerName, event.DstPlayerName, event.Money)
	} else if event.ItemID != "0" && event.ItemCount > 0 {
		message = fmt.Sprintf("%s -> %s: %d x %s", event.SrcPlayerName, event.DstPlayerName, event.ItemCount, itemName)
	} else {
		message = fmt.Sprintf("%s -> %s: mail", event.SrcPlayerName, event.DstPlayerName)
	}

	return FeedEvent{
		ID:            fmt.Sprintf("%d-%s-%s", event.Sequence, event.SrcRoleID, event.DstRoleID),
		Sequence:      event.Sequence,
		Timestamp:     timestamp.Format(time.RFC3339),
		Status:        "SENDMAIL",
		RoleID:        event.SrcRoleID,
		PlayerName:    event.SrcPlayerName,
		ItemID:        event.ItemID,
		ItemName:      itemName,
		IconURL:       iconURL,
		Result:        "SENDMAIL",
		Count:         event.ItemCount,
		Message:       message,
		StreakKey:     buildStreakKey(event.SrcRoleID, event.ItemID, event.SrcPlayerName, itemName),
		SrcRoleID:     event.SrcRoleID,
		DstRoleID:     event.DstRoleID,
		SrcPlayerName: event.SrcPlayerName,
		DstPlayerName: event.DstPlayerName,
		Money:         event.Money,
	}
}

func (h *Hub) LoadAllFromDirAndParser(dir string, parser func(int64, string) (FeedEvent, bool)) (int64, error) {
	events, err := loadAllEventsFromDir(dir, parser)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}
	h.SetAll(events)
	return events[len(events)-1].Sequence, nil
}

func (h *Hub) Publish(event FeedEvent) {
	h.mu.Lock()
	h.all = append(h.all, event)
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

func (h *Hub) SetAll(events []FeedEvent) {
	all := append([]FeedEvent(nil), events...)
	sort.Slice(all, func(i, j int) bool {
		return all[i].Sequence < all[j].Sequence
	})

	recent := append([]FeedEvent(nil), all...)
	if len(recent) > h.size {
		recent = recent[len(recent)-h.size:]
	}

	h.mu.Lock()
	h.all = all
	h.recent = recent
	h.mu.Unlock()
}

func (h *Hub) mergeEvents(events []FeedEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()

	bySequence := make(map[int64]FeedEvent, len(h.all)+len(events))
	for _, e := range h.all {
		bySequence[e.Sequence] = e
	}
	for _, e := range events {
		bySequence[e.Sequence] = e
	}

	merged := make([]FeedEvent, 0, len(bySequence))
	for _, e := range bySequence {
		merged = append(merged, e)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Sequence < merged[j].Sequence
	})

	h.all = append([]FeedEvent(nil), merged...)
	recent := append([]FeedEvent(nil), merged...)
	if overflow := len(recent) - h.size; overflow > 0 {
		recent = append([]FeedEvent(nil), recent[overflow:]...)
	}
	h.recent = recent
}

func (h *Hub) All() []FeedEvent {
	h.mu.RLock()
	events := append([]FeedEvent(nil), h.all...)
	h.mu.RUnlock()

	sort.Slice(events, func(i, j int) bool {
		return events[i].Sequence < events[j].Sequence
	})
	return events
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

func (h *Hub) AllJSON() ([]byte, error) {
	return json.Marshal(recentResponse{Events: h.All()})
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
