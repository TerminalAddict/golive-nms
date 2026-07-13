package main

import (
	"context"
	"testing"
)

func TestMetricsContainPortableBasics(t *testing.T) {
	m := metrics(context.Background())
	if _, ok := m["cpuCount"]; !ok {
		t.Fatal("cpuCount missing")
	}
	if _, ok := m["memoryTotalBytes"]; !ok {
		t.Fatal("memoryTotalBytes missing")
	}
}
