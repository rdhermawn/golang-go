package refine

import "testing"

func TestParseLineTimestamp(t *testing.T) {
	rawLine := []byte("2026-04-18 05:17:30 deb gamed: info : \xd3\xc3\xbb\xa71040\xbe\xab\xc1\xb6")
	timestamp, ok := ParseLineTimestamp(rawLine)
	if !ok {
		t.Fatal("expected timestamp to parse")
	}
	if got := timestamp.Format("2006-01-02 15:04:05"); got != "2026-04-18 05:17:30" {
		t.Fatalf("unexpected timestamp %q", got)
	}
}

func TestParseLineTimestampInvalid(t *testing.T) {
	if _, ok := ParseLineTimestamp([]byte("deb gamed: info")); ok {
		t.Fatal("expected invalid line to fail timestamp parsing")
	}
}
