package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testEnv struct {
	app    *App
	srv    *httptest.Server
	apiKey string
	userID string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cfg := &Config{
		Port: "0", PublicURL: "http://liveimg.test", HasPublicURL: false,
		ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second,
		JWTSecret:  []byte("test-secret-0123456789-abcdefghijklmnopqrstuvwxyz"),
		JWTTTL:     time.Hour,
		DemoMode:   true,
		HDMaxWidth: 1600, HDQuality: 80,
		AllowAudio: true, ShareTTL: time.Hour, PaletteCount: 4,
		LRUSize: 1000, PublicRatePerMin: 100000, PublicBurst: 100000, PublicMaxAge: 31536000,
		DefaultVisible: "private",
		MemLimitBytes:  2 << 30, MemHighWater: 0.9, MemCritical: 0.95,
		MemTick: time.Second, MaxHeavyJobs: 4,
		KeepAliveEnabled: false,
	}

	app := &App{cfg: cfg, startedAt: time.Now(), limiter: NewRateLimiter()}
	app.gov = NewMemoryGovernor(cfg.MemLimitBytes, cfg.MemHighWater, cfg.MemCritical, cfg.MemTick, cfg.MaxHeavyJobs)
	app.gov.Start(ctx)
	mem := NewMemStore()
	app.core, app.meta = mem, mem
	app.vault = NewVaultClient(nil, "", time.Second, 5*time.Second, time.Minute)
	app.auth = NewAuth(cfg, app.core)
	app.storage = NewStorageService(cfg, app.core, app.meta, app.vault, app.gov, nil)
	app.storage.StartWorkers(ctx)
	app.status = NewStatusTracker(app)
	t.Cleanup(func() { app.storage.Close() })

	u, err := app.core.UpsertOAuthUser(ctx, &User{
		ID: newID("usr"), Email: "tester@example.com", Name: "Tester",
		Provider: "email", ProviderID: "tester-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.core.SaveSubscription(ctx, &Subscription{UserID: u.ID, Tier: TierBusiness, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	plain, _, err := app.auth.NewAPIKey(ctx, u.ID, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app.Router())
	t.Cleanup(srv.Close)
	return &testEnv{app: app, srv: srv, apiKey: plain, userID: u.ID}
}

func (e *testEnv) upload(t *testing.T, filename string, data []byte, extra map[string]string) (int, map[string]any) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	for k, v := range extra {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/images", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-API-Key", e.apiKey)
	return e.do(t, req)
}

func (e *testEnv) do(t *testing.T, req *http.Request) (int, map[string]any) {
	t.Helper()
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var out map[string]any
	if len(raw) > 0 && bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		_ = json.Unmarshal(raw, &out)
	}
	return resp.StatusCode, out
}

