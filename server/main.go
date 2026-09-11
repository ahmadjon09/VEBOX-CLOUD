package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"golang.org/x/crypto/bcrypt"
)

type App struct {
	cfg       *Config
	core      CoreStore
	meta      MetaStore
	vault     *VaultClient
	gov       *MemoryGovernor
	redis     *RedisCache
	auth      *Auth
	storage   *StorageService
	limiter   *RateLimiter
	status    *StatusTracker
	startedAt time.Time

	dbStatsCache atomic.Value
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[vebox] ")

	cfg := LoadConfig()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := NewApp(ctx, cfg)
	if err != nil {
		log.Fatalf("ishga tushirib bo'lmadi: %v", err)
	}
	defer app.Close()

	go app.status.startSampler(ctx)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           app.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		log.Printf("HTTP server ishga tushdi: http://0.0.0.0:%s", cfg.Port)
		log.Printf("Dokumentatsiya: %s/docs", cfg.PublicURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server xatosi: %v", err)
		}
	}()

	if cfg.KeepAliveEnabled {
		go app.keepAlive(ctx)
	}

	<-ctx.Done()
	log.Println("to'xtatilmoqda… (graceful shutdown)")
	sh, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(sh); err != nil {
		log.Printf("shutdown xatosi: %v", err)
	}
	log.Println("to'xtatildi")
}

func NewApp(ctx context.Context, cfg *Config) (*App, error) {
	app := &App{cfg: cfg, startedAt: time.Now(), limiter: NewRateLimiter()}
	setPaletteSize(cfg.PaletteCount)

	app.gov = NewMemoryGovernor(cfg.MemLimitBytes, cfg.MemHighWater, cfg.MemCritical,
		cfg.MemTick, cfg.MaxHeavyJobs)
	app.gov.Start(ctx)
	log.Printf("Memory Governor yoqildi (limit≈%s, high=%.0f%%, critical=%.0f%%)",
		human(app.gov.limit), cfg.MemHighWater*100, cfg.MemCritical*100)

	if cfg.PostgresDSN != "" && isDSNPostgres(cfg.PostgresDSN) {
		pg, err := NewPostgresStore(ctx, cfg.PostgresDSN)
		if err != nil {
			if cfg.StrictDB {
				return nil, fmt.Errorf("PostgreSQL ulanmadi (STRICT_DB=1): %w", err)
			}
			log.Printf("PostgreSQL ulanmadi (%v) — in-memory fallback ishlatiladi", err)
		} else {
			app.core = pg
			log.Println("PostgreSQL ulandi va migratsiya bajarildi")
		}
	}

	if cfg.MongoURI != "" {
		mg, err := NewMongoStore(ctx, cfg.MongoURI, cfg.MongoDB)
		if err != nil {
			if cfg.StrictDB {
				return nil, fmt.Errorf("MongoDB ulanmadi (STRICT_DB=1): %w", err)
			}
			log.Printf("MongoDB ulanmadi (%v) — in-memory fallback ishlatiladi", err)
		} else {
			app.meta = mg
			log.Println("MongoDB ulandi")
		}
	}

	if app.core == nil || app.meta == nil {
		if cfg.StrictDB && !cfg.DemoMode {
			return nil, errors.New("STRICT_DB=1: POSTGRES_DSN va MONGO_URI ikkalasi ham kerak")
		}
		mem := NewMemStore()
		if app.core == nil {
			app.core = mem
		}
		if app.meta == nil {
			app.meta = mem
		}
		log.Println("DIQQAT: in-memory rejim faol — ma'lumotlar qayta ishga tushirishda yo'qoladi")
	}

	app.vault = NewVaultClient(cfg.VaultTokens, cfg.VaultChatID,
		cfg.FloodCooldown, cfg.VaultTimeout, cfg.FileLinkTTL)
	if app.vault.DemoMode() {
		log.Println("Saqlash: lokal efemer rejim (production uchun VAULT_* sozlang)")
	} else {
		log.Printf("Saqlash: taqsimlangan vault, %d ta tugun faol", len(cfg.VaultTokens))
	}

	app.redis = NewRedisCache(ctx, cfg.Redis())
	if app.redis.On() {
		log.Printf("Redis L2 kesh yoqildi (%s, budget %s)", app.redis.Stats()["addr"], humanBytes(int64(cfg.RedisMaxMB)*1024*1024))
	} else if cfg.RedisURL != "" {
		log.Printf("Redis sozlangan (%s), lekin ulanmadi: %s — fon rejimida har %s da qayta uriniladi, hozircha faqat ichki kesh ishlatiladi",
			app.redis.Stats()["addr"], app.redis.LastError(), redisRetryEvery)
	}

	app.auth = NewAuth(cfg, app.core)
	app.storage = NewStorageService(cfg, app.core, app.meta, app.vault, app.gov, app.redis)
	app.storage.StartWorkers(ctx)
	app.status = NewStatusTracker(app)

	if cfg.DemoMode {
		app.seedDemoUser(ctx)
	}
	return app, nil
}

