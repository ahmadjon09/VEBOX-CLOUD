package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"net/http"
	"strings"
)

type MediaKind string

const (
	KindImage   MediaKind = "image"
	KindAudio   MediaKind = "audio"
	KindVideo   MediaKind = "video"
	KindUnknown MediaKind = ""
)

const (
	MimeJPEG = "image/jpeg"
	MimePNG  = "image/png"
	MimeGIF  = "image/gif"
	MimeWebP = "image/webp"
	MimeBMP  = "image/bmp"
	MimeTIFF = "image/tiff"
	MimeICO  = "image/vnd.microsoft.icon"
	MimeSVG  = "image/svg+xml"
	MimeAVIF = "image/avif"
	MimeHEIC = "image/heic"
	MimeMP3  = "audio/mpeg"
	MimeM4A  = "audio/mp4"
)

var supportedImageMimes = map[string]bool{
	MimeJPEG: true,
	MimePNG:  true,
	MimeWebP: true,
	MimeBMP:  true,
	MimeTIFF: true,
	MimeICO:  true,
	MimeSVG:  true,
	MimeAVIF: true,
	MimeHEIC: true,
}

var supportedAudioMimes = map[string]bool{
	MimeM4A: true,
	MimeMP3: true,
}

var ErrUnsupportedMedia = errors.New("unsupported media format")

func SupportedFormats() map[string][]string {
	return map[string][]string{
		"image": {"JPEG", "PNG", "WebP", "BMP", "TIFF", "ICO", "SVG", "AVIF", "HEIC/HEIF"},
		"audio": {"M4A", "MP3"},
	}
}

func SupportedFormatsText() string {
	return "JPEG, PNG, WebP, BMP, TIFF, ICO, SVG, AVIF, HEIC (rasm) · " +
		"M4A/MP3 (audio). GIF va video qo'llab-quvvatlanmaydi"
}

func KindOfMime(m string) MediaKind {
	switch {
	case strings.HasPrefix(m, "image/"):
		return KindImage
	case strings.HasPrefix(m, "audio/"):
		return KindAudio
	case strings.HasPrefix(m, "video/"):
		return KindVideo
	}
	return KindUnknown
}

func IsImageMime(m string) bool { return KindOfMime(m) == KindImage }
func IsAudioMime(m string) bool { return KindOfMime(m) == KindAudio }
func IsVideoMime(m string) bool { return KindOfMime(m) == KindVideo }

func IsSupported(info *MediaInfo) bool {
	switch info.Kind {
	case KindImage:
		return supportedImageMimes[info.Mime]
	case KindAudio:
		return supportedAudioMimes[info.Mime]
	}
	return false
}

var extForMime = map[string]string{
	MimeJPEG:           "jpg",
	MimePNG:            "png",
	MimeGIF:            "gif",
	MimeWebP:           "webp",
	MimeBMP:            "bmp",
	MimeTIFF:           "tiff",
	MimeICO:            "ico",
	MimeSVG:            "svg",
	MimeAVIF:           "avif",
	MimeHEIC:           "heic",
	MimeMP3:            "mp3",
	MimeM4A:            "m4a",
	"video/mp4":        "mp4",
	"video/quicktime":  "mov",
	"video/webm":       "webm",
	"video/x-matroska": "mkv",
	"video/x-msvideo":  "avi",
	"video/3gpp":       "3gp",
}

func ExtForMime(m string) string {
	if e, ok := extForMime[m]; ok {
		return e
	}
	return "bin"
}

func EnsureExt(name, mime string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "file"
	}
	if strings.LastIndexByte(name, '.') > 0 {
		return name
	}
	if ext := ExtForMime(mime); ext != "bin" {
		return name + "." + ext
	}
	return name
}

type MediaInfo struct {
	Kind     MediaKind
	Mime     string
	Width    int
	Height   int
	Duration float64
}

func sniffAudio(b []byte) string {
	switch {
	case string(b[0:4]) == "fLaC":
		return "audio/flac"
	case string(b[0:4]) == "OggS":
		head := b
		if len(head) > 4096 {
			head = head[:4096]
		}
		if bytes.Contains(head, []byte("OpusHead")) {
			return "audio/opus"
		}
		return "audio/ogg"
	case bytes.HasPrefix(b, []byte("#!AMR")):
		return "audio/amr"
	case string(b[0:4]) == "FORM" && (string(b[8:12]) == "AIFF" || string(b[8:12]) == "AIFC"):
		return "audio/aiff"
	}
	return ""
}