func (e *testEnv) get(t *testing.T, path string, withKey bool, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
	if withKey {
		req.Header.Set("X-API-Key", e.apiKey)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return resp, raw
}

func imageFromResp(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	data, _ := body["data"].(map[string]any)
	if data == nil {
		t.Fatalf("data yo'q: %v", body)
	}
	im, _ := data["image"].(map[string]any)
	if im == nil {
		t.Fatalf("image yo'q: %v", data)
	}
	return im
}

func TestUploadDefaultsToPrivate(t *testing.T) {
	e := newTestEnv(t)
	code, body := e.upload(t, "photo.png", samplePNG(t, 320, 200), nil)
	if code != http.StatusCreated {
		t.Fatalf("status = %d, body = %v", code, body)
	}
	im := imageFromResp(t, body)
	if im["visibility"] != "private" {
		t.Fatalf("standart ko'rinish = %v, want private", im["visibility"])
	}
	if im["kind"] != "image" {
		t.Fatalf("kind = %v", im["kind"])
	}
	urls, _ := im["urls"].(map[string]any)
	if ph := fmt.Sprint(urls["placeholder"]); ph != "" && ph != "<nil>" {
		t.Fatalf("placeholder (base64) chiqmasin: %v", urls["placeholder"])
	}
	if bg := fmt.Sprint(im["bg"]); !validBg(bg) {
		t.Fatalf("bg rang yo'q: %v", im["bg"])
	}
	id := fmt.Sprint(im["id"])
	if !validMediaID(id) || len(id) != 8 || strings.HasPrefix(id, "img_") {
		t.Fatalf("CDN id qisqa emas: %q", id)
	}
	if urls["download"] != "/d/"+fmt.Sprint(im["id"]) {
		t.Fatalf("download havolasi: %v", urls["download"])
	}

	resp, _ := e.get(t, "/cdn/"+fmt.Sprint(im["id"]), false, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("yopiq fayl kalitsiz ochildi: %d", resp.StatusCode)
	}
	resp, raw := e.get(t, "/cdn/"+fmt.Sprint(im["id"]), true, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("egasi uchun status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type = %q", ct)
	}
	if len(raw) == 0 {
		t.Fatal("bo'sh javob")
	}
}

func TestUploadPublicWhenRequested(t *testing.T) {
	e := newTestEnv(t)
	code, body := e.upload(t, "public.png", samplePNG(t, 100, 100), map[string]string{"visibility": "public"})
	if code != http.StatusCreated {
		t.Fatalf("status = %d %v", code, body)
	}
	im := imageFromResp(t, body)
	if im["visibility"] != "public" {
		t.Fatalf("visibility = %v", im["visibility"])
	}
	resp, _ := e.get(t, "/cdn/"+fmt.Sprint(im["id"]), false, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ochiq fayl status = %d", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("cache-control = %q", cc)
	}
	_, body2 := e.upload(t, "mode.png", samplePNG(t, 60, 60), map[string]string{"mode": "public"})
	if im2 := imageFromResp(t, body2); im2["visibility"] != "public" {
		t.Fatalf("mode=public ishlamadi: %v", im2["visibility"])
	}
}

func TestUploadVideoRejected(t *testing.T) {
	e := newTestEnv(t)
	code, body := e.upload(t, "clip.mp4", sampleMP4(), map[string]string{"visibility": "public"})
	if code != http.StatusUnsupportedMediaType {
		t.Fatalf("video rad etilmadi: %d %v", code, body)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj == nil || !strings.Contains(fmt.Sprint(errObj["message"]), "video") {
		t.Fatalf("xato matnida video haqida izoh yo'q: %v", body)
	}
}

func TestUploadGIFRejected(t *testing.T) {
	e := newTestEnv(t)
	code, body := e.upload(t, "anim.gif", sampleGIF(t, 2), nil)
	if code != http.StatusUnsupportedMediaType {
		t.Fatalf("GIF rad etilmadi: %d %v", code, body)
	}
}

func TestUploadMP3Accepted(t *testing.T) {
	e := newTestEnv(t)
	raw := append(sampleMP3(), make([]byte, 64)...)
	code, body := e.upload(t, "song.mp3", raw, map[string]string{"visibility": "public"})
	if code != http.StatusCreated {
		t.Fatalf("MP3 qabul qilinmadi: %d %v", code, body)
	}
	im := imageFromResp(t, body)
	if im["kind"] != "audio" || im["mime_type"] != "audio/mpeg" {
		t.Fatalf("kind/mime = %v/%v", im["kind"], im["mime_type"])
	}
	if fn := fmt.Sprint(im["filename"]); !strings.HasSuffix(fn, ".mp3") {
		t.Fatalf("filename kengaytmasi yo'q: %q", fn)
	}
	urls, _ := im["urls"].(map[string]any)
	resp, got := e.get(t, urls["cdn"].(string), false, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("MP3 CDN status = %d", resp.StatusCode)
	}
	if !bytes.Equal(got, raw) {
		t.Fatal("MP3 baytlari o'zgargan")
	}
	if ct := resp.Header.Get("Content-Type"); ct != "audio/mpeg" {
		t.Fatalf("content-type = %q", ct)
	}
}

type countSub struct {
	CoreStore
	n atomic.Int64
}

func (c *countSub) GetSubscription(ctx context.Context, userID string) (*Subscription, error) {
	c.n.Add(1)
	return c.CoreStore.GetSubscription(ctx, userID)
}

func TestCDNGetSkipsDB(t *testing.T) {
	e := newTestEnv(t)
	cs := &countSub{CoreStore: e.app.core}
	e.app.core = cs
	e.app.storage.core = cs
	e.app.auth.core = cs

	_, body := e.upload(t, "photo.png", samplePNG(t, 80, 60), map[string]string{"visibility": "public"})
	im := imageFromResp(t, body)
	id := fmt.Sprint(im["id"])
	before := cs.n.Load()

	for _, tc := range []struct {
		path string
		auth bool
	}{
		{"/cdn/" + id, false}, {"/preview/" + id, false},
		{"/i/" + id, true}, {"/d/" + id, true},
	} {
		resp, _ := e.get(t, tc.path, tc.auth, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d", tc.path, resp.StatusCode)
		}
	}
	if got := cs.n.Load(); got != before {
		t.Fatalf("CDN GET GetSubscription chaqirdi: %d → %d", before, got)
	}

	e.app.auth.PurgeSub(e.userID)
	resp, _ := e.get(t, "/cdn/"+id+"?w=40", false, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("?w= status = %d", resp.StatusCode)
	}
	if got := cs.n.Load(); got != before {
		t.Fatalf("?w= GetSubscription chaqirdi: %d → %d", before, got)
	}
}

type countMeta struct {
	MetaStore
	n atomic.Int64
}

func (c *countMeta) GetImage(ctx context.Context, id string) (*ImageMeta, error) {
	c.n.Add(1)
	return c.MetaStore.GetImage(ctx, id)
}

func TestCDNGetUsesTicketNotMongo(t *testing.T) {
	e := newTestEnv(t)
	cm := &countMeta{MetaStore: e.app.meta}
	e.app.meta = cm
	e.app.storage.meta = cm

	_, body := e.upload(t, "photo.png", samplePNG(t, 80, 60), map[string]string{"visibility": "public"})
	im := imageFromResp(t, body)
	id := fmt.Sprint(im["id"])
	if strings.HasPrefix(id, "img_") || len(id) != 8 || !validMediaID(id) {
		t.Fatalf("CDN id qisqa emas: %q", id)
	}

	e.app.storage.metaCache.Remove(id)
	before := cm.n.Load()
	resp, _ := e.get(t, "/cdn/"+id, false, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CDN status = %d", resp.StatusCode)
	}
	if got := cm.n.Load(); got != before {
		t.Fatalf("ticket bor, lekin GetImage chaqirildi: %d → %d", before, got)
	}

	e.app.storage.forget(id)
	resp, _ = e.get(t, "/cdn/"+id, false, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sovuq CDN status = %d", resp.StatusCode)
	}
	if cm.n.Load() <= before {
		t.Fatal("ticket yo'qolgach GetImage chaqirilmadi")
	}
}

func TestPreviewIsColorSwatchNotHD(t *testing.T) {
	e := newTestEnv(t)
	png := samplePNG(t, 320, 200)
	_, body := e.upload(t, "photo.png", png, map[string]string{"visibility": "public"})
	im := imageFromResp(t, body)
	id := fmt.Sprint(im["id"])
	resp, raw := e.get(t, "/preview/"+id, false, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview status = %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "svg") {
		t.Fatalf("preview content-type = %q, want svg swatch", ct)
	}
	if len(raw) > 400 {
		t.Fatalf("preview HD oqizilgan bo'lishi mumkin: %d bytes (asl %d)", len(raw), len(png))
	}
	if !bytes.Contains(raw, []byte("fill=")) {
		t.Fatal("preview swatch fill yo'q")
	}
}

func TestUploadM4AKeepsBytes(t *testing.T) {
	e := newTestEnv(t)
	raw := append(sampleM4A(), make([]byte, 64)...)
	code, body := e.upload(t, "tone.m4a", raw, map[string]string{"visibility": "public"})
	if code != http.StatusCreated {
		t.Fatalf("status = %d %v", code, body)
	}
	im := imageFromResp(t, body)
	if im["kind"] != "audio" || im["mime_type"] != "audio/mp4" {
		t.Fatalf("kind/mime = %v/%v", im["kind"], im["mime_type"])
	}
	id := fmt.Sprint(im["id"])
	if _, ok := e.app.storage.staging.Get(hotKey(id, "hd")); ok {
		t.Fatal("audio RAM staging'ga tushdi")
	}
	resp, _ := e.get(t, "/cdn/"+id, false, map[string]string{"Range": "bytes=0-15"})
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("range status = %d (206 kutilgan)", resp.StatusCode)
	}
	if cr := resp.Header.Get("Content-Range"); !strings.HasPrefix(cr, "bytes 0-15/") {
		t.Fatalf("content-range = %q", cr)
	}
}

func TestUploadRejectsUnsupportedFormat(t *testing.T) {
	e := newTestEnv(t)
	code, body := e.upload(t, "note.txt", []byte("bu shunchaki matn fayli"), nil)
	if code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d %v", code, body)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj == nil || !strings.Contains(fmt.Sprint(errObj["message"]), "M4A") {
		t.Fatalf("xato matnida formatlar ro'yxati yo'q: %v", body)
	}
}

