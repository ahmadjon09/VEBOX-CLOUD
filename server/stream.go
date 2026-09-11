package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

var streamBufPool = sync.Pool{
	New: func() any { b := make([]byte, 32*1024); return b },
}

type flushWriter struct {
	w io.Writer
	f http.Flusher
	n int
}

func newFlushWriter(w http.ResponseWriter) io.Writer {
	fw := &flushWriter{w: w}
	if f, ok := w.(http.Flusher); ok {
		fw.f = f
	}
	return fw
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	fw.n += n
	if fw.f != nil && fw.n >= 128*1024 {
		fw.f.Flush()
		fw.n = 0
	}
	return n, err
}

func (app *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	pol := PolicyFrom(r.Context())

	if app.gov.Busy() {
		w.Header().Set("Retry-After", "5")
		writeErr(w, http.StatusServiceUnavailable, "server_busy",
			"Server vaqtincha band. Bir necha soniyadan so'ng qayta urinib ko'ring")
		return
	}

	maxBytes := app.maxUploadBytes(pol)
	bodyLimit := maxBytes + uploadOverheadBytes

	if cl := r.ContentLength; cl > bodyLimit {
		w.Header().Set("X-Max-Upload-Bytes", strconv.FormatInt(maxBytes, 10))
		writeErr(w, http.StatusRequestEntityTooLarge, "file_too_large",
			"Fayl hajmi limitdan katta ("+app.uploadLimitsText(pol)+")")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, bodyLimit)

	var (
		raw      []byte
		filename string
		err      error
	)
	ct := r.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(ct, "multipart/form-data"):
		if err = r.ParseMultipartForm(8 << 20); err != nil {
			if isTooLarge(err) {
				writeUploadTooLarge(w, app, pol)
				return
			}
			if r.Context().Err() != nil {
				return
			}
			writeErr(w, http.StatusBadRequest, "invalid_multipart", "Form o'qib bo'lmadi: "+err.Error())
			return
		}
		defer func() { _ = r.MultipartForm.RemoveAll() }()
		f, fh, ferr := r.FormFile("file")
		if ferr != nil {
			writeErr(w, http.StatusBadRequest, "missing_file", "`file` maydoni topilmadi")
			return
		}
		defer f.Close()
		filename = fh.Filename
		if fh.Size > maxBytes {
			writeUploadTooLarge(w, app, pol)
			return
		}
		raw, err = io.ReadAll(io.LimitReader(f, maxBytes+1))
		if err != nil {
			if isTooLarge(err) {
				writeUploadTooLarge(w, app, pol)
				return
			}
			writeErr(w, http.StatusBadRequest, "read_failed", "Faylni o'qib bo'lmadi")
			return
		}
	default:
		filename = r.URL.Query().Get("filename")
		raw, err = io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
		if err != nil {
			if isTooLarge(err) {
				writeUploadTooLarge(w, app, pol)
				return
			}
			writeErr(w, http.StatusBadRequest, "read_failed", "Tanani o'qib bo'lmadi")
			return
		}
	}

	if int64(len(raw)) > maxBytes {
		writeUploadTooLarge(w, app, pol)
		return
	}
	if len(raw) == 0 {
		writeErr(w, http.StatusBadRequest, "empty_file", "Fayl bo'sh")
		return
	}

	visibility := strings.ToLower(firstParam(r, "visibility", "mode"))
	res, err := app.storage.Upload(r.Context(), user, pol, filename, raw, visibility)
	if err != nil {
		var qe *QuotaError
		switch {
		case asQuota(err, &qe):
			writeJSON(w, http.StatusPaymentRequired, map[string]any{
				"error": map[string]any{"code": qe.Code, "message": qe.Message,
					"limit": qe.Limit, "used": qe.Used},
			})
		case err == ErrServerBusy:
			w.Header().Set("Retry-After", "5")
			writeErr(w, http.StatusServiceUnavailable, "server_busy", err.Error())
		case errors.Is(err, ErrVideoNotSupported):
			writeErr(w, http.StatusUnsupportedMediaType, "unsupported_format",
				"Afsuski, video fayllar qo'llab-quvvatlanmaydi — rasm yoki audio (M4A/MP3) yuklang")
		case errors.Is(err, ErrAudioDisabled):
			writeErr(w, http.StatusUnsupportedMediaType, "audio_disabled",
				"Ushbu serverda audio yuklash o'chirilgan")
		case errors.Is(err, ErrUnsupportedFormat) || strings.Contains(err.Error(), "unsupported"):
			writeErr(w, http.StatusUnsupportedMediaType, "unsupported_format",
				"Qo'llab-quvvatlanadigan formatlar: "+SupportedFormatsText())
		case errors.Is(err, context.Canceled):
			return
		case errors.Is(err, ErrVaultUnavailable):
			w.Header().Set("Retry-After", "15")
			writeErr(w, http.StatusServiceUnavailable, "storage_unavailable",
				"Saqlash qatlami vaqtincha band. 10-20 soniyadan so'ng qayta urinib ko'ring")
		case errors.Is(err, context.DeadlineExceeded):
			w.Header().Set("Retry-After", "15")
			writeErr(w, http.StatusGatewayTimeout, "storage_timeout",
				"Saqlash vaqti tugadi. Kichikroq fayl bilan qayta urinib ko'ring")
		default:
			log.Printf("[upload] unexpected error (user=%s, %d bytes): %v", user.ID, len(raw), err)
			writeErr(w, http.StatusBadGateway, "upload_failed",
				"Yuklab bo'lmadi, qayta urinib ko'ring")
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "data": res})
}