func SniffMedia(b []byte) (*MediaInfo, error) {
	if len(b) < 12 {
		return nil, ErrUnsupportedMedia
	}
	info := &MediaInfo{}

	if string(b[0:4]) == "RIFF" {
		switch string(b[8:12]) {
		case "WAVE":
			info.Kind, info.Mime = KindAudio, "audio/wav"
		case "AVI ":
			info.Kind, info.Mime = KindVideo, "video/x-msvideo"
		}
	}

	if info.Kind == KindUnknown && string(b[4:8]) == "ftyp" {
		switch brand := string(b[8:12]); brand {
		case "avif", "avis":
			info.Kind, info.Mime = KindImage, MimeAVIF
		case "heic", "heix", "hevc", "hevx", "heim", "heis", "mif1", "msf1":
			info.Kind, info.Mime = KindImage, MimeHEIC
		case "M4A ", "M4B ", "mp4a":
			info.Kind, info.Mime = KindAudio, MimeM4A
		case "qt  ":
			info.Kind, info.Mime = KindVideo, "video/quicktime"
		case "3gp4", "3gp5", "3gp6", "3ge6", "3gg6", "3gf6":
			info.Kind, info.Mime = KindVideo, "video/3gpp"
		case "mp41", "mp42", "mp4v", "isom", "iso2", "iso3", "iso4", "iso5", "iso6",
			"iso7", "iso8", "avc1", "dash", "M4V ", "mmp4":
			info.Kind, info.Mime = KindVideo, "video/mp4"
		}
	}

	if info.Kind == KindUnknown && binary.BigEndian.Uint32(b[0:4]) == 0x1A45DFA3 {
		head := b
		if len(head) > 4096 {
			head = head[:4096]
		}
		if bytes.Contains(head, []byte("matroska")) {
			info.Kind, info.Mime = KindVideo, "video/x-matroska"
		} else {
			info.Kind, info.Mime = KindVideo, "video/webm"
		}
	}

	if info.Kind == KindUnknown {
		if m := sniffAudio(b); m != "" {
			info.Kind, info.Mime = KindAudio, m
		}
	}

	if info.Kind == KindUnknown && string(b[0:3]) == "ID3" {
		info.Kind, info.Mime = KindAudio, MimeMP3
	}

	if info.Kind == KindUnknown && mp3ChainAt(b, mp3Start(b)) {
		info.Kind, info.Mime = KindAudio, MimeMP3
	}

	if info.Kind == KindUnknown && mp3Lookahead(b) {
		info.Kind, info.Mime = KindAudio, MimeMP3
	}

	if info.Kind == KindUnknown {
		mt, _, _ := strings.Cut(http.DetectContentType(b), ";")
		mt = strings.TrimSpace(mt)
		switch KindOfMime(mt) {
		case KindImage, KindAudio, KindVideo:
			info.Kind, info.Mime = KindOfMime(mt), mt
		}
	}

	if info.Kind == KindUnknown && looksLikeSVG(b) {
		info.Kind, info.Mime = KindImage, MimeSVG
	}

	if info.Kind == KindUnknown {
		return nil, ErrUnsupportedMedia
	}
	return info, nil
}

func mp3Start(b []byte) int {
	if len(b) > 10 && string(b[0:3]) == "ID3" {
		size := 0
		if b[3] == 4 {
			size = int(b[6])<<21 | int(b[7])<<14 | int(b[8])<<7 | int(b[9])
		} else if b[3] == 2 {
			size = int(b[6])<<16 | int(b[7])<<8 | int(b[8])
		}
		off := 10 + size
		if b[5]&0x10 != 0 {
			off += 10
		}
		if off > 0 && off < len(b) {
			return off
		}
	}
	return 0
}

func mp3ChainAt(b []byte, off int) bool {
	f := mp3FrameAt(b, off)
	if !f.ok {
		return false
	}
	for i := 0; i < 2; i++ {
		off += f.len
		next := mp3FrameAt(b, off)
		if !next.ok || next.version != f.version || next.layer != f.layer || next.rate != f.rate {
			return false
		}
		f = next
	}
	return true
}

func mp3Lookahead(b []byte) bool {
	if b[0] == 0xFF && b[1] == 0xD8 {
		return false
	}
	limit := len(b)
	if limit > 512 {
		limit = 512
	}
	for i := 1; i+4 < limit; i++ {
		if b[i] == 0xFF && b[i+1]&0xE0 == 0xE0 && mp3ChainAt(b, i) {
			return true
		}
	}
	return false
}