func (app *App) seedDemoUser(ctx context.Context) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("demo1234"), bcrypt.DefaultCost)
	u, err := app.core.UpsertOAuthUser(ctx, &User{
		ID: newID("usr"), Email: "demo@vebox.local", Name: "Demo User",
		Provider: "email", ProviderID: "demo-000", IsAdmin: true,
		PasswordHash: string(hash),
	})
	if err != nil {
		return
	}
	_, _ = app.core.UpsertOAuthUser(ctx, &User{
		ID: newID("usr"), Email: "member@vebox.local", Name: "Demo Member",
		Provider: "github", ProviderID: "demo-member-001", IsAdmin: false,
	})

	plain, _, err := app.auth.NewAPIKey(ctx, u.ID, "demo")
	if err != nil {
		return
	}
	tok, _ := app.auth.IssueJWT(u)
	log.Println("──────────── DEMO REJIM ────────────")
	log.Printf("Login     : demo@vebox.local / demo1234")
	log.Printf("API Key   : %s", plain)
	log.Printf("Admin JWT : %s", tok)
	log.Println("────────────────────────────────────")
}

func (app *App) Close() {
	if app.storage != nil {
		app.storage.Close()
	}
	if app.core != nil {
		_ = app.core.Close()
	}
	if app.meta != nil {
		if c, ok := app.meta.(interface{ Close() error }); ok && !sameStore(app.core, app.meta) {
			_ = c.Close()
		}
	}
}

func sameStore(a CoreStore, b MetaStore) bool {
	am, ok1 := a.(*MemStore)
	bm, ok2 := b.(*MemStore)
	return ok1 && ok2 && am == bm
}

func (app *App) keepAlive(ctx context.Context) {
	interval := app.cfg.KeepAliveInterval
	if interval < time.Minute {
		interval = 10 * time.Minute
	}
	client := &http.Client{Timeout: 8 * time.Second}
	target := "http://127.0.0.1:" + app.cfg.Port + "/healthz"
	log.Printf("Keep-alive faol: %s har %s da", target, interval)

	probe := func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		if resp.StatusCode == http.StatusOK {
			log.Printf("[keepalive] OK 200 — tizim jonli")
			return nil
		}
		log.Printf("[keepalive] DIQQAT: /healthz -> %d (biznes xato)", resp.StatusCode)
		return fmt.Errorf("healthz %d", resp.StatusCode)
	}

	for attempt := 0; attempt < 12; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
		if err := probe(); err == nil {
			break
		} else if attempt == 0 {
			log.Printf("[keepalive] birinchi so'rov hali erkin (server ishga tushmoqda): %v", err)
		}
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			probe()
		}
	}
}

