package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func samplePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 3), uint8(y * 5), 128, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func sampleJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{200, 120, 40, 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func sampleGIF(t *testing.T, frames int) []byte {
	t.Helper()
	g := &gif.GIF{}
	for i := 0; i < frames; i++ {
		img := image.NewPaletted(image.Rect(0, 0, 8, 8), color.Palette{color.Black, color.White})
		g.Image = append(g.Image, img)
		g.Delay = append(g.Delay, 10)
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func sampleBMP(w, h int) []byte {
	row := ((w*3 + 3) / 4) * 4
	off := 54
	size := off + row*h
	b := make([]byte, size)
	copy(b[0:], "BM")
	binary.LittleEndian.PutUint32(b[2:], uint32(size))
	binary.LittleEndian.PutUint32(b[10:], uint32(off))
	binary.LittleEndian.PutUint32(b[14:], 40)
	binary.LittleEndian.PutUint32(b[18:], uint32(w))
	binary.LittleEndian.PutUint32(b[22:], uint32(h))
	binary.LittleEndian.PutUint16(b[26:], 1)
	binary.LittleEndian.PutUint16(b[28:], 24)
	for i := off; i < size; i++ {
		b[i] = byte(i)
	}
	return b
}

func sampleSVG() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<!-- izoh -->` + "\n" +
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 320 180">` +
		`<rect width="320" height="180" fill="#4f7cf7"/></svg>`)
}

func box(typ string, payload []byte) []byte {
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out[0:4], uint32(8+len(payload)))
	copy(out[4:8], typ)
	copy(out[8:], payload)
	return out
}

func sampleMP4() []byte {
	ftyp := box("ftyp", append([]byte("isom"), 0, 0, 0, 0, 'i', 's', 'o', 'm'))

	mvhd := make([]byte, 4+96)
	binary.BigEndian.PutUint32(mvhd[4+8:], 1000)
	binary.BigEndian.PutUint32(mvhd[4+12:], 6500)

	tkhd := make([]byte, 4+80)
	binary.BigEndian.PutUint32(tkhd[len(tkhd)-8:], 1280<<16)
	binary.BigEndian.PutUint32(tkhd[len(tkhd)-4:], 720<<16)

	moov := box("moov", append(box("mvhd", mvhd), box("trak", box("tkhd", tkhd))...))
	mdat := box("mdat", make([]byte, 64))
	return append(append(ftyp, moov...), mdat...)
}

func sampleWebM() []byte {
	vint := func(id uint64, idLen int) []byte {
		b := make([]byte, idLen)
		for i := idLen - 1; i >= 0; i-- {
			b[i] = byte(id & 0xff)
			id >>= 8
		}
		return b
	}
	size := func(n int) []byte { return []byte{byte(0x80 | n)} }

	elems := func(pairs ...[]byte) []byte {
		var out []byte
		for _, p := range pairs {
			out = append(out, p...)
		}
		return out
	}
	el := func(id uint64, idLen int, payload []byte) []byte {
		return append(append(vint(id, idLen), size(len(payload))...), payload...)
	}

	doctype := el(0x4282, 2, []byte("webm"))
	header := el(0x1A45DFA3, 4, elems(doctype, el(0x4286, 2, []byte{1}), el(0x42F7, 2, []byte{1})))

	timecodeScale := el(0x2AD7B1, 3, []byte{0x0F, 0x42, 0x40})
	duration := el(0x4489, 2, make([]byte, 4))
	binary.BigEndian.PutUint32(duration[len(duration)-4:], 0x461C4000)
	info := el(0x1549A966, 4, elems(timecodeScale, duration))

	pw := el(0xB0, 1, []byte{0x02, 0x80})
	ph := el(0xBA, 1, []byte{0x01, 0xE0})
	video := el(0xE0, 1, elems(pw, ph))
	track := el(0xAE, 1, video)
	tracks := el(0x1654AE6B, 4, track)

	segment := el(0x18538067, 4, elems(info, tracks))
	return append(header, segment...)
}

