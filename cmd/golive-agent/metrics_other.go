//go:build !linux

package main

import (
	"context"
	"runtime"
)

func metrics(_ context.Context) map[string]any { return map[string]any{"cpuCount": runtime.NumCPU()} }
