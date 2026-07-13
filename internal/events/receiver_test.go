package events

import "testing"

func TestParsePRI(t *testing.T) {
	f, s, b := parsePRI("<134>1 test message")
	if f == nil || s == nil || *f != 16 || *s != 6 || b != "1 test message" {
		t.Fatalf("unexpected %v %v %q", f, s, b)
	}
	f, s, b = parsePRI("plain")
	if f != nil || s != nil || b != "plain" {
		t.Fatal("plain message parsing failed")
	}
}
