package monitor

import "testing"

func TestMetricEscape(t *testing.T) {
	got := metricEscape("a\"b\\c\n")
	want := `a\"b\\c\n`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