const uploadOverheadBytes = 1 << 20

func isTooLarge(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

func writeUploadTooLarge(w http.ResponseWriter, app *App, pol TierPolicy) {
	w.Header().Set("X-Max-Upload-Bytes", strconv.FormatInt(app.maxUploadBytes(pol), 10))
	writeErr(w, http.StatusRequestEntityTooLarge, "file_too_large",
		"Fayl hajmi limitdan katta ("+app.uploadLimitsText(pol)+")")
}

func (app *App) maxUploadBytes(pol TierPolicy) int64 {
	max := int64(0)
	for _, k := range []MediaKind{KindImage, KindAudio} {
		if v := app.maxUploadBytesFor(pol, k); v > max {
			max = v
		}
	}
	if max <= 0 {
		max = MaxFileSizeBytes
	}
	return max
}

func (app *App) maxUploadBytesFor(pol TierPolicy, kind MediaKind) int64 {
	v := pol.MaxBytesFor(kind)
	if kind == KindAudio {
		if c := int64(app.cfg.MaxAudioMB) * 1024 * 1024; c > 0 && c < v {
			v = c
		}
	}
	if ceiling := kindCeilingKB(kind) * 1024; v <= 0 || v > ceiling {
		v = ceiling
	}
	return v
}

func (app *App) uploadLimitsText(pol TierPolicy) string {
	return "rasm: " + humanBytes(app.maxUploadBytesFor(pol, KindImage)) +
		", audio: " + humanBytes(app.maxUploadBytesFor(pol, KindAudio))
}

//go:embed static/404.svg
var missingMediaSVG string

func writeMissingImage(w http.ResponseWriter, r *http.Request) {
	status := http.StatusNotFound
	if r != nil && r.URL.Query().Get("missing") == "image" {
		status = http.StatusOK
	}
	writeMissingImageStatus(w, status)
}

func writeMissingImageStatus(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Image-Missing", "1")
	w.Header().Set("Retry-After", "10")
	w.Header().Set("X-Robots-Tag", "noindex")
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'; script-src 'none'; sandbox")
	w.Header().Set("Content-Length", strconv.Itoa(len(missingMediaSVG)))
	w.WriteHeader(status)
	_, _ = io.WriteString(w, missingMediaSVG)
}

func (app *App) missingMedia(w http.ResponseWriter, r *http.Request, msg string) {
	if strings.Contains(r.Header.Get("Accept"), "application/json") ||
		r.URL.Query().Get("missing") == "error" {
		writeErr(w, http.StatusNotFound, "not_found", msg)
		return
	}
	writeMissingImage(w, r)
}

func (app *App) loadForDelivery(w http.ResponseWriter, r *http.Request) (*ImageMeta, bool) {
	id := chi.URLParam(r, "id")
	if !validMediaID(id) {
		app.missingMedia(w, r, "Media topilmadi")
		return nil, false
	}
	m, err := app.storage.Get(r.Context(), id)
	if err != nil {
		app.missingMedia(w, r, "Media topilmadi")
		return nil, false
	}
	if !m.IsPublic() {
		user := UserFrom(r.Context())
		if user == nil || (user.ID != m.UserID && !user.IsAdmin) {
			app.missingMedia(w, r, "Media topilmadi")
			return nil, false
		}
	}
	return m, true
}

func (app *App) deliveryPolicy(r *http.Request, m *ImageMeta) TierPolicy {
	if u := UserFrom(r.Context()); u != nil && u.ID == m.UserID {
		return PolicyFrom(r.Context())
	}
	return publicPolicy()
}

func (app *App) deliverMedia(w http.ResponseWriter, r *http.Request, m *ImageMeta, variant string, width int) {
	var pol TierPolicy
	if width > 0 && !m.IsStreamed() {
		pol = app.deliveryPolicy(r, m)
	}
	app.storage.Deliver(w, r, m, variant, width, pol)
}

func (app *App) handleStream(w http.ResponseWriter, r *http.Request) {
	m, ok := app.loadForDelivery(w, r)
	if !ok {
		return
	}
	variant := r.URL.Query().Get("v")
	width := clampInt(atoiDef(r.URL.Query().Get("w"), 0), 0, 4096)
	app.deliverMedia(w, r, m, variant, width)
}

func (app *App) handleCDN(w http.ResponseWriter, r *http.Request) {
	m, ok := app.loadForDelivery(w, r)
	if !ok {
		return
	}
	width := clampInt(atoiDef(r.URL.Query().Get("w"), 0), 0, 4096)
	if m.Filename != "" {
		w.Header().Set("Content-Disposition", "inline; filename=\""+safeHeaderName(m.Filename)+"\"")
	}
	w.Header().Set("Timing-Allow-Origin", "*")
	w.Header().Set("X-Accel-Expires", strconv.Itoa(app.cfg.PublicMaxAge))
	app.deliverMedia(w, r, m, "hd", width)
}

func (app *App) handlePreview(w http.ResponseWriter, r *http.Request) {
	m, ok := app.loadForDelivery(w, r)
	if !ok {
		return
	}
	app.deliverMedia(w, r, m, "thumb", 0)
}

func (app *App) handleUpdateImage(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	var body struct {
		Visibility string `json:"visibility"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Visibility = strings.ToLower(strings.TrimSpace(body.Visibility))
	if !validVisibility(body.Visibility) {
		writeErr(w, http.StatusBadRequest, "invalid_visibility", "visibility: public | private")
		return
	}
	m, err := app.storage.SetVisibility(r.Context(), user.ID, chi.URLParam(r, "id"), body.Visibility)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "Rasm topilmadi")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": m.Public(app.cfg.LinkBase())})
}

func safeHeaderName(n string) string {
	var b strings.Builder
	for _, c := range n {
		if c < 32 || c == '"' || c == '\\' || c > 126 {
			continue
		}
		b.WriteRune(c)
	}
	if b.Len() == 0 {
		return "image"
	}
	return b.String()
}

func (app *App) handleList(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	limit := clampInt(atoiDef(r.URL.Query().Get("limit"), 30), 1, 100)
	offset := clampInt(atoiDef(r.URL.Query().Get("offset"), 0), 0, 1<<30)
	q := r.URL.Query()
	filter := ListFilter{
		Q:          strings.TrimSpace(q.Get("q")),
		Kind:       strings.ToLower(strings.TrimSpace(q.Get("kind"))),
		Visibility: strings.ToLower(strings.TrimSpace(q.Get("visibility"))),
		Sort:       strings.ToLower(strings.TrimSpace(q.Get("sort"))),
	}
	filter.Q = clipRunes(filter.Q, 200)
	items, total, err := app.storage.List(r.Context(), user.ID, limit, offset, filter)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_failed", "Ro'yxatni olib bo'lmadi")
		return
	}
	if items == nil {
		items = []MediaLink{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"data": items,
		"note": "Ro'yxat faqat havolalarni qaytaradi — fayl tafsilotlari: GET /v1/images/details?ids=... yoki GET /v1/images/{id}",
		"pagination": map[string]any{
			"total": total, "limit": limit, "offset": offset,
			"filter": map[string]any{
				"q": filter.Q, "kind": filter.Kind,
				"visibility": filter.Visibility, "sort": filter.Sort,
			},
		},
	})
}

func (app *App) handleDetails(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	ids := parseIDs(r, 100)
	if len(ids) == 0 {
		writeErr(w, http.StatusBadRequest, "missing_ids", "`ids` parametri bo'sh yoki noto'g'ri")
		return
	}
	items := app.storage.Details(r.Context(), user.ID, user.IsAdmin, ids)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": items})
}

func (app *App) handleImageMeta(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	m, err := app.storage.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil || (m.UserID != user.ID && !user.IsAdmin) {
		writeErr(w, http.StatusNotFound, "not_found", "Rasm topilmadi")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": m.Public(app.cfg.LinkBase())})
}

func (app *App) handleDelete(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	if err := app.storage.Delete(r.Context(), user.ID, chi.URLParam(r, "id")); err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "Rasm topilmadi")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "O'chirildi"})
}

func (app *App) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserFrom(r.Context())
		pol := PolicyFrom(r.Context())
		key := "anon:" + clientIP(r)
		if user != nil {
			key = "u:" + user.ID
		}
		ok, remaining, retry := app.limiter.Allow(key, pol.RatePerMin, pol.Burst)
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(pol.RatePerMin))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			writeErr(w, http.StatusTooManyRequests, "rate_limit_exceeded",
				"So'rovlar chegarasi oshib ketdi. Keyinroq urinib ko'ring")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (app *App) publicRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserFrom(r.Context()) != nil {
			app.rateLimit(next).ServeHTTP(w, r)
			return
		}
		key := "pub:" + clientIP(r)
		ok, remaining, retry := app.limiter.Allow(key, app.cfg.PublicRatePerMin, app.cfg.PublicBurst)
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(app.cfg.PublicRatePerMin))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			writeErr(w, http.StatusTooManyRequests, "rate_limit_exceeded",
				"So'rovlar chegarasi oshib ketdi. Keyinroq urinib ko'ring")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (app *App) requireIdentified(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserFrom(r.Context()) == nil {
			if wantJSON(r) {
				writeErr(w, http.StatusUnauthorized, "api_key_required",
					"Bu manzil faqat API kaliti yoki sessiya bilan ochiladi. Ochiq havola: /cdn/{id}")
				return
			}
			writeMissingImageStatus(w, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func wantJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") ||
		r.URL.Query().Get("missing") == "error"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{
		"ok": false,
		"error": map[string]any{
			"code": code, "message": msg, "status": status,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	})
}

func asQuota(err error, target **QuotaError) bool {
	if qe, ok := err.(*QuotaError); ok {
		*target = qe
		return true
	}
	return false
}

func firstParam(r *http.Request, names ...string) string {
	q := r.URL.Query()
	for _, n := range names {
		if v := q.Get(n); v != "" {
			return v
		}
	}
	if r.MultipartForm != nil {
		for _, n := range names {
			if vals := r.MultipartForm.Value[n]; len(vals) > 0 && vals[0] != "" {
				return vals[0]
			}
		}
	}
	if r.PostForm != nil {
		for _, n := range names {
			if v := r.PostForm.Get(n); v != "" {
				return v
			}
		}
	}
	return ""
}

func atoiDef(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("CF-Connecting-IP"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i > 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		host = host[:i]
	}
	return host
}

func parseIDs(r *http.Request, max int) []string {
	raw := r.URL.Query().Get("ids")
	if raw == "" && r.Body != nil && r.Method != http.MethodGet {
		var body struct {
			IDs []string `json:"ids"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body) == nil {
			raw = strings.Join(body.IDs, ",")
		}
	}
	ids := make([]string, 0, 16)
	for _, id := range strings.Split(raw, ",") {
		id = strings.TrimSpace(id)
		if !validMediaID(id) {
			continue
		}
		ids = append(ids, id)
		if max > 0 && len(ids) >= max {
			break
		}
	}
	return ids
}