func sampleAVIF() []byte {
	ispe := make([]byte, 4+8)
	binary.BigEndian.PutUint32(ispe[4:], 640)
	binary.BigEndian.PutUint32(ispe[8:], 480)
	meta := box("meta", append([]byte{0, 0, 0, 0},
		box("iprp", box("ipco", box("ispe", ispe)))...))
	return append(box("ftyp", []byte("avifisom")), meta...)
}

func sampleMP3() []byte {
	frame := make([]byte, 417)
	frame[0], frame[1], frame[2], frame[3] = 0xFF, 0xFB, 0x90, 0x00
	out := make([]byte, 0, 417*6)
	for i := 0; i < 6; i++ {
		out = append(out, frame...)
	}
	return out
}

func sampleMP3ID3() []byte {
	tag := []byte("ID3\x03\x00\x00\x00\x00\x00\x00")
	return append(tag, sampleMP3()...)
}

func sampleFLAC() []byte {
	b := []byte("fLaC\x00\x00\x00\x22")
	b = append(b, make([]byte, 34)...)
	return b
}

func sampleWAV() []byte {
	b := []byte("RIFF")
	b = append(b, 0, 0, 0, 0)
	b = append(b, []byte("WAVEfmt ")...)
	b = append(b, 16, 0, 0, 0)
	b = append(b, 1, 0)
	b = append(b, 2, 0)
	b = append(b, 0x44, 0xAC, 0, 0)
	b = append(b, make([]byte, 8)...)
	b = append(b, []byte("data")...)
	b = append(b, 8, 0, 0, 0)
	b = append(b, make([]byte, 8)...)
	return b
}

func sampleOGG() []byte {
	payload := append([]byte("\x01vorbis"), make([]byte, 24)...)
	b := []byte("OggS")
	b = append(b, 0, 2)
	b = append(b, make([]byte, 8)...)
	b = append(b, make([]byte, 4)...)
	b = append(b, make([]byte, 4)...)
	b = append(b, make([]byte, 4)...)
	b = append(b, byte(len(payload)))
	b = append(b, byte(len(payload)))
	b = append(b, payload...)
	return b
}

func sampleOpus() []byte {
	payload := append([]byte("OpusHead"), make([]byte, 16)...)
	b := []byte("OggS")
	b = append(b, 0, 2)
	b = append(b, make([]byte, 20)...)
	b = append(b, byte(len(payload)), byte(len(payload)))
	b = append(b, payload...)
	return b
}

func sampleM4A() []byte {
	return box("ftyp", []byte("M4A isom"))
}

func sampleAIFF() []byte {
	b := []byte("FORM")
	b = append(b, 0, 0, 0, 0)
	b = append(b, []byte("AIFFCOMM")...)
	b = append(b, 18, 0, 0, 0)
	b = append(b, make([]byte, 18)...)
	return b
}

func sampleAMR() []byte {
	return append([]byte("#!AMR\n"), make([]byte, 32)...)
}