func (app *App) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5, "application/json", "text/html", "text/plain", "application/javascript"))
	r.Use(middleware.Timeout(app.cfg.RequestTimeout()))
	r.Use(app.securityHeaders)
	if len(app.cfg.CORSOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins: app.cfg.CORSOrigins,
			AllowedMethods: []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", "X-API-Key", "Range"},
			ExposedHeaders: []string{"X-Image-Id", "X-Image-Size-Bytes", "X-Image-Size-KB",
				"X-Delivery", "X-Image-Missing", "X-RateLimit-Limit", "X-RateLimit-Remaining",
				"ETag", "Accept-Ranges", "Content-Range",
				"X-Max-Upload-Bytes"},
			AllowCredentials: true,
			MaxAge:           3600,
		}))
	}

	r.Get("/openapi.json", app.handleOpenAPI)
	r.Get("/healthz", app.handleHealth)

	r.Route("/auth", func(r chi.Router) {
		r.Get("/session", app.auth.SessionHandler)
		r.Post("/logout", app.auth.LogoutHandler)
		r.Get("/github/login", func(w http.ResponseWriter, r *http.Request) {
			app.auth.LoginHandler(w, r, "github")
		})
		r.Get("/github/callback", func(w http.ResponseWriter, r *http.Request) {
			app.auth.CallbackHandler(w, r, "github")
		})
	})

	r.Group(func(r chi.Router) {
		r.Use(app.auth.OptionalAPIKey)
		r.Use(app.publicRateLimit)
		r.Get("/s/{id}", app.handleSigned)
		r.Group(func(r chi.Router) {
			r.Use(app.hotlinkGuard)
			r.Get("/cdn/{id}", app.handleCDN)
			r.Get("/preview/{id}", app.handlePreview)
			r.Head("/cdn/{id}", app.handleCDN)
			r.Head("/preview/{id}", app.handlePreview)
		})
		r.Group(func(r chi.Router) {
			r.Use(app.requireIdentified)
			r.Get("/i/{id}", app.handleStream)
			r.Get("/d/{id}", app.handleDownload)
			r.Head("/i/{id}", app.handleStream)
			r.Head("/d/{id}", app.handleDownload)
		})
	})

	r.Route("/v1", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/signup", app.auth.SignupHandler)
			r.Post("/login", app.auth.LoginEmailHandler)
			r.Post("/change-password", app.auth.ChangePasswordHandler)
		})
		r.Get("/status", app.handleStatus)

		r.Group(func(r chi.Router) {
			r.Use(app.auth.RequireAuth)
			r.Use(app.rateLimit)
			r.Post("/images", app.handleUpload)
			r.Get("/images", app.handleList)
			r.Delete("/images", app.handleBulkDelete)
			r.Patch("/images", app.handleBulkVisibility)
			r.Get("/images/details", app.handleDetails)
			r.Get("/images/{id}", app.handleImageMeta)
			r.Patch("/images/{id}", app.handleUpdateImage)
			r.Delete("/images/{id}", app.handleDelete)
			r.Post("/images/{id}/share", app.handleShare)
			r.Get("/me", app.handleMe)
			r.Get("/analytics", app.handleAnalytics)
		})

		r.Group(func(r chi.Router) {
			r.Use(app.auth.RequireAuth)
			r.Use(app.rateLimit)
			r.Get("/keys", app.handleListKeys)
			r.Post("/keys", app.handleCreateKey)
			r.Delete("/keys/{id}", app.handleDeleteKey)
			r.Delete("/keys", app.handleDeleteAllKeys)
			r.Get("/profile", func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": UserFrom(r.Context())})
			})
			r.Patch("/profile", app.handleUpdateProfile)
			r.Post("/profile/avatar", app.handleAvatarUpload)
			r.Get("/settings", app.handleGetSettings)
			r.Patch("/settings", app.handleUpdateSettings)
		})

		r.Route("/admin", func(r chi.Router) {
			r.Use(app.auth.RequireAuth)
			r.Use(app.auth.RequireAdmin)
			r.Get("/users", app.adminListUsers)
			r.Put("/users/{id}/subscription", app.adminSetSubscription)
			r.Post("/users/{id}/ban", app.adminBanUser)
			r.Post("/users/{id}/admin", app.adminSetAdmin)
			r.Get("/system", app.adminSystem)
			r.Get("/analytics", app.adminAnalytics)
			r.Post("/cache/invalidate", app.adminInvalidateCache)
		})
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if isMediaPath(r.URL.Path) {
			app.missingMedia(w, r, "Media topilmadi")
			return
		}
		writeErr(w, http.StatusNotFound, "not_found", "Bunday manzil mavjud emas. /docs ga qarang")
	})
	return r
}