func looksLikeSVG(b []byte) bool {
	head := b
	if len(head) > 4096 {
		head = head[:4096]
	}
	if !bytes.Contains(bytes.ToLower(head), []byte("<svg")) {
		return false
	}
	s := strings.ToLower(string(head))
	i := strings.Index(s, "<svg")
	if i < 0 {
		return false
	}
	pre := s[:i]
	for {
		j := strings.IndexByte(pre, '<')
		if j < 0 {
			break
		}
		rest := strings.TrimSpace(pre[j:])
		switch {
		case strings.HasPrefix(rest, "<?"):
			if k := strings.Index(rest, "?>"); k >= 0 {
				pre = rest[k+2:]
				continue
			}
		case strings.HasPrefix(rest, "<!"):
			if k := strings.Index(rest, ">"); k >= 0 {
				pre = rest[k+1:]
				continue
			}
		}
		return false
	}
	return true
}

func ProbeMedia(raw []byte, mime string) (width, height int, duration float64) {
	switch mime {
	case MimeM4A, "video/mp4", "video/quicktime", "video/3gpp", MimeAVIF, MimeHEIC:
		return probeISOBMFF(raw)
	case MimeMP3:
		return 0, 0, probeMP3(raw)
	}
	return 0, 0, 0
}

func probeISOBMFF(b []byte) (int, int, float64) {
	var (
		width, height int
		duration      float64
		bestArea      float64
	)
	walkBoxes(b, "", 0, 4, func(path string, body []byte) bool {
		switch {
		case path == "moov/mvhd" && len(body) >= 20:
			if ts, dur, ok := readMvhd(body); ok {
				duration = dur / ts
			}
		case path == "moov/trak/tkhd" && len(body) >= 84:
			w := float64(binary.BigEndian.Uint32(body[len(body)-8:])) / 65536
			h := float64(binary.BigEndian.Uint32(body[len(body)-4:])) / 65536
			if area := w * h; area > bestArea {
				bestArea, width, height = area, int(w+0.5), int(h+0.5)
			}
		case strings.HasSuffix(path, "/ispe") && len(body) >= 12:
			w := binary.BigEndian.Uint32(body[4:8])
			h := binary.BigEndian.Uint32(body[8:12])
			if w > 0 && h > 0 && float64(w*h) > bestArea {
				bestArea, width, height = float64(w*h), int(w), int(h)
			}
		}
		return true
	})
	return width, height, math.Round(duration*1000) / 1000
}

func readMvhd(body []byte) (timescale, duration float64, ok bool) {
	if body[0] == 1 {
		if len(body) < 32 {
			return 0, 0, false
		}
		ts := binary.BigEndian.Uint32(body[20:24])
		du := binary.BigEndian.Uint64(body[24:32])
		if ts == 0 {
			return 0, 0, false
		}
		return float64(ts), float64(du), true
	}
	ts := binary.BigEndian.Uint32(body[12:16])
	du := binary.BigEndian.Uint32(body[16:20])
	if ts == 0 {
		return 0, 0, false
	}
	return float64(ts), float64(du), true
}

func walkBoxes(b []byte, prefix string, depth, maxDepth int, visit func(path string, body []byte) bool) {
	if depth > maxDepth {
		return
	}
	for off := 0; off+8 <= len(b); {
		size := uint64(binary.BigEndian.Uint32(b[off : off+4]))
		typ := string(b[off+4 : off+8])
		hdr := 8
		if size == 1 {
			if off+16 > len(b) {
				return
			}
			size = binary.BigEndian.Uint64(b[off+8 : off+16])
			hdr = 16
		} else if size == 0 {
			size = uint64(len(b) - off)
		}
		if size < uint64(hdr) || uint64(off)+size > uint64(len(b)) {
			return
		}
		body := b[off+hdr : uint64(off)+size]
		if isContainerBox(typ) {
			visit(prefix+typ, body)
			inner := body
			if typ == "meta" && len(inner) > 4 {
				inner = inner[4:]
			}
			walkBoxes(inner, prefix+typ+"/", depth+1, maxDepth, visit)
		} else {
			visit(prefix+typ, body)
		}
		off += int(size)
	}
}

func isContainerBox(t string) bool {
	switch t {
	case "moov", "trak", "mdia", "minf", "stbl", "edts", "udta", "dinf",
		"meta", "iprp", "ipco", "iref":
		return true
	}
	return false
}

type mp3FrameInfo struct {
	version int
	layer   int
	rate    int
	samples int
	len     int
	mono    bool
	ok      bool
}

var mp3Bitrates = [2][3][16]int{
	{
		{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, 0},
		{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0},
		{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0},
	},
	{
		{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, 0},
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
	},
}

var mp3Rates = map[int][3]int{
	3: {44100, 48000, 32000},
	2: {22050, 24000, 16000},
	0: {11025, 12000, 8000},
}