func TestSniffMedia(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		kind MediaKind
		mime string
	}{
		{"png", samplePNG(t, 20, 10), KindImage, "image/png"},
		{"jpeg", sampleJPEG(t, 20, 10), KindImage, "image/jpeg"},
		{"gif", sampleGIF(t, 2), KindImage, "image/gif"},
		{"bmp", sampleBMP(8, 6), KindImage, "image/bmp"},
		{"svg", sampleSVG(), KindImage, "image/svg+xml"},
		{"avif", sampleAVIF(), KindImage, "image/avif"},
		{"mp4", sampleMP4(), KindVideo, "video/mp4"},
		{"webm", sampleWebM(), KindVideo, "video/webm"},
		{"mov", append([]byte{0, 0, 0, 0x14, 'f', 't', 'y', 'p', 'q', 't', ' ', ' '}, make([]byte, 8)...), KindVideo, "video/quicktime"},
		{"heic", append([]byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c'}, make([]byte, 12)...), KindImage, "image/heic"},
		{"mkv", append([]byte{0x1A, 0x45, 0xDF, 0xA3, 0x93, 0x42, 0x82, 0x88, 'm', 'a', 't', 'r', 'o', 's', 'k', 'a'}, make([]byte, 8)...), KindVideo, "video/x-matroska"},
		{"avi", append([]byte("RIFF"), append([]byte{0, 0, 0, 0}, []byte("AVI ")...)...), KindVideo, "video/x-msvideo"},
		{"mp3", sampleMP3(), KindAudio, "audio/mpeg"},
		{"mp3-id3", sampleMP3ID3(), KindAudio, "audio/mpeg"},
		{"flac", sampleFLAC(), KindAudio, "audio/flac"},
		{"wav", sampleWAV(), KindAudio, "audio/wav"},
		{"ogg-vorbis", sampleOGG(), KindAudio, "audio/ogg"},
		{"opus", sampleOpus(), KindAudio, "audio/opus"},
		{"m4a", sampleM4A(), KindAudio, "audio/mp4"},
		{"aiff", sampleAIFF(), KindAudio, "audio/aiff"},
		{"amr", sampleAMR(), KindAudio, "audio/amr"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info, err := SniffMedia(c.data)
			if err != nil {
				t.Fatalf("SniffMedia() xato: %v", err)
			}
			if info.Kind != c.kind || info.Mime != c.mime {
				t.Fatalf("got %s/%s, want %s/%s", info.Kind, info.Mime, c.kind, c.mime)
			}
			if KindOfMime(info.Mime) != c.kind {
				t.Fatalf("KindOfMime(%s) != %s", info.Mime, c.kind)
			}
		})
	}
}

func TestSupportPolicy(t *testing.T) {
	ok := []struct {
		name string
		data []byte
	}{
		{"png", samplePNG(t, 10, 10)},
		{"jpeg", sampleJPEG(t, 10, 10)},
		{"webp", append([]byte{0x52, 0x49, 0x46, 0x46}, make([]byte, 4)...)},
		{"m4a", sampleM4A()},
		{"mp3", sampleMP3()},
		{"avif", sampleAVIF()},
		{"svg", sampleSVG()},
	}
	for _, c := range ok {
		t.Run(c.name, func(t *testing.T) {
			if c.name == "webp" {
				if !IsSupported(&MediaInfo{Kind: KindImage, Mime: "image/webp"}) {
					t.Fatal("webp qabul qilinmadi")
				}
				return
			}
			info, err := SniffMedia(c.data)
			if err != nil {
				t.Fatalf("SniffMedia: %v", err)
			}
			if !IsSupported(info) {
				t.Fatalf("%s support'da emas: %s", c.name, info.Mime)
			}
		})
	}

	bad := []struct {
		name string
		data []byte
	}{
		{"gif", sampleGIF(t, 1)},
		{"mp4-video", sampleMP4()},
		{"webm-video", sampleWebM()},
		{"wav", sampleWAV()},
		{"ogg", sampleOGG()},
		{"flac", sampleFLAC()},
	}
	for _, c := range bad {
		t.Run("reject/"+c.name, func(t *testing.T) {
			info, err := SniffMedia(c.data)
			if err != nil {
				t.Fatalf("SniffMedia: %v", err)
			}
			if IsSupported(info) {
				t.Fatalf("%s rad etilishi kerak edi (%s)", c.name, info.Mime)
			}
		})
	}
}

func TestSniffMP3WithJunkPrefix(t *testing.T) {
	raw := append([]byte{0x00, 0x00, 0x00, 0xFF, 0x00}, sampleMP3()...)
	info, err := SniffMedia(raw)
	if err != nil {
		t.Fatalf("junk-prefixed MP3 rad etildi: %v", err)
	}
	if info.Kind != KindAudio || info.Mime != "audio/mpeg" {
		t.Fatalf("got %s/%s, want audio/mpeg", info.Kind, info.Mime)
	}
	if !IsSupported(info) {
		t.Fatal("junk-prefixed MP3 support'da emas")
	}
}

