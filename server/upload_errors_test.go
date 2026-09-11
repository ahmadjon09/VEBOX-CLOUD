package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
	"time"
)

func limitEnv(t *testing.T, maxKB int64) *testEnv {
	t.Helper()
	e := newTestEnv(t)
	up, aud := maxKB, maxKB
	sub := &Subscription{
		UserID: e.userID, Tier: TierBusiness, UpdatedAt: time.Now(),
		CustomMaxUploadKB: &up, CustomMaxAudioKB: &aud,
	}
	if err := e.app.core.SaveSubscription(context.Background(), sub); err != nil {
		t.Fatalf("SaveSubscription: %v", err)
	}
	return e
}

func (e *testEnv) uploadResp(t *testing.T, filename string, data []byte) (*http.Response, map[string]any) {
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
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/v1/images", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-API-Key", e.apiKey)
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("so'rov yuborilmadi (server ulanishni uzgan bo'lishi mumkin): %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return resp, out
}

func errCode(body map[string]any) string {
	if body == nil {
		return ""
	}
	e, _ := body["error"].(map[string]any)
	if e == nil {
		return ""
	}
	c, _ := e["code"].(string)
	return c
}

func TestOversizeBodyRejectedBeforeReading(t *testing.T) {
	e := limitEnv(t, 64)
	big := bytes.Repeat([]byte{0x5A}, (64+1024+256)*1024)
	resp, body := e.uploadResp(t, "clip.png", big)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, kutilgan 413 (body=%v)", resp.StatusCode, body)
	}
	if got := errCode(body); got != "file_too_large" {
		t.Fatalf("xato kodi = %q, kutilgan file_too_large", got)
	}
	if resp.Header.Get("X-Max-Upload-Bytes") == "" {
		t.Fatal("X-Max-Upload-Bytes header'i yuborilmadi (frontend preflight uchun kerak)")
	}
}

func TestOversizeFileRejectedAfterMultipart(t *testing.T) {
	e := limitEnv(t, 64)
	small := bytes.Repeat([]byte{0x5A}, 200*1024)
	resp, body := e.uploadResp(t, "clip.png", small)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, kutilgan 413 (body=%v)", resp.StatusCode, body)
	}
	if got := errCode(body); got != "file_too_large" {
		t.Fatalf("xato kodi = %q, kutilgan file_too_large", got)
	}
}

func TestStorageUnavailableIs503(t *testing.T) {
	e := newTestEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	blocked := NewVaultClient([]string{"test-token"}, "-100123", time.Hour, 5*time.Second, time.Minute)
	blocked.nodes[0].block(time.Hour)
	if blocked.DemoMode() {
		t.Fatal("vault demo rejimda qoldi — test noto'g'ri")
	}

	st := NewStorageService(e.app.cfg, e.app.core, e.app.meta, blocked, e.app.gov, nil)
	st.StartWorkers(ctx)
	t.Cleanup(st.Close)
	e.app.storage = st

	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"song.m4a", sampleM4A()},
		{"song.mp3", sampleMP3()},
		{"song2.m4a", append(sampleM4A(), []byte("extra-payload")...)},
	} {
		resp, body := e.uploadResp(t, tc.name, tc.data)
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("%s: status = %d, kutilgan 503 (body=%v)", tc.name, resp.StatusCode, body)
		}
		if got := errCode(body); got != "storage_unavailable" {
			t.Fatalf("%s: xato kodi = %q, kutilgan storage_unavailable", tc.name, got)
		}
		if resp.Header.Get("Retry-After") == "" {
			t.Fatalf("%s: Retry-After header'i yo'q", tc.name)
		}
	}
}

func TestPerKindCeilingIs50MB(t *testing.T) {
	for _, kind := range []MediaKind{KindImage, KindAudio} {
		if got := kindCeilingKB(kind); got != 50*1024 {
			t.Fatalf("%s ceiling = %d KB, kutilgan 51200 KB", kind, got)
		}
	}
	for _, tier := range []Tier{TierFree, TierPro, TierBusiness} {
		p := DefaultPolicies[tier]
		for _, kind := range []MediaKind{KindImage, KindAudio} {
			if got := p.MaxBytesFor(kind); got != 50*1024*1024 {
				t.Fatalf("%s/%s limiti = %d, kutilgan 52428800", tier, kind, got)
			}
		}
	}
}

func TestM4AUploadsAndStreamsBack(t *testing.T) {
	e := newTestEnv(t)
	data := append(sampleM4A(), make([]byte, 128)...)
	code, body := e.upload(t, "qoshiq.m4a", data, map[string]string{"visibility": "public"})
	if code != http.StatusCreated {
		t.Fatalf("status = %d, kutilgan 201 (body=%v)", code, body)
	}
	im := imageFromResp(t, body)
	if im["kind"] != "audio" || im["mime_type"] != "audio/mp4" {
		t.Fatalf("kind/mime = %v/%v", im["kind"], im["mime_type"])
	}
	urls, _ := im["urls"].(map[string]any)
	resp, got := e.get(t, urls["cdn"].(string), false, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delivery status = %d", resp.StatusCode)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("baytlar o'zgargan (transkodlash bo'lmasligi kerak)")
	}
	if ra := resp.Header.Get("Accept-Ranges"); ra != "bytes" {
		t.Fatalf("accept-ranges = %q (pleyer seek qila olmaydi)", ra)
	}
}