func TestSVGServedWithStrictCSP(t *testing.T) {
	e := newTestEnv(t)
	_, body := e.upload(t, "logo.svg", sampleSVG(), map[string]string{"visibility": "public"})
	im := imageFromResp(t, body)
	if im["mime_type"] != "image/svg+xml" || im["width"].(float64) != 320 {
		t.Fatalf("svg metadata: %v", im)
	}
	urls, _ := im["urls"].(map[string]any)
	resp, raw := e.get(t, fmt.Sprint(urls["cdn"]), false, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("svg status = %d", resp.StatusCode)
	}
	if !bytes.Contains(raw, []byte("<svg")) {
		t.Fatal("svg tarkibi buzilgan")
	}
	csp := resp.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'none'") || !strings.Contains(csp, "sandbox") {
		t.Fatalf("SVG uchun qat'iy CSP yo'q: %q", csp)
	}
}

func TestSignedShareLinkOpensPrivateFile(t *testing.T) {
	e := newTestEnv(t)
	_, body := e.upload(t, "secret.png", samplePNG(t, 64, 64), nil)
	im := imageFromResp(t, body)
	id := fmt.Sprint(im["id"])

	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/images/"+id+"/share",
		strings.NewReader(`{"ttl_seconds":3600}`))
	req.Header.Set("X-API-Key", e.apiKey)
	req.Header.Set("Content-Type", "application/json")
	code, share := e.do(t, req)
	if code != http.StatusOK {
		t.Fatalf("share status = %d %v", code, share)
	}
	data, _ := share["data"].(map[string]any)
	link := fmt.Sprint(data["url"])
	if !strings.Contains(link, "/s/"+id+"?exp=") || !strings.Contains(link, "sig=") {
		t.Fatalf("havola formati noto'g'ri: %s", link)
	}

	resp, _ := e.get(t, strings.TrimPrefix(link, "http://liveimg.test"), false, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("imzolangan havola status = %d", resp.StatusCode)
	}
	resp2, _ := e.get(t, "/s/"+id+"?exp="+strings.Split(strings.Split(link, "exp=")[1], "&")[0]+"&sig=deadbeef", false, nil)
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("buzilgan imzo status = %d", resp2.StatusCode)
	}
	expired := e.app.storage.SignShare(id, time.Now().Add(-time.Hour))
	resp3, _ := e.get(t, "/s/"+id+"?exp="+fmt.Sprint(time.Now().Add(-time.Hour).Unix())+"&sig="+expired, false, nil)
	if resp3.StatusCode != http.StatusNotFound {
		t.Fatalf("eskirgan havola status = %d", resp3.StatusCode)
	}
}