func (app *App) handleDownload(w http.ResponseWriter, r *http.Request) {
	m, ok := app.loadForDelivery(w, r)
	if !ok {
		return
	}
	name := safeHeaderName(m.Filename)
	if name == "" || name == "image" {
		name = m.ID + "." + ExtForMime(m.MimeType)
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"; filename*=UTF-8''"+url.QueryEscape(name))
	w.Header().Set("Content-Type", m.MimeType)
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	app.deliverMedia(w, r, m, "hd", 0)
}

func (app *App) handleBulkDelete(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	ids := parseIDs(r, 100)
	if len(ids) == 0 {
		writeErr(w, http.StatusBadRequest, "missing_ids", "`ids` parametri bo'sh yoki noto'g'ri")
		return
	}
	deleted, failed := app.storage.BulkDelete(r.Context(), user.ID, ids)
	if deleted == nil {
		deleted = []string{}
	}
	if failed == nil {
		failed = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]any{
			"deleted": deleted, "failed": failed,
			"deleted_count": len(deleted), "failed_count": len(failed),
		},
	})
}

func (app *App) handleBulkVisibility(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	vis := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("visibility")))
	if !validVisibility(vis) {
		writeErr(w, http.StatusBadRequest, "invalid_visibility", "visibility: public | private")
		return
	}
	ids := parseIDs(r, 100)
	if len(ids) == 0 {
		writeErr(w, http.StatusBadRequest, "missing_ids", "`ids` parametri bo'sh")
		return
	}
	updated, failed := 0, 0
	for _, id := range ids {
		if _, err := app.storage.SetVisibility(r.Context(), user.ID, id, vis); err != nil {
			failed++
			continue
		}
		updated++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]any{
			"updated": updated, "failed": failed, "visibility": vis,
		},
	})
}