func TestSniffMediaRejectsNonMedia(t *testing.T) {
	bad := [][]byte{
		[]byte("oddiy matn fayli, hech qanday media emas"),
		append([]byte("%PDF-1.7\n"), make([]byte, 64)...),
		[]byte{0x7F, 'E', 'L', 'F', 2, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 1, 2},
	}
	for i, b := range bad {
		if _, err := SniffMedia(b); err == nil {
			t.Fatalf("%d-holat: noto'g'ri fayl qabul qilindi", i)
		}
	}
}

func TestSniffSVGRequiresSVGRoot(t *testing.T) {
	html := []byte(`<!DOCTYPE html><html><body><img src="x.svg"></body></html>`)
	if _, err := SniffMedia(html); err == nil {
		t.Fatal("HTML fayl SVG sifatida qabul qilindi")
	}
}

func TestSVGDimensions(t *testing.T) {
	w, h := svgDimensions(sampleSVG())
	if w != 320 || h != 180 {
		t.Fatalf("viewBox o'lchami noto'g'ri: %dx%d (320x180 kutilgan)", w, h)
	}
	w2, h2 := svgDimensions([]byte(`<svg width="120px" height="80px" xmlns="http://www.w3.org/2000/svg"></svg>`))
	if w2 != 120 || h2 != 80 {
		t.Fatalf("width/height noto'g'ri: %dx%d", w2, h2)
	}
}

func TestProbeISOBMFF(t *testing.T) {
	w, h, dur := ProbeMedia(sampleMP4(), "video/mp4")
	if w != 1280 || h != 720 {
		t.Fatalf("MP4 o'lchami: got %dx%d, want 1280x720", w, h)
	}
	if dur < 6.4 || dur > 6.6 {
		t.Fatalf("MP4 davomiylik: got %v, want ~6.5s", dur)
	}
}

func TestProbeAVIFDimensions(t *testing.T) {
	w, h, _ := ProbeMedia(sampleAVIF(), "image/avif")
	if w != 640 || h != 480 {
		t.Fatalf("AVIF o'lchami: got %dx%d, want 640x480", w, h)
	}
}

func TestProbeAudioDuration(t *testing.T) {
	if _, _, dur := ProbeMedia(sampleMP3(), "audio/mpeg"); dur <= 0 || dur > 0.2 {
		t.Fatalf("MP3 davomiylik: got %v, want ~0.157", dur)
	}
	if _, _, dur := ProbeMedia(sampleMP3ID3(), "audio/mpeg"); dur <= 0 || dur > 0.2 {
		t.Fatalf("MP3(ID3) davomiylik: got %v, want ~0.157", dur)
	}
}

func TestProbeMediaOnGarbageDoesNotPanic(t *testing.T) {
	for _, mime := range []string{"video/mp4", "video/webm", "image/avif"} {
		for n := 0; n < 40; n++ {
			junk := bytes.Repeat([]byte{byte(n*7 + 1)}, 8+n)
			ProbeMedia(junk, mime)
		}
	}
}

func TestProcessImageFastProducesBg(t *testing.T) {
	res, err := ProcessImageFast(samplePNG(t, 640, 400))
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindImage || res.MimeType != "image/png" {
		t.Fatalf("got %s/%s", res.Kind, res.MimeType)
	}
	if res.Width != 640 || res.Height != 400 {
		t.Fatalf("o'lcham: %dx%d", res.Width, res.Height)
	}
	if !validBg(res.Bg) {
		t.Fatalf("bg rang yo'q yoki noto'g'ri: %q", res.Bg)
	}
	if res.Checksum == "" || len(res.HD) == 0 {
		t.Fatal("HD/checksum bo'sh")
	}
}

func TestDominantColorSolidPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{200, 120, 40, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	res, err := ProcessImageFast(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if res.Bg != "#c87828" {
		t.Fatalf("bg = %q, want #c87828", res.Bg)
	}
}

func TestShortMediaID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 400; i++ {
		id := shortID(mediaIDLen)
		if len(id) != 8 || !validMediaID(id) {
			t.Fatalf("yaroqsiz id: %q", id)
		}
		if seen[id] {
			t.Fatalf("to'qnashuv: %q", id)
		}
		seen[id] = true
	}
	if !validMediaID("img_8f3a2c91d40b6e7a") {
		t.Fatal("eski img_ id rad etildi")
	}
	if validMediaID("abc") || validMediaID("bad id!") || validMediaID("") {
		t.Fatal("yaroqsiz id qabul qilindi")
	}
}

func TestProcessImageFastVideoKeepsBytes(t *testing.T) {
	raw := sampleMP4()
	res, err := ProcessImageFast(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsVideo() {
		t.Fatal("video aniqlanmadi")
	}
	if !bytes.Equal(res.HD, raw) {
		t.Fatal("video baytlari o'zgargan")
	}
	if res.Bg != "" {
		t.Fatal("video uchun bg bo'sh bo'lishi kerak")
	}
	if res.Width != 1280 || res.Height != 720 || res.Duration == 0 {
		t.Fatalf("video metadata: %dx%d %vs", res.Width, res.Height, res.Duration)
	}
}

func TestProcessImageFastSVGKeepsBytes(t *testing.T) {
	raw := sampleSVG()
	res, err := ProcessImageFast(raw)
	if err != nil {
		t.Fatal(err)
	}
	if res.MimeType != "image/svg+xml" {
		t.Fatalf("got %s", res.MimeType)
	}
	if !bytes.Equal(res.HD, raw) {
		t.Fatal("SVG baytlari o'zgargan")
	}
	if res.Width != 320 || res.Height != 180 {
		t.Fatalf("SVG o'lchami: %dx%d", res.Width, res.Height)
	}
	if res.Bg != "#4f7cf7" {
		t.Fatalf("SVG bg = %q, want #4f7cf7", res.Bg)
	}
}

func TestProcessImageHDNeverTranscodesVideoOrSVG(t *testing.T) {
	if out, mime, err := ProcessImageHD(sampleMP4(), "video/mp4", 4000, 1920, 82); err != nil || out != nil || mime != "" {
		t.Fatalf("video qayta kodlandi: %v %v %v", out, mime, err)
	}
	if out, mime, err := ProcessImageHD(sampleSVG(), "image/svg+xml", 4000, 1920, 82); err != nil || out != nil || mime != "" {
		t.Fatalf("SVG qayta kodlandi: %v %v %v", out, mime, err)
	}
}

func TestProcessImageHDOptimizesSmallJPEG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 640, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 640; x++ {
			img.Set(x, y, color.RGBA{uint8((x * 7) % 256), uint8((y * 3) % 256), uint8((x + y) % 256), 255})
		}
	}
	var src bytes.Buffer
	if err := jpeg.Encode(&src, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	raw := src.Bytes()
	out, mime, err := ProcessImageHD(raw, "image/jpeg", 640, 1600, 82)
	if err != nil {
		t.Fatalf("HD: %v", err)
	}
	if out == nil || mime != "image/jpeg" {
		t.Fatalf("kichik JPEG optimallashtirilmadi: out=%v mime=%q", out == nil, mime)
	}
	if len(out) >= len(raw) {
		t.Fatalf("HD asl fayldan kichik emas: %d >= %d", len(out), len(raw))
	}
}