func TestListFiltersAndBulkOperations(t *testing.T) {
	e := newTestEnv(t)
	_, imgBody := e.upload(t, "alpha.png", samplePNG(t, 50, 50), nil)
	_, audBody := e.upload(t, "beta-tone.m4a", append(sampleM4A(), make([]byte, 32)...), map[string]string{"visibility": "public"})
	_, svgBody := e.upload(t, "gamma.svg", sampleSVG(), nil)

	imgID := fmt.Sprint(imageFromResp(t, imgBody)["id"])
	audID := fmt.Sprint(imageFromResp(t, audBody)["id"])
	svgID := fmt.Sprint(imageFromResp(t, svgBody)["id"])

	list := func(query string) []any {
		req, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/v1/images?"+query, nil)
		req.Header.Set("X-API-Key", e.apiKey)
		code, body := e.do(t, req)
		if code != http.StatusOK {
			t.Fatalf("list %s status = %d %v", query, code, body)
		}
		data, _ := body["data"].([]any)
		return data
	}
	if n := len(list("limit=50")); n != 3 {
		t.Fatalf("jami fayl = %d, want 3", n)
	}
	if n := len(list("kind=audio")); n != 1 {
		t.Fatalf("audio filtri = %d", n)
	}
	if n := len(list("kind=svg")); n != 1 {
		t.Fatalf("svg filtri = %d", n)
	}
	if n := len(list("visibility=private")); n != 2 {
		t.Fatalf("private filtri = %d", n)
	}
	if n := len(list("q=BETA")); n != 1 {
		t.Fatalf("qidiruv filtri = %d", n)
	}
	if n := len(list("kind=image&sort=name")); n != 2 {
		t.Fatalf("image filtri = %d", n)
	}

	row0, _ := list("limit=50")[0].(map[string]any)
	if row0["urls"] == nil || row0["id"] == nil {
		t.Fatalf("ro'yxat qatori urls/id emas: %v", row0)
	}
	for _, banned := range []string{"size_bytes", "filename", "checksum", "created_at", "width"} {
		if _, ok := row0[banned]; ok {
			t.Fatalf("ro'yxat %s ni ham qaytardi: %v", banned, row0)
		}
	}

	reqD, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/v1/images/details?ids="+imgID+","+audID, nil)
	reqD.Header.Set("X-API-Key", e.apiKey)
	codeD, bodyD := e.do(t, reqD)
	if codeD != http.StatusOK {
		t.Fatalf("details status = %d %v", codeD, bodyD)
	}
	dArr, _ := bodyD["data"].([]any)
	if len(dArr) != 2 {
		t.Fatalf("details soni = %d, want 2", len(dArr))
	}
	det, _ := dArr[0].(map[string]any)
	if det["size_bytes"] == nil || det["filename"] == nil || det["urls"] == nil {
		t.Fatalf("details to'liq emas: %v", det)
	}

	req, _ := http.NewRequest(http.MethodPatch, e.srv.URL+"/v1/images?ids="+imgID+","+svgID+"&visibility=public", nil)
	req.Header.Set("X-API-Key", e.apiKey)
	code, body := e.do(t, req)
	if code != http.StatusOK {
		t.Fatalf("bulk patch status = %d %v", code, body)
	}
	if d, _ := body["data"].(map[string]any); d["updated"].(float64) != 2 {
		t.Fatalf("updated = %v", d["updated"])
	}
	resp, _ := e.get(t, "/cdn/"+imgID, false, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ochiq qilingan fayl status = %d", resp.StatusCode)
	}

	req2, _ := http.NewRequest(http.MethodDelete, e.srv.URL+"/v1/images?ids="+imgID+","+audID+",nomavjud_id_uzun", nil)
	req2.Header.Set("X-API-Key", e.apiKey)
	code2, body2 := e.do(t, req2)
	if code2 != http.StatusOK {
		t.Fatalf("bulk delete status = %d %v", code2, body2)
	}
	d2, _ := body2["data"].(map[string]any)
	if d2["deleted_count"].(float64) != 2 || d2["failed_count"].(float64) != 1 {
		t.Fatalf("bulk delete natijasi = %v", d2)
	}
	if n := len(list("limit=50")); n != 1 {
		t.Fatalf("o'chirishdan keyin qolgan fayl = %d, want 1", n)
	}
}

func TestDownloadEndpointSetsAttachment(t *testing.T) {
	e := newTestEnv(t)
	_, body := e.upload(t, "my file (1).png", samplePNG(t, 40, 40), map[string]string{"visibility": "public"})
	im := imageFromResp(t, body)
	resp, _ := e.get(t, "/d/"+fmt.Sprint(im["id"]), true, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download status = %d", resp.StatusCode)
	}
	cd := resp.Header.Get("Content-Disposition")
	if !strings.HasPrefix(cd, "attachment;") || !strings.Contains(cd, "filename*=UTF-8''") {
		t.Fatalf("content-disposition = %q", cd)
	}
}

