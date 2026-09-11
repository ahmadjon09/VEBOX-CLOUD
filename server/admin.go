package main

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/go-chi/chi/v5"
)

func (app *App) adminListUsers(w http.ResponseWriter, r *http.Request) {
	limit := clampInt(atoiDef(r.URL.Query().Get("limit"), 50), 1, 200)
	offset := clampInt(atoiDef(r.URL.Query().Get("offset"), 0), 0, 1<<30)
	users, err := app.core.ListUsers(r.Context(), limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	type row struct {
		User         User          `json:"user"`
		Subscription *Subscription `json:"subscription"`
		Policy       TierPolicy    `json:"effective_policy"`
		Usage        *UsageCounter `json:"usage"`
	}
	out := make([]row, 0, len(users))
	for i := range users {
		s, _ := app.core.GetSubscription(r.Context(), users[i].ID)
		u, _ := app.core.GetUsage(r.Context(), users[i].ID)
		out = append(out, row{User: users[i], Subscription: s, Policy: EffectivePolicy(s), Usage: u})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": out, "count": len(out)})
}

func (app *App) adminSetSubscription(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if _, err := app.core.GetUser(r.Context(), userID); err != nil {
		writeErr(w, http.StatusNotFound, "user_not_found", "Foydalanuvchi topilmadi")
		return
	}
	var body struct {
		Tier               Tier           `json:"tier"`
		ExpiresAt          *time.Time     `json:"expires_at"`
		CustomRatePerMin   *int           `json:"custom_rate_per_min"`
		CustomBurst        *int           `json:"custom_burst"`
		CustomMaxUploadKB  *int64         `json:"custom_max_upload_kb"`
		CustomMaxAudioKB   *int64         `json:"custom_max_audio_kb"`
		CustomStorageMB    *int64         `json:"custom_storage_mb"`
		CustomMonthlyFiles *int64         `json:"custom_monthly_files"`
		StorageRules       map[string]any `json:"storage_rules"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Tier == "" || body.Tier == TierPro {
		body.Tier = TierFree
	}
	if body.Tier != TierFree && body.Tier != TierBusiness {
		writeErr(w, http.StatusBadRequest, "invalid_tier", "tier: free | business")
		return
	}
	if body.CustomMaxUploadKB != nil && *body.CustomMaxUploadKB > MaxImageSizeKB {
		v := MaxImageSizeKB
		body.CustomMaxUploadKB = &v
	}
	if body.CustomMaxAudioKB != nil && *body.CustomMaxAudioKB > MaxAudioSizeKB {
		v := MaxAudioSizeKB
		body.CustomMaxAudioKB = &v
	}
	sub := &Subscription{
		UserID: userID, Tier: body.Tier, ExpiresAt: body.ExpiresAt,
		CustomRatePerMin: body.CustomRatePerMin, CustomBurst: body.CustomBurst,
		CustomMaxUploadKB: body.CustomMaxUploadKB, CustomMaxAudioKB: body.CustomMaxAudioKB,
		CustomStorageMB: body.CustomStorageMB, CustomMonthlyFiles: body.CustomMonthlyFiles,
		StorageRules: body.StorageRules,
	}
	sub.UpdatedAt = time.Now().UTC()
	if err := app.core.SaveSubscription(r.Context(), sub); err != nil {
		writeErr(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	app.auth.InvalidateKeyCache()
	app.auth.PurgeSub(userID)
	if app.redis.On() {
		app.redis.Del("sub:" + userID)
	}

	warn := ""
	if body.Tier != TierBusiness && (body.CustomRatePerMin != nil || body.CustomStorageMB != nil) {
		warn = "Custom limitlar odatda faqat Business tarif uchun ishlatiladi"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "data": sub, "effective_policy": EffectivePolicy(sub), "warning": warn,
	})
}

func (app *App) adminBanUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Banned bool `json:"banned"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	target := chi.URLParam(r, "id")
	if body.Banned && target == UserFrom(r.Context()).ID {
		writeErr(w, http.StatusBadRequest, "self_ban_not_allowed",
			"O'zingizni bloklay olmaydasiz — avval boshqa admin yaratib qo'ying")
		return
	}
	if err := app.core.SetBanned(r.Context(), target, body.Banned); err != nil {
		writeErr(w, http.StatusNotFound, "user_not_found", "Foydalanuvchi topilmadi")
		return
	}
	app.auth.InvalidateKeyCache()
	app.auth.PurgeUser(target)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "banned": body.Banned})
}

func (app *App) adminSetAdmin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Admin bool `json:"admin"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	id := chi.URLParam(r, "id")
	if err := app.core.SetAdmin(r.Context(), id, body.Admin); err != nil {
		writeErr(w, http.StatusNotFound, "user_not_found", "Foydalanuvchi topilmadi")
		return
	}
	app.auth.PurgeUser(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "admin": body.Admin})
}

func (app *App) adminSystem(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"uptime":          time.Since(app.startedAt).Round(time.Second).String(),
		"uptime_seconds":  int64(time.Since(app.startedAt).Seconds()),
		"started_at":      app.startedAt.UTC().Format(time.RFC3339),
		"version":         Version,
		"system":          app.systemInfo(),
		"memory":          app.gov.Snapshot(),
		"redis":           app.redis.Stats(),
		"jobs":            app.storage.QueueStats(),
		"storage_backend": storageBackend(app.vault),
		"databases":       app.dbStats(r.Context()),
		"tiers":           DefaultPolicies,
		"config":          app.publicConfig(),
	})
}

func (app *App) systemInfo() map[string]any {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	host, _ := os.Hostname()
	out := map[string]any{
		"go_version":    runtime.Version(),
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"num_cpu":       runtime.NumCPU(),
		"num_goroutine": runtime.NumGoroutine(),
		"pid":           os.Getpid(),
		"hostname":      host,
		"server_time":   time.Now().UTC().Format(time.RFC3339),
		"heap_alloc":    ms.HeapAlloc,
		"heap_sys":      ms.HeapSys,
		"gc_pause_ms":   round2(float64(ms.PauseTotalNs) / 1e6),
		"num_gc":        ms.NumGC,
	}
	if total, free, avail, ok := diskStats("/"); ok && total > 0 {
		out["disk"] = map[string]any{
			"total_bytes":  total,
			"free_bytes":   free,
			"avail_bytes":  avail,
			"used_bytes":   total - free,
			"used_percent": round2(float64(total-free) / float64(total) * 100),
		}
	}
	return out
}

func (app *App) publicConfig() map[string]any {
	return map[string]any{
		"public_url":         app.cfg.PublicURL,
		"web_url":            app.cfg.WebURL,
		"allow_audio":        app.cfg.AllowAudio,
		"max_audio_mb":       app.cfg.MaxAudioMB,
		"hd_max_width":       app.cfg.HDMaxWidth,
		"hd_quality":         app.cfg.HDQuality,
		"default_visibility": app.cfg.DefaultVisible,
		"hotlink_protect":    app.cfg.HotlinkProtect,
		"public_max_age":     app.cfg.PublicMaxAge,
		"rate_public":        app.cfg.PublicRatePerMin,
		"keepalive":          app.cfg.KeepAliveEnabled,
		"demo_mode":          app.cfg.DemoMode,
		"cors_origins":       len(app.cfg.CORSOrigins),
		"redis_enabled":      app.redis.On(),
	}
}

const dbStatsTTL = 15 * time.Second

type dbStatsEntry struct {
	data map[string]any
	exp  time.Time
}

func (app *App) dbStats(ctx context.Context) map[string]any {
	now := time.Now()
	if cached := app.dbStatsCache.Load(); cached != nil {
		if e, ok := cached.(*dbStatsEntry); ok && now.Before(e.exp) {
			return e.data
		}
	}
	c, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	out := map[string]any{}
	if st, err := app.core.SystemStats(c); err == nil {
		out["postgres"] = withQuota(st)
	} else {
		out["postgres"] = map[string]any{"connected": false, "error": err.Error()}
	}
	if st, err := app.meta.SystemStats(c); err == nil {
		out["mongo"] = withQuota(st)
	} else {
		out["mongo"] = map[string]any{"connected": false, "error": err.Error()}
	}
	app.dbStatsCache.Store(&dbStatsEntry{data: out, exp: now.Add(dbStatsTTL)})
	return out
}

func withQuota(st map[string]any) map[string]any {
	used, _ := st["size_bytes"].(int64)
	quota := envInt64("DB_QUOTA_BYTES", 0)
	if quota > 0 && used > 0 {
		st["quota_bytes"] = quota
		st["free_bytes"] = max64(0, quota-used)
		st["used_percent"] = round2(float64(used) / float64(quota) * 100)
	}
	return st
}

func storageMode(v *VaultClient) string {
	if v.DemoMode() {
		return "local-ephemeral"
	}
	return "distributed-vault"
}

func storageBackend(v *VaultClient) map[string]any {
	mode := storageMode(v)
	nodes := v.PoolStats()
	var up, cooling, okCnt, failCnt, inflight int64
	for _, n := range nodes {
		if av, _ := n["available"].(bool); av {
			up++
		} else {
			cooling++
		}
		if o, _ := n["ok"].(int64); o > 0 {
			okCnt += o
		}
		if f, _ := n["failed"].(int64); f > 0 {
			failCnt += f
		}
		if i, _ := n["inflight"].(int64); i > 0 {
			inflight += i
		}
	}
	return map[string]any{
		"mode":  mode,
		"nodes": nodes,
		"summary": map[string]any{
			"nodes":      len(nodes),
			"available":  up,
			"cooling":    cooling,
			"ok":         okCnt,
			"failed":     failCnt,
			"inflight":   inflight,
			"link_cache": v.LinkCacheSize(),
		},
	}
}

func (app *App) adminAnalytics(w http.ResponseWriter, r *http.Request) {
	days := clampInt(atoiDef(r.URL.Query().Get("days"), 7), 1, 365)
	since := time.Now().AddDate(0, 0, -days)
	res, err := app.meta.Analytics(r.Context(), r.URL.Query().Get("user_id"), since)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "analytics_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "since": since, "data": res})
}

func (app *App) adminInvalidateCache(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	switch kind {
	case "auth":
		app.auth.InvalidateKeyCache()
	case "media":
		app.storage.PurgeLocalCaches()
	case "redis":
		app.redis.FlushNamespace()
	default:
		app.auth.InvalidateKeyCache()
		app.storage.PurgeLocalCaches()
		app.redis.FlushNamespace()
	}
	app.dbStatsCache.Store((*dbStatsEntry)(nil))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cleared": kind})
}
