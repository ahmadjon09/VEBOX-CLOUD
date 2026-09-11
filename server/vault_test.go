package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestVaultDeleteRemovesObject(t *testing.T) {
	v := NewVaultClient(nil, "", time.Second, 5*time.Second, time.Minute)
	if !v.DemoMode() {
		t.Skip("haqiqiy vault sozlangan — demo testi o'tkazildi")
	}
	ctx := context.Background()

	ref, err := v.Put(ctx, "test.png", []byte("obyekt mazmuni"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got, err := v.Fetch(ctx, ref, 1<<20); err != nil || string(got) != "obyekt mazmuni" {
		t.Fatalf("Fetch: got %q err=%v", got, err)
	}

	if err := v.Delete(ctx, ref); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := v.Fetch(ctx, ref, 1<<20); err == nil {
		t.Fatal("Delete'dan keyin obyekt hali ham o'qilmoqda")
	}
	if err := v.Delete(ctx, ref); err != nil {
		t.Fatalf("ikkinchi Delete: %v", err)
	}
	if err := v.Delete(ctx, ""); err != nil {
		t.Fatalf("bo'sh referens: %v", err)
	}
}

func TestRefParts(t *testing.T) {
	cases := []struct {
		ref, fileID string
		msgID       int64
	}{
		{"12345#AgACAgIAAxkBAAI", "AgACAgIAAxkBAAI", 12345},
		{"AgACAgIAAxkBAAI", "AgACAgIAAxkBAAI", 0},
		{"demo:test.png:abc", "demo:test.png:abc", 0},
		{"#AgAC", "AgAC", 0},
		{"", "", 0},
	}
	for _, c := range cases {
		f, m := refParts(c.ref)
		if f != c.fileID || m != c.msgID {
			t.Fatalf("refParts(%q) = %q,%d; want %q,%d", c.ref, f, m, c.fileID, c.msgID)
		}
	}
	if got := makeRef(0, "abc"); got != "abc" {
		t.Fatalf("makeRef(0) = %q", got)
	}
	if got := makeRef(77, "abc"); got != "77#abc" {
		t.Fatalf("makeRef(77) = %q", got)
	}
}

func TestFileIDFromTelegramAudioMessage(t *testing.T) {
	cases := []struct {
		name, raw, want string
	}{
		{"document", `{"message_id":1,"document":{"file_id":"DOCID","file_size":10}}`, "DOCID"},
		{"audio-mp3", `{"message_id":2,"audio":{"file_id":"AUDIOID","duration":12,"mime_type":"audio/mpeg","file_name":"song.mp3"}}`, "AUDIOID"},
		{"voice", `{"message_id":3,"voice":{"file_id":"VOICEID"}}`, "VOICEID"},
		{"photo", `{"message_id":4,"photo":[{"file_id":"P1"},{"file_id":"P2"}]}`, "P2"},
		{"empty", `{"message_id":5}`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var msg remoteMsg
			if err := json.Unmarshal([]byte(c.raw), &msg); err != nil {
				t.Fatal(err)
			}
			if got := fileIDFromMsg(msg); got != c.want {
				t.Fatalf("fileIDFromMsg = %q, want %q", got, c.want)
			}
		})
	}
}