func TestPublicAPIEndpoints(t *testing.T) {
	e := newTestEnv(t)
	resp, raw := e.get(t, "/healthz", false, nil)
	if resp.StatusCode != http.StatusOK || !bytes.Contains(raw, []byte(`"status":"ok"`)) {
		t.Fatalf("healthz = %d %s", resp.StatusCode, raw)
	}
	if bytes.Contains(raw, []byte("uptime")) {
		t.Fatal("healthz anonim foydalanuvchiga uptime ko'rsatdi")
	}
	for _, p := range []string{"/openapi.json", "/v1/status", "/auth/session"} {
		r2, raw2 := e.get(t, p, false, nil)
		if r2.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d", p, r2.StatusCode)
		}
		if p == "/v1/status" && !bytes.Contains(raw2, []byte(`"status"`)) {
			t.Fatalf("status javobi bo'sh: %s", raw2)
		}
		if p == "/auth/session" && !bytes.Contains(raw2, []byte(`"email"`)) {
			t.Fatalf("session javobida providerlar yo'q: %s", raw2)
		}
	}
}

func TestUploadQuotaPerKind(t *testing.T) {
	pol := DefaultPolicies[TierFree]

	want := map[MediaKind]int64{
		KindImage: MaxImageSizeKB * 1024,
		KindAudio: MaxAudioSizeKB * 1024,
	}
	for _, tier := range []Tier{TierFree, TierPro, TierBusiness} {
		p := DefaultPolicies[tier]
		for kind, limit := range want {
			if got := p.MaxBytesFor(kind); got != limit {
				t.Fatalf("%s/%s limiti = %d, kutilgan %d", tier, kind, got, limit)
			}
		}
		if !p.UnlimitedTraffic {
			t.Fatalf("%s: trafik cheklangan", tier)
		}
	}

	for kind, limit := range want {
		if qe := CheckUploadQuota(pol, &UsageCounter{}, limit+1, kind); qe == nil || qe.Code != "file_too_large" {
			t.Fatalf("%s chegaradan katta fayl rad etilmadi: %v", kind, qe)
		}
		if qe := CheckUploadQuota(pol, &UsageCounter{}, limit, kind); qe != nil {
			t.Fatalf("%s chegaradagi fayl rad etildi: %v", kind, qe)
		}
	}

	custom := DefaultPolicies[TierBusiness]
	custom.MaxUploadKB = MaxImageSizeKB * 2
	custom.MaxAudioKB = MaxAudioSizeKB * 2
	for kind, limit := range want {
		if got := custom.MaxBytesFor(kind); got != limit {
			t.Fatalf("custom %s limiti global ceiling'dan oshdi: %d", kind, got)
		}
	}

	up, aud := MaxImageSizeKB*2, MaxAudioSizeKB*2
	effective := EffectivePolicy(&Subscription{
		Tier:              TierBusiness,
		CustomMaxUploadKB: &up,
		CustomMaxAudioKB:  &aud,
		StorageRules:      map[string]any{"unlimited_traffic": true},
	})
	if effective.MaxUploadKB != MaxImageSizeKB || effective.MaxAudioKB != MaxAudioSizeKB {
		t.Fatalf("effective custom limit global ceiling bilan normallashtirilmadi: %+v", effective)
	}
	if !effective.UnlimitedTraffic {
		t.Fatal("EffectivePolicy cheksiz trafikni saqlamadi")
	}
	off := EffectivePolicy(&Subscription{Tier: TierPro, StorageRules: map[string]any{"allow_audio": false}})
	if off.AllowAudio {
		t.Fatal("allow_audio=false qo'llanmadi")
	}

	noAudio := pol
	noAudio.AllowAudio = false
	if qe := CheckUploadQuota(noAudio, &UsageCounter{}, 1024, KindAudio); qe == nil || qe.Code != "audio_not_allowed" {
		t.Fatalf("audio ruxsati tekshirilmadi: %v", qe)
	}

	free := DefaultPolicies[TierFree]
	if free.RatePerMin != 120 || free.Burst != 60 {
		t.Fatalf("Free so'rov limiti o'zgardi: rate=%d burst=%d", free.RatePerMin, free.Burst)
	}
	if free.StorageMB != 0 || free.MonthlyFiles != 0 {
		t.Fatalf("Free saqlash/oylik cheksiz emas: storage=%d monthly=%d", free.StorageMB, free.MonthlyFiles)
	}
	if !free.OnTheFlyResize || !free.Analytics {
		t.Fatal("Free'da resize/analytics o'chiq")
	}
	if qe := CheckUploadQuota(free, &UsageCounter{StorageBytes: 1 << 40, MonthFiles: 1_000_000}, 1024, KindImage); qe != nil {
		t.Fatalf("cheksiz kvota rad etildi: %v", qe)
	}
	if DefaultPolicies[TierPro].StorageMB != 0 || TierPrice[TierPro] != "0" {
		t.Fatal("eski Pro ham bepul/cheksiz bo'lishi kerak")
	}
	if EffectivePolicy(&Subscription{Tier: TierPro}).Name != TierFree {
		t.Fatal("pro yozuvi bepulga tushmadi")
	}
}

