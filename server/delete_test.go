package main

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"
)

func TestDeleteRemovesDBAndVault(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	st, body := e.upload(t, "del.png", samplePNG(t, 40, 30), nil)
	if st != http.StatusCreated {
		t.Fatalf("upload status = %d, kutilgan 201", st)
	}
	im := imageFromResp(t, body)
	id, _ := im["id"].(string)
	if id == "" {
		t.Fatalf("id yo'q: %v", body)
	}

	var m *ImageMeta
	var err error
	dl := time.Now().Add(5 * time.Second)
	for {
		m, err = e.app.meta.GetImage(ctx, id)
		if err == nil || time.Now().After(dl) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GetImage: %v", err)
	}
	ref := m.VaultRef
	if ref == "" {
		t.Fatal("VaultRef bo'sh — obyekt bulutga yozilmagan")
	}

	if _, err := e.app.vault.Fetch(ctx, ref, 1<<20); err != nil {
		t.Fatalf("Delete'dan oldin obyekt o'qilmadi: %v", err)
	}

	before, err := e.app.core.GetUsage(ctx, e.userID)
	if err != nil {
		t.Fatalf("GetUsage: %v", err)
	}
	if before.FileCount < 1 {
		t.Fatalf("kvota hisoblanmagan: %+v", before)
	}

	req, _ := http.NewRequest(http.MethodDelete, e.srv.URL+"/v1/images/"+id, nil)
	req.Header.Set("X-API-Key", e.apiKey)
	dst, _ := e.do(t, req)
	if dst != http.StatusOK {
		t.Fatalf("DELETE status = %d", dst)
	}

	if _, err := e.app.meta.GetImage(ctx, id); err == nil {
		t.Fatal("DB yozuvi hali ham mavjud")
	}

	after, err := e.app.core.GetUsage(ctx, e.userID)
	if err != nil {
		t.Fatalf("GetUsage(2): %v", err)
	}
	if after.FileCount != before.FileCount-1 {
		t.Fatalf("kvota qaytmadi: before=%d after=%d", before.FileCount, after.FileCount)
	}
	if after.StorageBytes != before.StorageBytes-m.SizeBytes {
		t.Fatalf("hajm qaytmadi: before=%d after=%d size=%d",
			before.StorageBytes, after.StorageBytes, m.SizeBytes)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := e.app.vault.Fetch(ctx, ref, 1<<20); err != nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("obyekt Delete'dan keyin ham bulutda qoldi (orphan)")
}

func TestAnalyticsTimelineForFreeTier(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	if err := e.app.core.SaveSubscription(ctx, &Subscription{
		UserID: e.userID, Tier: TierFree, UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("SaveSubscription: %v", err)
	}
	if !DefaultPolicies[TierFree].Analytics {
		t.Fatal("Free tarifda Analytics ochiq bo'lishi kerak")
	}

	if st, _ := e.upload(t, "an.png", samplePNG(t, 20, 20), nil); st != http.StatusCreated {
		t.Fatalf("upload = %d, kutilgan 201", st)
	}

	resp, raw := e.get(t, "/v1/analytics?days=7", true, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Free tarif uchun analytics status = %d (kutilgan 200)", resp.StatusCode)
	}
	if !bytes.Contains(raw, []byte(`"timeline"`)) {
		t.Fatalf("javobda timeline yo'q: %s", raw)
	}
	if n := bytes.Count(raw, []byte(`"date"`)); n != 7 {
		t.Fatalf("timeline kunlari = %d, kutilgan 7", n)
	}
}