func TestParseRange(t *testing.T) {
	const size = 100
	ok := []struct {
		hdr    string
		lo, hi int64
	}{
		{"bytes=0-49", 0, 49},
		{"bytes=50-", 50, 99},
		{"bytes=-20", 80, 99},
		{"bytes=0-999", 0, 99},
		{"bytes=99-99", 99, 99},
		{"bytes= 10 - 20 ", 10, 20},
		{"bytes=0-10, 20-30", 0, 10},
	}
	for _, c := range ok {
		lo, hi, valid := parseRange(c.hdr, size)
		if !valid || lo != c.lo || hi != c.hi {
			t.Fatalf("parseRange(%q) = %d,%d,%v — want %d,%d,true", c.hdr, lo, hi, valid, c.lo, c.hi)
		}
	}
	bad := []string{"", "bytes=", "bytes=-", "bytes=abc-def", "bytes=100-", "bytes=200-300", "items=0-1", "bytes=-0"}
	for _, h := range bad {
		if _, _, valid := parseRange(h, size); valid {
			t.Fatalf("parseRange(%q) qabul qilindi, rad etilishi kerak", h)
		}
	}
}

func TestListFilterMatch(t *testing.T) {
	img := &ImageMeta{ID: "img_1", Filename: "photo.PNG", MimeType: "image/png", Kind: "image", Visibility: "public"}
	vid := &ImageMeta{ID: "img_2", Filename: "clip.mp4", MimeType: "video/mp4", Kind: "video", Visibility: "private"}
	gifm := &ImageMeta{ID: "img_3", Filename: "anim.gif", MimeType: "image/gif", Kind: "image", Visibility: "public"}
	svg := &ImageMeta{ID: "img_4", Filename: "logo.svg", MimeType: "image/svg+xml", Kind: "image", Visibility: "private"}
	legacy := &ImageMeta{ID: "img_5", Filename: "old.jpg", MimeType: "image/jpeg", Visibility: "private"}

	cases := []struct {
		f    ListFilter
		want int
	}{
		{ListFilter{}, 5},
		{ListFilter{Kind: "image"}, 4},
		{ListFilter{Kind: "gif"}, 1},
		{ListFilter{Kind: "svg"}, 1},
		{ListFilter{Visibility: "private"}, 3},
		{ListFilter{Visibility: "public"}, 2},
		{ListFilter{Q: "PHOTO"}, 1},
		{ListFilter{Q: "clip"}, 1},
		{ListFilter{Q: "nomavjud"}, 0},
	}
	all := []*ImageMeta{img, vid, gifm, svg, legacy}
	for _, c := range cases {
		got := 0
		for _, m := range all {
			if c.f.Match(m) {
				got++
			}
		}
		if got != c.want {
			t.Fatalf("filter %+v: got %d, want %d", c.f, got, c.want)
		}
	}
	if !(ListFilter{Sort: "views"}.Active()) {
		t.Fatal("sort filtersiz Active() false")
	}
	if (ListFilter{Sort: "newest"}.Active()) {
		t.Fatal("standart saralash Active() true bo'lmasligi kerak")
	}
}

func TestPublicImageKindAndDuration(t *testing.T) {
	m := &ImageMeta{
		ID: "img_x", Filename: "clip.mp4", MimeType: "video/mp4",
		Kind: "video", Duration: 65.4, Visibility: "private",
	}
	p := m.Public("")
	if p.Kind != "video" || p.DurationFmt != "1:05" {
		t.Fatalf("got kind=%s dur=%s", p.Kind, p.DurationFmt)
	}
	if p.URLs.Download != "/d/img_x" || p.URLs.CDN != "/cdn/img_x" {
		t.Fatalf("havolalar noto'g'ri: %+v", p.URLs)
	}
	legacy := (&ImageMeta{ID: "img_y", MimeType: "image/jpeg"}).Public("")
	if legacy.Kind != "image" || legacy.Visibility != "private" {
		t.Fatalf("legacy: %+v", legacy)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[float64]string{0: "", 5: "0:05", 6.9: "0:06", 65: "1:05", 3600: "1:00:00", 3661.6: "1:01:01"}
	for in, want := range cases {
		if got := FormatDuration(in); got != want {
			t.Fatalf("FormatDuration(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	if got := humanBytes(10 * 1024 * 1024); !strings.Contains(got, "MB") {
		t.Fatalf("humanBytes = %q", got)
	}
	if got := humanBytes(0); got != "0 B" {
		t.Fatalf("humanBytes(0) = %q", got)
	}
}
