package main

import (
	"context"
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type MemoryGovernor struct {
	limit    uint64
	high     float64
	critical float64
	tick     time.Duration

	state  atomic.Int32
	used   atomic.Uint64
	lastGC atomic.Int64
	gcRuns atomic.Int64

	heavy chan struct{}
	once  sync.Once
}

const (
	StateOK       = 0
	StateHigh     = 1
	StateCritical = 2
)

func NewMemoryGovernor(limitBytes uint64, high, critical float64, tick time.Duration, maxHeavy int) *MemoryGovernor {
	if limitBytes == 0 {
		limitBytes = detectMemLimit()
	}
	if high <= 0 || high >= 1 {
		high = 0.80
	}
	if critical <= high || critical >= 1 {
		critical = 0.92
	}
	if maxHeavy <= 0 {
		maxHeavy = runtime.NumCPU() * 2
	}
	g := &MemoryGovernor{
		limit: limitBytes, high: high, critical: critical, tick: tick,
		heavy: make(chan struct{}, maxHeavy),
	}
	debug.SetMemoryLimit(int64(float64(limitBytes) * 0.85))
	debug.SetGCPercent(50)
	return g
}

func detectMemLimit() uint64 {
	paths := []string{"/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		s := strings.TrimSpace(string(b))
		if s == "max" {
			continue
		}
		if n, err := strconv.ParseUint(s, 10, 64); err == nil && n > 0 && n < (1<<44) {
			return n
		}
	}
	return 512 * 1024 * 1024
}

func (g *MemoryGovernor) Start(ctx context.Context) {
	g.once.Do(func() {
		go func() {
			t := time.NewTicker(g.tick)
			defer t.Stop()
			var ms runtime.MemStats
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					runtime.ReadMemStats(&ms)
					used := ms.HeapInuse + ms.StackInuse + ms.MSpanInuse + ms.MCacheInuse
					g.used.Store(used)
					ratio := float64(used) / float64(g.limit)

					switch {
					case ratio >= g.critical:
						if g.state.Swap(StateCritical) != StateCritical {
							log.Printf("[mem] CRITICAL %.1f%% (%s/%s) — og'ir so'rovlar rad etiladi",
								ratio*100, human(used), human(g.limit))
						}
						g.forceRelease()
					case ratio >= g.high:
						if g.state.Swap(StateHigh) != StateHigh {
							log.Printf("[mem] HIGH %.1f%% (%s/%s) — throttling yoqildi",
								ratio*100, human(used), human(g.limit))
						}
						g.forceRelease()
					default:
						if g.state.Swap(StateOK) != StateOK {
							log.Printf("[mem] OK %.1f%% — normal rejim", ratio*100)
						}
					}
				}
			}
		}()
	})
}

func (g *MemoryGovernor) forceRelease() {
	now := time.Now().UnixNano()
	last := g.lastGC.Load()
	if now-last < int64(2*time.Second) {
		return
	}
	if !g.lastGC.CompareAndSwap(last, now) {
		return
	}
	go func() {
		runtime.GC()
		debug.FreeOSMemory()
		g.gcRuns.Add(1)
	}()
}

func (g *MemoryGovernor) State() int32 { return g.state.Load() }

func (g *MemoryGovernor) Busy() bool { return g.state.Load() == StateCritical }

func (g *MemoryGovernor) AcquireHeavy(ctx context.Context) (release func(), ok bool) {
	if g.state.Load() == StateCritical {
		return func() {}, false
	}
	wait := 5 * time.Second
	if g.state.Load() == StateHigh {
		wait = 1500 * time.Millisecond
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case g.heavy <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-g.heavy }) }, true
	case <-timer.C:
		return func() {}, false
	case <-ctx.Done():
		return func() {}, false
	}
}

func (g *MemoryGovernor) Snapshot() map[string]any {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	used := g.used.Load()
	if used == 0 {
		used = ms.HeapInuse
	}
	names := map[int32]string{StateOK: "ok", StateHigh: "high", StateCritical: "critical"}
	return map[string]any{
		"state":           names[g.state.Load()],
		"limit_bytes":     g.limit,
		"used_bytes":      used,
		"used_percent":    round2(float64(used) / float64(g.limit) * 100),
		"heap_alloc":      ms.HeapAlloc,
		"sys":             ms.Sys,
		"num_gc":          ms.NumGC,
		"forced_releases": g.gcRuns.Load(),
		"goroutines":      runtime.NumGoroutine(),
		"heavy_slots":     cap(g.heavy),
		"heavy_inuse":     len(g.heavy),
	}
}

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

func human(b uint64) string {
	const u = 1024
	if b < u {
		return strconv.FormatUint(b, 10) + "B"
	}
	div, exp := uint64(u), 0
	for n := b / u; n >= u; n /= u {
		div *= u
		exp++
	}
	return strconv.FormatFloat(float64(b)/float64(div), 'f', 1, 64) + string("KMGT"[exp]) + "iB"
}
