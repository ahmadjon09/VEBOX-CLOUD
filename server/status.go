package main

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type StatusTracker struct {
	mu            sync.Mutex
	startedAt     time.Time
	lastProbe     time.Time
	lastUp        time.Time
	totalUp       int64
	degradedSince time.Time

	app *App
}

func NewStatusTracker(app *App) *StatusTracker {
	now := time.Now()
	return &StatusTracker{startedAt: now, lastProbe: now, lastUp: now, app: app}
}

func (t *StatusTracker) tick(ctx context.Context) {
	t.mu.Lock()
	now := time.Now()
	elapsed := int64(now.Sub(t.lastProbe).Seconds())
	t.lastProbe = now

	up := !t.app.gov.Busy()
	upElapsed := int64(0)
	if up {
		t.totalUp += elapsed
		t.lastUp = now
		upElapsed = elapsed
	}
	day := now.UTC().Format("2006-01-02")
	wasDegraded := !t.degradedSince.IsZero()
	isDegraded := !up
	t.mu.Unlock()

	_ = t.app.meta.UpsertStatusDay(ctx, day, elapsed, upElapsed)

	t.mu.Lock()
	if isDegraded && !wasDegraded {
		t.degradedSince = now
		inc := &StatusIncident{
			ID:        newID("inc"),
			Component: "core",
			Summary:   "Server yuk ostida — bir qancha so'rovlar sekinroq ishlaydi",
			StartedAt: now,
		}
		go func() {
			c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = t.app.meta.AddIncident(c, inc)
		}()
	}
	if !isDegraded && wasDegraded {
		end := now
		inc := &StatusIncident{
			ID:        newID("incend"),
			Component: "core",
			Summary:   "Holat tiklandi — barcha tizimlar ishlayapti",
			StartedAt: t.degradedSince,
			EndedAt:   &end,
		}
		go func() {
			c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = t.app.meta.AddIncident(c, inc)
		}()
		t.degradedSince = time.Time{}
	}
	t.mu.Unlock()
}

func (t *StatusTracker) startSampler(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c, cancel := context.WithTimeout(ctx, 20*time.Second)
			t.tick(c)
			cancel()
		}
	}
}

func (t *StatusTracker) uptime() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return time.Since(t.startedAt)
}

func (t *StatusTracker) startedAtTime() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.startedAt
}

func (app *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	now := time.Now()

	status := "operational"
	if app.gov.Busy() {
		status = "degraded"
	}

	hist, _ := app.meta.StatusHistory(r.Context(), now.AddDate(0, 0, -90))
	if hist == nil {
		hist = []StatusDay{}
	}
	incidents, _ := app.meta.RecentIncidents(r.Context(), 20)
	if incidents == nil {
		incidents = []StatusIncident{}
	}

	var sum, n float64
	for _, d := range hist {
		if d.Total > 0 {
			sum += d.UptimePct
			n++
		}
	}
	avg := 100.0
	if n > 0 {
		avg = sum / n
		avg = float64(int(avg*100)) / 100
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"status": status,
		"uptime": map[string]any{
			"seconds":    int(app.status.uptime().Seconds()),
			"started_at": app.status.startedAtTime().UTC().Format(time.RFC3339),
		},
		"uptime_90d": avg,
		"version":    Version,
		"storage":    storageMode(app.vault),
		"database":   databaseMode(app.core, app.meta),
		"history":    hist,
		"incidents":  incidents,
		"updated_at": now.UTC().Format(time.RFC3339),
	})
}

func databaseMode(core CoreStore, meta MetaStore) string {
	switch {
	case core == nil && meta == nil:
		return "memory"
	default:
		return "connected"
	}
}