func (app *App) handleShare(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	var body struct {
		TTLSeconds int64 `json:"ttl_seconds"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body)
	ttl := time.Duration(body.TTLSeconds) * time.Second
	if ttl < time.Minute {
		ttl = app.cfg.ShareTTL
	}
	if ttl > 30*24*time.Hour {
		ttl = 30 * 24 * time.Hour
	}
	m, err := app.storage.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil || (m.UserID != user.ID && !user.IsAdmin) {
		writeErr(w, http.StatusNotFound, "not_found", "Fayl topilmadi")
		return
	}
	link, expiresAt := app.storage.ShareLink(m, ttl)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]any{
			"url":         link,
			"expires_at":  expiresAt.Format(time.RFC3339),
			"ttl_seconds": int64(ttl.Seconds()),
			"visibility":  m.Visibility,
			"note":        "Bu havola muddati tugaguncha KALITSIZ ochiladi — ehtiyot bo'ling",
		},
	})
}

func (app *App) handleSigned(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	exp := r.URL.Query().Get("exp")
	sig := r.URL.Query().Get("sig")
	if !app.storage.VerifyShare(id, exp, sig) {
		writeErr(w, http.StatusNotFound, "not_found", "Havola yaroqsiz yoki muddati o'tgan")
		return
	}
	m, err := app.storage.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "Fayl topilmadi")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Delivery", "signed")
	width := clampInt(atoiDef(r.URL.Query().Get("w"), 0), 0, 4096)
	app.deliverMedia(w, r, m, r.URL.Query().Get("v"), width)
}
