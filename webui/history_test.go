package webui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMonitorLogLine(t *testing.T) {
	line := "[2026-04-18 02:45:01] [OK] Role Name (1040) refined reset Mithril Plate from +5 -> +0 using Tienkang Stone"
	event, ok := ParseMonitorLogLine(7, line)
	if !ok {
		t.Fatal("expected log line to parse")
	}
	if event.Sequence != 7 {
		t.Fatalf("unexpected sequence %d", event.Sequence)
	}
	if event.Result != "RESET" {
		t.Fatalf("unexpected result %q", event.Result)
	}
	if event.RoleID != "1040" {
		t.Fatalf("unexpected role ID %q", event.RoleID)
	}
	if event.PlayerName != "Role Name" || event.ItemName != "Mithril Plate" {
		t.Fatalf("unexpected parsed names: %+v", event)
	}
	if event.StoneName != "Tienkang Stone" {
		t.Fatalf("unexpected stone name %q", event.StoneName)
	}
}

func TestParseMonitorLogLineLegacyFormat(t *testing.T) {
	line := "[2026-04-18 02:45:01] [OK] Role Name refined reset Mithril Plate from +5 -> +0 using Tienkang Stone"
	event, ok := ParseMonitorLogLine(8, line)
	if !ok {
		t.Fatal("expected legacy log line to parse")
	}
	if event.RoleID != "" {
		t.Fatalf("expected empty role ID for legacy line, got %q", event.RoleID)
	}
	if event.PlayerName != "Role Name" {
		t.Fatalf("unexpected player name %q", event.PlayerName)
	}
}

func TestParseMonitorLogLineLegacyRoleFormat(t *testing.T) {
	line := "[2026-04-18 02:45:01] [OK] Role Name (role 1040) refined reset Mithril Plate from +5 -> +0 using Tienkang Stone"
	event, ok := ParseMonitorLogLine(9, line)
	if !ok {
		t.Fatal("expected legacy role log line to parse")
	}
	if event.RoleID != "1040" {
		t.Fatalf("unexpected role ID %q", event.RoleID)
	}
	if event.PlayerName != "Role Name" {
		t.Fatalf("unexpected player name %q", event.PlayerName)
	}
}

func TestLoadMonitorHistoryKeepsNewestEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "monitor.log")
	content := "" +
		"[2026-04-18 02:44:58] [OK] Role Name (1040) refined success Mithril Plate from +0 -> +1 using Dragon Orb (5 Star)\n" +
		"[2026-04-18 02:44:59] [OK] Role Name (1040) refined success Mithril Plate from +1 -> +2 using Dragon Orb (5 Star)\n" +
		"[2026-04-18 02:45:01] [OK] Role Name (1040) refined reset Mithril Plate from +5 -> +0 using Tienkang Stone\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write history file: %v", err)
	}

	hub := NewHub(2)
	count, err := hub.LoadMonitorHistory(path)
	if err != nil {
		t.Fatalf("load history: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 history events, got %d", count)
	}

	events := hub.Recent()
	if len(events) != 2 {
		t.Fatalf("expected trimmed history, got %d entries", len(events))
	}
	if events[0].Sequence != 2 || events[1].Sequence != 3 {
		t.Fatalf("unexpected retained sequences: %+v", events)
	}
	if events[0].RoleID != "1040" || events[1].RoleID != "1040" {
		t.Fatalf("expected retained role IDs, got %+v", events)
	}
}