func TestEmailSignupAndLogin(t *testing.T) {
	e := newTestEnv(t)

	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/auth/signup",
		strings.NewReader(`{"name":"New User","email":"new@vebox.local","password":"secret123"}`))
	req.Header.Set("Content-Type", "application/json")
	code, body := e.do(t, req)
	if code != http.StatusCreated {
		t.Fatalf("signup status = %d %v", code, body)
	}
	data, _ := body["data"].(map[string]any)
	_ = data
	if body["user"] == nil {
		t.Fatalf("user yo'q: %v", body)
	}
	userObj, _ := body["user"].(map[string]any)
	if userObj["email"] != "new@vebox.local" {
		t.Fatalf("email = %v", userObj["email"])
	}
	if body["token"] == nil || fmt.Sprint(body["token"]) == "" {
		t.Fatal("JWT token qaytarilmadi")
	}

	req2, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/auth/signup",
		strings.NewReader(`{"email":"new@vebox.local","password":"secret123"}`))
	req2.Header.Set("Content-Type", "application/json")
	code2, _ := e.do(t, req2)
	if code2 != http.StatusConflict {
		t.Fatalf("takroriy signup status = %d, want 409", code2)
	}

	req3, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/auth/login",
		strings.NewReader(`{"email":"NEW@vebox.local","password":"secret123"}`))
	req3.Header.Set("Content-Type", "application/json")
	code3, body3 := e.do(t, req3)
	if code3 != http.StatusOK {
		t.Fatalf("login status = %d %v", code3, body3)
	}
	if body3["token"] == nil {
		t.Fatal("login token yo'q")
	}

	req4, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/auth/login",
		strings.NewReader(`{"email":"new@vebox.local","password":"wrongpass"}`))
	req4.Header.Set("Content-Type", "application/json")
	code4, _ := e.do(t, req4)
	if code4 != http.StatusUnauthorized {
		t.Fatalf("noto'g'ri parol status = %d, want 401", code4)
	}

	req5, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/auth/login",
		strings.NewReader(`{"email":"ghost@vebox.local","password":"secret123"}`))
	req5.Header.Set("Content-Type", "application/json")
	code5, _ := e.do(t, req5)
	if code5 != http.StatusUnauthorized {
		t.Fatalf("mavjud emas email status = %d, want 401", code5)
	}
}

func TestEmailAuthRejectsWeakPassword(t *testing.T) {
	e := newTestEnv(t)
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/auth/signup",
		strings.NewReader(`{"email":"weak@vebox.local","password":"123"}`))
	req.Header.Set("Content-Type", "application/json")
	code, _ := e.do(t, req)
	if code != http.StatusBadRequest {
		t.Fatalf("qisqa parol status = %d, want 400", code)
	}
	req2, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/auth/signup",
		strings.NewReader(`{"email":"not-an-email","password":"longenough123"}`))
	req2.Header.Set("Content-Type", "application/json")
	code2, _ := e.do(t, req2)
	if code2 != http.StatusBadRequest {
		t.Fatalf("noto'g'ri email status = %d, want 400", code2)
	}
}

func TestJWTTokenGrantsAccess(t *testing.T) {
	e := newTestEnv(t)
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/auth/signup",
		strings.NewReader(`{"email":"jwt@vebox.local","password":"secret123"}`))
	req.Header.Set("Content-Type", "application/json")
	code, body := e.do(t, req)
	if code != http.StatusCreated {
		t.Fatalf("signup status = %d %v", code, body)
	}
	token := fmt.Sprint(body["token"])

	req2, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/v1/me", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	code2, body2 := e.do(t, req2)
	if code2 != http.StatusOK {
		t.Fatalf("/v1/me Bearer bilan status = %d %v", code2, body2)
	}
	acc, _ := body2["data"].(map[string]any)
	if acc == nil || acc["user"] == nil {
		t.Fatalf("me javobi bo'sh: %v", body2)
	}
}

