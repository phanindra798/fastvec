// Package benchenv records what a benchmark ran on. A QPS figure without the
// machine behind it can't be compared to anything.
package benchenv

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type Env struct {
	CapturedUTC string   `json:"captured_utc"`
	SourceState string   `json:"source_state"`
	CPU         string   `json:"cpu"`
	LogicalCPU  int      `json:"logical_cpus"`
	GOMAXPROCS  int      `json:"gomaxprocs"`
	Go          string   `json:"go_version"`
	OS          string   `json:"os"`
	Kernel      string   `json:"kernel"`
	LoadAvg     string   `json:"load_at_start"`
	Commit      string   `json:"commit"`
	Tags        []string `json:"build_tags"`
}

func Capture(tags ...string) Env {
	if tags == nil {
		tags = []string{}
	}
	return Env{
		CapturedUTC: time.Now().UTC().Format(time.RFC3339),
		SourceState: sourceState(),
		CPU:         cpuModel(),
		LogicalCPU:  runtime.NumCPU(),
		GOMAXPROCS:  runtime.GOMAXPROCS(0),
		Go:          runtime.Version(),
		OS:          runtime.GOOS + "/" + runtime.GOARCH,
		Kernel:      firstLine("/proc/sys/kernel/osrelease"),
		LoadAvg:     firstLine("/proc/loadavg"),
		Commit:      commit(),
		Tags:        tags,
	}
}

func cpuModel() string {
	if runtime.GOOS == "windows" {
		out, err := exec.Command("reg", "query", `HKLM\HARDWARE\DESCRIPTION\System\CentralProcessor\0`, "/v", "ProcessorNameString").Output()
		if err == nil {
			if _, value, ok := strings.Cut(string(out), "REG_SZ"); ok {
				return strings.TrimSpace(value)
			}
		}
		if value := os.Getenv("PROCESSOR_IDENTIFIER"); value != "" {
			return value
		}
	}
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "unknown"
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if name, val, ok := strings.Cut(sc.Text(), ":"); ok && strings.TrimSpace(name) == "model name" {
			return strings.TrimSpace(val)
		}
	}
	return "unknown"
}

func sourceState() string {
	out, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return "unknown"
	}
	if len(strings.TrimSpace(string(out))) > 0 {
		return "dirty"
	}
	return "clean"
}

func firstLine(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "unknown"
	}
	line, _, _ := strings.Cut(string(b), "\n")
	return strings.TrimSpace(line)
}

func commit() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