func mp3FrameAt(b []byte, off int) mp3FrameInfo {
	var f mp3FrameInfo
	if off < 0 || off+4 > len(b) {
		return f
	}
	if b[off] != 0xFF || b[off+1]&0xE0 != 0xE0 {
		return f
	}
	h1, h2, h3 := b[off+1], b[off+2], b[off+3]
	vcode := int(h1>>3) & 0x03
	layerCode := int(h1>>1) & 0x03
	if vcode == 1 || layerCode == 0 {
		return f
	}
	version := 1
	switch vcode {
	case 3:
		version = 1
	case 2:
		version = 2
	case 0:
		version = 25
	}
	rates, ok := mp3Rates[vcode]
	if !ok {
		return f
	}
	brIdx := int(h2>>4) & 0x0F
	srIdx := int(h2>>2) & 0x03
	if brIdx == 0 || brIdx == 15 || srIdx == 3 {
		return f
	}
	layer := 4 - layerCode
	bitrate := mp3Bitrates[0][layer-1][brIdx]
	if version != 1 {
		bitrate = mp3Bitrates[1][layer-1][brIdx]
	}
	rate := rates[srIdx]
	if bitrate <= 0 || rate <= 0 {
		return f
	}
	samples := 1152
	switch {
	case layer == 1:
		samples = 384
	case layer == 3 && version != 1:
		samples = 576
	}
	pad := int((h2 >> 1) & 0x01)
	padBytes := pad
	if layer == 1 {
		padBytes = pad * 4
	}
	if h1&0x01 == 0 {
		padBytes += 2
	}
	// (samples/8)*bitrate*1000/rate already accounts for the 4-byte header —
	// adding another 4 breaks frame chaining and MP3s get rejected as unknown.
	frameLen := (samples/8)*bitrate*1000/rate + padBytes
	if frameLen <= 4 {
		return f
	}
	f.version, f.layer, f.rate, f.samples = version, layer, rate, samples
	f.len = frameLen
	f.mono = (h3>>6)&0x03 == 3
	f.ok = true
	return f
}

func probeMP3(b []byte) float64 {
	off := mp3Start(b)
	first := mp3FrameAt(b, off)
	if !first.ok {
		return 0
	}
	sideInfo := 17
	if first.version == 1 {
		sideInfo = 32
	}
	if first.mono {
		sideInfo = sideInfo/2 + 1
	}
	x := off + 4 + sideInfo
	if x+12 <= len(b) {
		tag := string(b[x : x+4])
		if tag == "Xing" || tag == "Info" {
			flags := binary.BigEndian.Uint32(b[x+4 : x+8])
			if flags&0x01 != 0 {
				frames := binary.BigEndian.Uint32(b[x+8 : x+12])
				if frames > 0 {
					return round3(float64(frames) * float64(first.samples) / float64(first.rate))
				}
			}
		}
	}
	if vb := probeMP3VBRI(b, off); vb > 0 {
		return vb
	}
	const maxFrames = 400000
	frames, p := 0, off
	var total int64
	for p+4 <= len(b) && frames < maxFrames {
		f := mp3FrameAt(b, p)
		if !f.ok {
			break
		}
		frames++
		total += int64(f.len)
		p += f.len
	}
	if frames == 0 {
		return 0
	}
	if p+4 >= len(b) {
		return round3(float64(frames) * float64(first.samples) / float64(first.rate))
	}
	avg := float64(total) / float64(frames)
	if avg <= 0 {
		return 0
	}
	est := float64(len(b)-off) / avg
	return round3(est * float64(first.samples) / float64(first.rate))
}

func probeMP3VBRI(b []byte, off int) float64 {
	first := mp3FrameAt(b, off)
	if !first.ok {
		return 0
	}
	x := off + 4 + first.len
	if x+24 > len(b) {
		return 0
	}
	if string(b[x:x+4]) != "VBRI" {
		return 0
	}
	ver := binary.BigEndian.Uint16(b[x+4 : x+6])
	if ver != 1 {
		return 0
	}
	ofs := binary.BigEndian.Uint16(b[x+10 : x+12])
	if int(x)+12+int(ofs)+12 > len(b) {
		return 0
	}
	p := x + 12 + int(ofs)
	if binary.BigEndian.Uint16(b[p:p+2]) != 2 {
		return 0
	}
	frames := binary.BigEndian.Uint32(b[p+6 : p+10])
	if frames == 0 {
		return 0
	}
	return round3(float64(frames) * float64(first.samples) / float64(first.rate))
}

func round3(f float64) float64 { return math.Round(f*1000) / 1000 }
