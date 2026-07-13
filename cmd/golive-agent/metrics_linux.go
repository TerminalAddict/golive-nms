//go:build linux

package main

import (
	"context"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var cpuState struct {
	sync.Mutex
	total, idle uint64
}

func metrics(ctx context.Context) map[string]any {
	m := map[string]any{"cpuCount": runtime.NumCPU()}
	readOS(m)
	readScalar(m, "/proc/uptime", "uptimeSeconds")
	readLoad(m)
	readMemory(m)
	readCPU(m)
	readDisk(m)
	readNetwork(m)
	readUpdates(ctx, m)
	if entries, e := os.ReadDir("/proc"); e == nil {
		count := 0
		for _, v := range entries {
			if _, e = strconv.Atoi(v.Name()); e == nil {
				count++
			}
		}
		m["processCount"] = count
	}
	return m
}

var updateState struct {
	sync.Mutex
	at      time.Time
	manager string
	count   int
}

func readOS(m map[string]any) {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), "\"")
		switch key {
		case "ID":
			m["osId"] = value
		case "PRETTY_NAME":
			m["osName"] = value
		case "VERSION_ID":
			m["osVersion"] = value
		}
	}
}

// Update checks use the host package manager when available; the agent itself
// remains a static binary with no runtime library dependency. Results are cached
// because package metadata scans can be relatively expensive.
func readUpdates(ctx context.Context, m map[string]any) {
	updateState.Lock()
	defer updateState.Unlock()
	if time.Since(updateState.at) < time.Hour {
		m["packageManager"], m["pendingUpdates"] = updateState.manager, updateState.count
		return
	}
	manager, count := pendingUpdates(ctx)
	updateState.at, updateState.manager, updateState.count = time.Now(), manager, count
	m["packageManager"], m["pendingUpdates"] = manager, count
}

func pendingUpdates(parent context.Context) (string, int) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	type candidate struct {
		name   string
		args   []string
		prefix string
	}
	for _, c := range []candidate{
		{"apt", []string{"apt-get", "-s", "-o", "Debug::NoLocking=1", "upgrade"}, "Inst "},
		{"dnf", []string{"dnf", "--quiet", "check-update"}, ""},
		{"yum", []string{"yum", "--quiet", "check-update"}, ""},
		{"apk", []string{"apk", "version", "-l", "<"}, ""},
	} {
		if _, err := exec.LookPath(c.args[0]); err != nil {
			continue
		}
		out, _ := exec.CommandContext(ctx, c.args[0], c.args[1:]...).Output()
		count := 0
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if c.prefix != "" {
				if strings.HasPrefix(line, c.prefix) {
					count++
				}
			} else if line != "" && !strings.HasPrefix(line, "Last metadata") && !strings.HasPrefix(line, "Obsoleting") && len(strings.Fields(line)) >= 2 {
				count++
			}
		}
		return c.name, count
	}
	return "source/unknown", 0
}
func readScalar(m map[string]any, path, key string) {
	if b, e := os.ReadFile(path); e == nil {
		f := strings.Fields(string(b))
		if len(f) > 0 {
			if v, e := strconv.ParseFloat(f[0], 64); e == nil {
				m[key] = v
			}
		}
	}
}
func readLoad(m map[string]any) {
	if b, e := os.ReadFile("/proc/loadavg"); e == nil {
		f := strings.Fields(string(b))
		if len(f) >= 3 {
			m["load1"], _ = strconv.ParseFloat(f[0], 64)
			m["load5"], _ = strconv.ParseFloat(f[1], 64)
			m["load15"], _ = strconv.ParseFloat(f[2], 64)
		}
	}
}
func readMemory(m map[string]any) {
	b, e := os.ReadFile("/proc/meminfo")
	if e != nil {
		return
	}
	values := map[string]uint64{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 {
			v, _ := strconv.ParseUint(f[1], 10, 64)
			values[strings.TrimSuffix(f[0], ":")] = v * 1024
		}
	}
	m["memoryTotalBytes"] = values["MemTotal"]
	m["memoryAvailableBytes"] = values["MemAvailable"]
	m["memoryUsedBytes"] = values["MemTotal"] - values["MemAvailable"]
	m["swapTotalBytes"] = values["SwapTotal"]
	m["swapFreeBytes"] = values["SwapFree"]
}
func readCPU(m map[string]any) {
	b, e := os.ReadFile("/proc/stat")
	if e != nil {
		return
	}
	line := strings.SplitN(string(b), "\n", 2)[0]
	f := strings.Fields(line)
	if len(f) < 5 {
		return
	}
	var total uint64
	for _, x := range f[1:] {
		v, _ := strconv.ParseUint(x, 10, 64)
		total += v
	}
	idle, _ := strconv.ParseUint(f[4], 10, 64)
	if len(f) > 5 {
		wait, _ := strconv.ParseUint(f[5], 10, 64)
		idle += wait
	}
	cpuState.Lock()
	if cpuState.total > 0 && total > cpuState.total {
		delta := total - cpuState.total
		idleDelta := idle - cpuState.idle
		m["cpuUsedPercent"] = (1 - float64(idleDelta)/float64(delta)) * 100
	}
	cpuState.total, cpuState.idle = total, idle
	cpuState.Unlock()
}
func readDisk(m map[string]any) {
	var stat unix.Statfs_t
	if unix.Statfs("/", &stat) == nil {
		total := stat.Blocks * uint64(stat.Bsize)
		free := stat.Bavail * uint64(stat.Bsize)
		m["rootDiskTotalBytes"] = total
		m["rootDiskFreeBytes"] = free
		m["rootDiskUsedBytes"] = total - free
	}
}
func readNetwork(m map[string]any) {
	b, e := os.ReadFile("/proc/net/dev")
	if e != nil {
		return
	}
	var rx, tx uint64
	for _, line := range strings.Split(string(b), "\n")[2:] {
		parts := strings.Split(line, ":")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "lo" {
			continue
		}
		f := strings.Fields(parts[1])
		if len(f) >= 9 {
			a, _ := strconv.ParseUint(f[0], 10, 64)
			z, _ := strconv.ParseUint(f[8], 10, 64)
			rx += a
			tx += z
		}
	}
	m["networkReceiveBytesTotal"] = rx
	m["networkTransmitBytesTotal"] = tx
}