func isMediaPath(p string) bool {
	for _, pre := range []string{"/i/", "/cdn/", "/preview/", "/d/", "/s/"} {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	return false
}

func (app *App) securityHeaders(next http.Handler) http.Handler {
	hsts := ""
	if app.cfg.IsHTTPS() {
		hsts = "max-age=15552000; includeSubDomains"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		if hsts != "" {
			w.Header().Set("Strict-Transport-Security", hsts)
		}
		next.ServeHTTP(w, r)
	})
}

func (app *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	code := http.StatusOK
	if app.gov.Busy() {
		status, code = "degraded", http.StatusServiceUnavailable
	}
	w.Header().Set("Cache-Control", "no-store")
	out := map[string]any{"status": status, "time": time.Now().UTC()}
	if u := app.auth.SessionUser(r); u != nil && u.IsAdmin {
		out["uptime"] = time.Since(app.startedAt).Round(time.Second).String()
		out["memory"] = app.gov.Snapshot()
		out["storage"] = storageMode(app.vault)
	}
	writeJSON(w, code, out)
}

func (app *App) handleMe(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	pol := PolicyFrom(r.Context())
	usage, _ := app.storage.UsageCached(r.Context(), u.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]any{
			"user":    u,
			"account": TierSnapshot(pol, usage),
		},
	})
}

func (app *App) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	pol := PolicyFrom(r.Context())
	u := UserFrom(r.Context())
	days := clampInt(atoiDef(r.URL.Query().Get("days"), 7), 1, 90)
	res, err := app.meta.Analytics(r.Context(), u.ID, time.Now().AddDate(0, 0, -days))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "analytics_failed", "Analitikani olib bo'lmadi")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "days": days, "pro": pol.Analytics, "data": res,
	})
}

func (app *App) handleListKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := app.core.ListAPIKeys(r.Context(), UserFrom(r.Context()).ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_failed", "Kalitlarni olib bo'lmadi")
		return
	}
	if keys == nil {
		keys = []APIKey{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": keys})
}

func (app *App) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	plain, rec, err := app.auth.NewAPIKey(r.Context(), UserFrom(r.Context()).ID, body.Name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create_failed", "Kalit yaratib bo'lmadi")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"ok": true, "data": rec, "api_key": plain,
		"note": "Bu kalit boshqa ko'rsatilmaydi — xavfsiz joyda saqlang",
	})
}

func (app *App) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	if err := app.core.DeleteAPIKey(r.Context(), UserFrom(r.Context()).ID, chi.URLParam(r, "id")); err != nil {
		writeErr(w, http.StatusNotFound, "key_not_found", "Kalit topilmadi")
		return
	}
	app.auth.InvalidateKeyCache()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Kalit butunlay o'chirildi"})
}

func (app *App) handleDeleteAllKeys(w http.ResponseWriter, r *http.Request) {
	if err := app.core.DeleteAllAPIKeys(r.Context(), UserFrom(r.Context()).ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "delete_failed", "Kalitlarni o'chirib bo'lmadi")
		return
	}
	app.auth.InvalidateKeyCache()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Barcha kalitlar o'chirildi"})
}

func (app *App) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	var body struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	name = clipRunes(name, 80)
	fresh, err := app.core.UpdateProfile(r.Context(), u.ID, name, u.AvatarURL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "update_failed", "Profil yangilanmadi")
		return
	}
	app.auth.PurgeUser(u.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": fresh})
}