func TestProfileUpdateAndAvatarUpload(t *testing.T) {
	e := newTestEnv(t)

	req, _ := http.NewRequest(http.MethodPatch, e.srv.URL+"/v1/profile",
		strings.NewReader(`{"name":"Yangi Ism"}`))
	req.Header.Set("X-API-Key", e.apiKey)
	req.Header.Set("Content-Type", "application/json")
	code, body := e.do(t, req)
	if code != http.StatusOK {
		t.Fatalf("profile patch status = %d %v", code, body)
	}
	d, _ := body["data"].(map[string]any)
	if d["name"] != "Yangi Ism" {
		t.Fatalf("name = %v", d["name"])
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "avatar.png")
	_, _ = fw.Write(samplePNG(t, 64, 64))
	_ = mw.Close()
	req2, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/profile/avatar", &buf)
	req2.Header.Set("Content-Type", mw.FormDataContentType())
	req2.Header.Set("X-API-Key", e.apiKey)
	code2, body2 := e.do(t, req2)
	if code2 != http.StatusOK {
		t.Fatalf("avatar upload status = %d %v", code2, body2)
	}
	d2, _ := body2["data"].(map[string]any)
	avatarURL := fmt.Sprint(d2["avatar_url"])
	if !strings.Contains(avatarURL, "/cdn/") {
		t.Fatalf("avatar_url CDN'da emas: %q", avatarURL)
	}
	if aid := mediaIDFromURL(avatarURL); aid == "" || len(aid) != 8 {
		t.Fatalf("avatar id qisqa emas: %q", avatarURL)
	}
	resp, _ := e.get(t, avatarURL, false, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("avatar yetkazib berish status = %d", resp.StatusCode)
	}

	var bufGif bytes.Buffer
	mwGif := multipart.NewWriter(&bufGif)
	fwGif, _ := mwGif.CreateFormFile("file", "avatar.gif")
	_, _ = fwGif.Write(sampleGIF(t, 1))
	_ = mwGif.Close()
	req3, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/profile/avatar", &bufGif)
	req3.Header.Set("Content-Type", mwGif.FormDataContentType())
	req3.Header.Set("X-API-Key", e.apiKey)
	code3, _ := e.do(t, req3)
	if code3 != http.StatusUnsupportedMediaType {
		t.Fatalf("GIF avatar status = %d, want 415", code3)
	}
}

func TestAPIKeyPermanentDelete(t *testing.T) {
	e := newTestEnv(t)

	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/keys",
		strings.NewReader(`{"name":"temp"}`))
	req.Header.Set("X-API-Key", e.apiKey)
	req.Header.Set("Content-Type", "application/json")
	code, body := e.do(t, req)
	if code != http.StatusCreated {
		t.Fatalf("key create status = %d %v", code, body)
	}
	if body["api_key"] == nil || fmt.Sprint(body["api_key"]) == "" {
		t.Fatal("yangi kalit qaytarilmadi")
	}

	req2, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/v1/keys", nil)
	req2.Header.Set("X-API-Key", e.apiKey)
	_, listBody := e.do(t, req2)
	keys, _ := listBody["data"].([]any)
	if len(keys) < 2 {
		t.Fatalf("kalitlar ro'yxati = %d, kamida 2 bo'lishi kerak", len(keys))
	}

	keyID := ""
	for _, k := range keys {
		kk, _ := k.(map[string]any)
		if kk["name"] == "temp" {
			keyID = fmt.Sprint(kk["id"])
		}
	}
	if keyID == "" {
		t.Fatal("yangi kalit ID topilmadi")
	}

	req3, _ := http.NewRequest(http.MethodDelete, e.srv.URL+"/v1/keys/"+keyID, nil)
	req3.Header.Set("X-API-Key", e.apiKey)
	code3, body3 := e.do(t, req3)
	if code3 != http.StatusOK {
		t.Fatalf("key delete status = %d %v", code3, body3)
	}

	req4, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/v1/keys", nil)
	req4.Header.Set("X-API-Key", e.apiKey)
	_, listBody2 := e.do(t, req4)
	keys2, _ := listBody2["data"].([]any)
	for _, k := range keys2 {
		kk, _ := k.(map[string]any)
		if fmt.Sprint(kk["id"]) == keyID {
			t.Fatal("o'chirilgan kalit hali ro'yxatda")
		}
	}

	req5, _ := http.NewRequest(http.MethodDelete, e.srv.URL+"/v1/keys/key_noma'lum_123", nil)
	req5.Header.Set("X-API-Key", e.apiKey)
	code5, _ := e.do(t, req5)
	if code5 != http.StatusNotFound {
		t.Fatalf("noma'lum key delete status = %d, want 404", code5)
	}

	req6, _ := http.NewRequest(http.MethodDelete, e.srv.URL+"/v1/keys", nil)
	req6.Header.Set("X-API-Key", e.apiKey)
	code6, _ := e.do(t, req6)
	if code6 != http.StatusOK {
		t.Fatalf("delete all status = %d", code6)
	}
	req7, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/v1/keys", nil)
	code7, _ := e.do(t, req7)
	_ = code7
}

func TestOAuthStateSurvivesWithoutLRU(t *testing.T) {
	e := newTestEnv(t)
	next := "/console"
	state := e.app.auth.mintOAuthState(next)
	got, ok := e.app.auth.parseOAuthState(state)
	if !ok || got != next {
		t.Fatalf("imzolangan state ishlamadi: ok=%v next=%q", ok, got)
	}
	e.app.auth.states.Purge()
	got, ok = e.app.auth.parseOAuthState(state)
	if !ok || got != next {
		t.Fatalf("LRU'siz state: ok=%v next=%q", ok, got)
	}
	if _, ok = e.app.auth.parseOAuthState("buzuq"); ok {
		t.Fatal("buzuq state qabul qilindi")
	}
}

