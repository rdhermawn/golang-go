package config

import "testing"

func boolPtr(v bool) *bool {
	return &v
}

func intPtr(v int) *int {
	return &v
}

func TestRefineLevelFilterEnabledDefaultsToTrue(t *testing.T) {
	filter := RefineLevelFilter{
		MinLevel: intPtr(1),
		MaxLevel: intPtr(3),
	}

	if !filter.Enabled() {
		t.Fatal("expected nil enable to default to true")
	}
	if !filter.Allows(2) {
		t.Fatal("expected level inside range to be allowed")
	}
}

func TestRefineLevelFilterDisabledBlocksAllLevels(t *testing.T) {
	filter := RefineLevelFilter{
		Enable:   boolPtr(false),
		MinLevel: intPtr(1),
		MaxLevel: intPtr(12),
	}

	if filter.Allows(6) {
		t.Fatal("expected disabled filter to block level")
	}
}

func TestDiscordShouldSendByResultLevel(t *testing.T) {
	cfg := DiscordConfig{
		LevelFilters: DiscordLevelFilters{
			Success: RefineLevelFilter{
				MinLevel: intPtr(5),
				MaxLevel: intPtr(5),
			},
			Failure: RefineLevelFilter{
				MinLevel: intPtr(7),
				MaxLevel: intPtr(7),
			},
			Reset: RefineLevelFilter{
				MinLevel: intPtr(8),
				MaxLevel: intPtr(8),
			},
			Downgraded: RefineLevelFilter{
				MinLevel: intPtr(9),
				MaxLevel: intPtr(9),
			},
		},
	}

	tests := []struct {
		name        string
		result      string
		levelBefore int
		levelAfter  int
		want        bool
	}{
		{name: "success uses level after", result: "SUCCESS", levelBefore: 4, levelAfter: 5, want: true},
		{name: "success blocks unmatched level after", result: "SUCCESS", levelBefore: 5, levelAfter: 6, want: false},
		{name: "failure uses level before", result: "FAILURE", levelBefore: 7, levelAfter: 7, want: true},
		{name: "failure blocks unmatched level before", result: "FAILURE", levelBefore: 6, levelAfter: 6, want: false},
		{name: "reset uses level before", result: "RESET", levelBefore: 8, levelAfter: 0, want: true},
		{name: "downgraded uses level before", result: "DOWNGRADED", levelBefore: 9, levelAfter: 8, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.ShouldSend(tt.result, tt.levelBefore, tt.levelAfter); got != tt.want {
				t.Fatalf("ShouldSend(%q, %d, %d) = %v, want %v", tt.result, tt.levelBefore, tt.levelAfter, got, tt.want)
			}
		})
	}
}

func TestDiscordShouldSendHonorsDisableFlag(t *testing.T) {
	cfg := DiscordConfig{
		LevelFilters: DiscordLevelFilters{
			Failure: RefineLevelFilter{
				Enable:   boolPtr(false),
				MinLevel: intPtr(1),
				MaxLevel: intPtr(12),
			},
		},
	}

	if cfg.ShouldSend("FAILURE", 10, 10) {
		t.Fatal("expected disabled failure filter to block Discord send")
	}
}