func (app *App) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	pol := PolicyFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]any{
			"hd_processing": pol.HDProcessing,
			"max_hd_width":  pol.MaxHDWidth,
		},
	})
}

func (app *App) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	var body struct {
		HDProcessing *bool `json:"hd_processing"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	sub, err := app.core.GetSubscription(r.Context(), u.ID)
	if err != nil || sub == nil {
		sub = &Subscription{UserID: u.ID, Tier: TierFree}
	}
	if sub.StorageRules == nil {
		sub.StorageRules = map[string]any{}
	}
	if body.HDProcessing != nil {
		sub.StorageRules["hd_processing"] = *body.HDProcessing
	}
	sub.UpdatedAt = time.Now().UTC()
	if err := app.core.SaveSubscription(r.Context(), sub); err != nil {
		writeErr(w, http.StatusInternalServerError, "save_failed", "Sozlamalar saqlanmadi")
		return
	}
	app.auth.PurgeSub(u.ID)
	pol := EffectivePolicy(sub)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]any{
			"hd_processing": pol.HDProcessing,
			"max_hd_width":  pol.MaxHDWidth,
		},
	})
}

func (app *App) handleAvatarUpload(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	const maxAvatar = 5 << 20
	if cl := r.ContentLength; cl > maxAvatar+uploadOverheadBytes {
		writeErr(w, http.StatusRequestEntityTooLarge, "file_too_large", "Avatar 5 MB'dan katta bo'lmasin")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAvatar+uploadOverheadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_multipart", "Form o'qib bo'lmadi")
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	f, fh, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing_file", "`file` maydoni topilmadi")
		return
	}
	defer f.Close()
	if fh.Size > maxAvatar {
		writeErr(w, http.StatusRequestEntityTooLarge, "file_too_large", "Avatar 5 MB'dan katta bo'lmasin")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxAvatar+1))
	if err != nil || int64(len(raw)) > maxAvatar {
		writeErr(w, http.StatusRequestEntityTooLarge, "file_too_large", "Avatar 5 MB'dan katta bo'lmasin")
		return
	}
	info, derr := DetectMedia(raw)
	if derr != nil || !IsSupported(info) || info.Kind != KindImage {
		writeErr(w, http.StatusUnsupportedMediaType, "unsupported_format",
			"Avatar uchun rasm formati kerak: "+SupportedFormatsText())
		return
	}
	res, uerr := app.storage.Upload(r.Context(), u, PolicyFrom(r.Context()), fh.Filename, raw, VisPublic)
	if uerr != nil {
		var qe *QuotaError
		if asQuota(uerr, &qe) {
			writeJSON(w, http.StatusPaymentRequired, map[string]any{
				"error": map[string]any{"code": qe.Code, "message": qe.Message}})
			return
		}
		writeErr(w, http.StatusBadGateway, "upload_failed", "Avatar yuklanmadi")
		return
	}
	old := u.AvatarURL
	fresh, perr := app.core.UpdateProfile(r.Context(), u.ID, u.Name, res.Image.URLs.CDN)
	if perr != nil {
		writeErr(w, http.StatusInternalServerError, "update_failed", "Avatar saqlanmadi")
		return
	}
	if id := mediaIDFromURL(old); id != "" {
		_ = app.storage.Delete(r.Context(), u.ID, id)
	}
	app.auth.PurgeUser(u.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": fresh})
}

func mediaIDFromURL(u string) string {
	for _, pre := range []string{"/cdn/", "/i/"} {
		if i := strings.Index(u, pre); i >= 0 {
			id := u[i+len(pre):]
			if j := strings.IndexAny(id, "?/"); j >= 0 {
				id = id[:j]
			}
			if validMediaID(id) {
				return id
			}
		}
	}
	return ""
}

func clipRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json", "JSON o'qib bo'lmadi: "+err.Error())
		return false
	}
	return true
}