func TestUpsertOAuthLinksByEmail(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	email := "same@vebox.local"
	u1, err := e.app.core.UpsertOAuthUser(ctx, &User{
		ID: newID("usr"), Email: email, Name: "Mail",
		Provider: "email", ProviderID: "email:" + email,
	})
	if err != nil {
		t.Fatal(err)
	}
	u2, err := e.app.core.UpsertOAuthUser(ctx, &User{
		ID: newID("usr"), Email: email, Name: "Octo",
		Provider: "github", ProviderID: "4242", AvatarURL: "https://gh/a.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if u2.ID != u1.ID {
		t.Fatalf("GitHub yangi qator yaratdi: %s vs %s", u2.ID, u1.ID)
	}
	if u2.Provider != "github" || u2.ProviderID != "4242" {
		t.Fatalf("provider yangilanmadi: %s/%s", u2.Provider, u2.ProviderID)
	}
	got, err := e.app.core.GetUser(ctx, u1.ID)
	if err != nil || got.Provider != "github" {
		t.Fatalf("bazada github yo'q: %+v %v", got, err)
	}
}

func TestStaleAPIKeyFallsBackToJWT(t *testing.T) {
	e := newTestEnv(t)
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/auth/signup",
		strings.NewReader(`{"email":"jwt2@vebox.local","password":"secret123"}`))
	req.Header.Set("Content-Type", "application/json")
	code, body := e.do(t, req)
	if code != http.StatusCreated {
		t.Fatalf("signup status = %d %v", code, body)
	}
	token := fmt.Sprint(body["token"])

	req2, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/v1/me", nil)
	req2.Header.Set("X-API-Key", "vb_live_stale_not_a_real_key")
	req2.Header.Set("Authorization", "Bearer "+token)
	code2, body2 := e.do(t, req2)
	if code2 != http.StatusOK {
		t.Fatalf("eski kalit JWT ni yopdi: %d %v", code2, body2)
	}
}

func TestStatusUptimeIsProcessAge(t *testing.T) {
	e := newTestEnv(t)
	time.Sleep(20 * time.Millisecond)
	resp, raw := e.get(t, "/v1/status", false, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	up, _ := out["uptime"].(map[string]any)
	if up == nil {
		t.Fatal("uptime yo'q")
	}
	sec, _ := up["seconds"].(float64)
	if sec < 0 {
		t.Fatalf("uptime seconds = %v", sec)
	}
}

func TestStatusEndpointPublic(t *testing.T) {
	e := newTestEnv(t)
	resp, raw := e.get(t, "/v1/status", false, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out["status"] != "operational" {
		t.Fatalf("status = %v", out["status"])
	}
	if out["uptime"] == nil {
		t.Fatal("uptime maydoni yo'q")
	}
	if out["history"] == nil {
		t.Fatal("history maydoni yo'q")
	}
}

func TestSettingsToggleHD(t *testing.T) {
	e := newTestEnv(t)

	get := func() map[string]any {
		req, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/v1/settings", nil)
		req.Header.Set("X-API-Key", e.apiKey)
		st, body := e.do(t, req)
		if st != http.StatusOK {
			t.Fatalf("GET /v1/settings status = %d: %v", st, body)
		}
		data, _ := body["data"].(map[string]any)
		if data == nil {
			t.Fatalf("data yo'q: %v", body)
		}
		return data
	}

	d := get()
	if d["hd_processing"] != true {
		t.Fatalf("default hd_processing = %v, kutilgan true", d["hd_processing"])
	}
	if d["max_hd_width"] == nil {
		t.Fatal("max_hd_width yo'q")
	}

	pb := bytes.NewBufferString(`{"hd_processing":false}`)
	req, _ := http.NewRequest(http.MethodPatch, e.srv.URL+"/v1/settings", pb)
	req.Header.Set("X-API-Key", e.apiKey)
	req.Header.Set("Content-Type", "application/json")
	st, body := e.do(t, req)
	if st != http.StatusOK {
		t.Fatalf("PATCH status = %d: %v", st, body)
	}
	pd, _ := body["data"].(map[string]any)
	if pd == nil || pd["hd_processing"] != false {
		t.Fatalf("PATCH javobi noto'g'ri: %v", body)
	}

	if d = get(); d["hd_processing"] != false {
		t.Fatalf("saqlanmadi: %v", d)
	}

	req, _ = http.NewRequest(http.MethodGet, e.srv.URL+"/v1/me", nil)
	req.Header.Set("X-API-Key", e.apiKey)
	st, body = e.do(t, req)
	if st != http.StatusOK {
		t.Fatalf("GET /v1/me status = %d", st)
	}
	acc, _ := body["data"].(map[string]any)["account"].(map[string]any)
	limits, _ := acc["limits"].(map[string]any)
	if limits == nil || limits["hd_processing"] != false {
		t.Fatalf("limits.hd_processing = %v, kutilgan false (limits: %v)", limits["hd_processing"], limits)
	}

	pb = bytes.NewBufferString(`{"hd_processing":true}`)
	req, _ = http.NewRequest(http.MethodPatch, e.srv.URL+"/v1/settings", pb)
	req.Header.Set("X-API-Key", e.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if st, _ = e.do(t, req); st != http.StatusOK {
		t.Fatalf("PATCH(yoqish) status = %d", st)
	}
	if d = get(); d["hd_processing"] != true {
		t.Fatalf("qayta yoqilmadi: %v", d)
	}
}
