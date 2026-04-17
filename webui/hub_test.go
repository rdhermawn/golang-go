package webui

import "testing"

func TestHubRecentSortsAndTrims(t *testing.T) {
	hub := NewHub(2)
	hub.Publish(FeedEvent{Sequence: 3, PlayerName: "Bellatrix"})
	hub.Publish(FeedEvent{Sequence: 1, PlayerName: "Bellatrix"})
	hub.Publish(FeedEvent{Sequence: 2, PlayerName: "Bellatrix"})

	events := hub.Recent()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Sequence != 2 || events[1].Sequence != 3 {
		t.Fatalf("unexpected event order: %+v", events)
	}
}

func TestBuildStreakKeyPrefersIDs(t *testing.T) {
	key := buildStreakKey("123", "456", "Bellatrix", "Mithril Plate")
	if key != "123:456" {
		t.Fatalf("unexpected key %q", key)
	}
}
