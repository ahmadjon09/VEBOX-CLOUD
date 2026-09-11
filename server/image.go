package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

var ErrUnsupportedFormat = errors.New("unsupported media format")

var paletteSize = 4

func setPaletteSize(n int) {
	if n < 1 {
		return
	}
	if n > 8 {
		n = 8
	}
	paletteSize = n
}

type ColorInfo struct {
	Hex string  `json:"hex" bson:"hex"`
	Pct float64 `json:"pct" bson:"pct"`
}

type ProcessedImage struct {
	Original []byte
	HD       []byte
	Width    int
	Height   int
	Duration float64
	MimeType string
	Kind     MediaKind
	Checksum string
	Bg       string
	Colors   []ColorInfo
}

func (p *ProcessedImage) IsAudio() bool { return p.Kind == KindAudio }

func (p *ProcessedImage) IsVideo() bool { return p.Kind == KindVideo }

func DetectMedia(b []byte) (*MediaInfo, error) {
	info, err := SniffMedia(b)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, http.DetectContentType(b))
	}
	return info, nil
}

func ProcessImageFast(raw []byte) (*ProcessedImage, error) {
	info, err := DetectMedia(raw)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)

	res := &ProcessedImage{
		Original: raw,
		HD:       raw,
		MimeType: info.Mime,
		Kind:     info.Kind,
		Checksum: hex.EncodeToString(sum[:]),
	}

	if info.Kind == KindAudio || info.Kind == KindVideo {
		res.Width, res.Height, res.Duration = ProbeMedia(raw, info.Mime)
		return res, nil
	}

	src, _, derr := image.Decode(bytes.NewReader(raw))
	if derr != nil {
		res.Width, res.Height, _ = ProbeMedia(raw, info.Mime)
		if res.Width == 0 && info.Mime == MimeSVG {
			res.Width, res.Height = svgDimensions(raw)
		}
		if info.Mime == MimeSVG {
			res.Bg, res.Colors = svgPalette(raw)
		}
		return res, nil
	}
	b := src.Bounds()
	res.Width, res.Height = b.Dx(), b.Dy()
	res.Bg, res.Colors = analyzeColors(src)
	return res, nil
}

func ProcessImageHD(raw []byte, mime string, width, hdMaxWidth, hdQuality int) ([]byte, string, error) {
	switch mime {
	case MimeJPEG, MimePNG:
	default:
		return nil, "", nil
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", err
	}
	if hdMaxWidth > 0 && width > hdMaxWidth {
		src = imaging.Resize(src, hdMaxWidth, 0, imaging.Lanczos)
	}
	var hbuf bytes.Buffer
	switch mime {
	case MimePNG:
		if err := png.Encode(&hbuf, src); err != nil {
			return nil, "", err
		}
	default:
		q := hdQuality
		if q <= 0 || q > 100 {
			q = 82
		}
		if err := jpeg.Encode(&hbuf, src, &jpeg.Options{Quality: q}); err != nil {
			return nil, "", err
		}
	}
	if hbuf.Len() > 0 && hbuf.Len() < len(raw) {
		return hbuf.Bytes(), mime, nil
	}
	return nil, "", nil
}

func ProcessImage(raw []byte, hdMaxWidth, hdQuality int, optimize bool) (*ProcessedImage, error) {
	res, err := ProcessImageFast(raw)
	if err != nil {
		return nil, err
	}
	if optimize {
		if hd, mime, err := ProcessImageHD(raw, res.MimeType, res.Width, hdMaxWidth, hdQuality); err == nil && hd != nil {
			res.HD = hd
			res.MimeType = mime
			if res.Colors == nil {
				if src, _, derr := image.Decode(bytes.NewReader(hd)); derr == nil {
					res.Bg, res.Colors = analyzeColors(src)
				}
			}
		}
	}
	return res, nil
}

func TransformOnTheFly(raw []byte, width int, quality int) ([]byte, string, error) {
	if width <= 0 {
		return raw, "", nil
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", err
	}
	if src.Bounds().Dx() <= width {
		return raw, "", nil
	}
	dst := imaging.Resize(src, width, 0, imaging.Lanczos)
	var buf bytes.Buffer
	if quality <= 0 {
		quality = 82
	}
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "image/jpeg", nil
}

var (
	svgWHRe   = regexp.MustCompile(`(?i)\bwidth\s*=\s*"([0-9.]+)(px)?"`)
	svgHRe    = regexp.MustCompile(`(?i)\bheight\s*=\s*"([0-9.]+)(px)?"`)
	svgVBRe   = regexp.MustCompile(`(?i)\bviewBox\s*=\s*"\s*(-?[0-9.]+)[ ,]+(-?[0-9.]+)[ ,]+([0-9.]+)[ ,]+([0-9.]+)`)
	svgRootRe = regexp.MustCompile(`(?is)<svg[^>]*>`)
	svgFillRe = regexp.MustCompile(`(?i)\bfill=["'](#[0-9a-fA-F]{3}([0-9a-fA-F]{3})?)["']`)
	svgAllRe  = regexp.MustCompile(`(?i)\b(?:fill|stroke|stop-color|flood-color|lighting-color)\s*=\s*["'](#[0-9a-fA-F]{3,8})["']|\b(?:fill|stroke|stop-color)\s*:\s*(#[0-9a-fA-F]{3,8})`)
)

type colorBucket struct {
	r, g, b uint64
	n       uint64
}

func analyzeColors(src image.Image) (string, []ColorInfo) {
	if src == nil {
		return "", nil
	}
	avg := averageHex(src)

	small := imaging.Resize(src, 48, 48, imaging.Box)
	buckets := map[int]*colorBucket{}
	total := uint64(0)
	b := small.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := small.At(x, y).RGBA()
			if a < 0x4000 {
				continue
			}
			rr, gg, bb := uint8(r>>8), uint8(g>>8), uint8(bl>>8)
			k := (int(rr)>>4)<<8 | (int(gg)>>4)<<4 | int(bb)>>4
			c := buckets[k]
			if c == nil {
				c = &colorBucket{}
				buckets[k] = c
			}
			c.r += uint64(rr)
			c.g += uint64(gg)
			c.b += uint64(bb)
			c.n++
			total++
		}
	}
	if total == 0 {
		return avg, nil
	}

	type cand struct {
		r, g, b uint8
		n       uint64
	}
	list := make([]cand, 0, len(buckets))
	for _, c := range buckets {
		list = append(list, cand{uint8(c.r / c.n), uint8(c.g / c.n), uint8(c.b / c.n), c.n})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n })

	picked := make([]cand, 0, paletteSize)
	for _, c := range list {
		if len(picked) >= paletteSize {
			break
		}
		dup := false
		for _, p := range picked {
			if colorDist(c.r, c.g, c.b, p.r, p.g, p.b) < 34 {
				dup = true
				break
			}
		}
		if !dup {
			picked = append(picked, c)
		}
	}

	colors := make([]ColorInfo, 0, len(picked))
	for _, p := range picked {
		colors = append(colors, ColorInfo{
			Hex: fmt.Sprintf("#%02x%02x%02x", p.r, p.g, p.b),
			Pct: round2(float64(p.n) / float64(total) * 100),
		})
	}
	if len(colors) == 0 {
		colors = append(colors, ColorInfo{Hex: avg, Pct: 100})
	}
	return avg, colors
}

func colorDist(r1, g1, b1, r2, g2, b2 uint8) int {
	dr := int(r1) - int(r2)
	dg := int(g1) - int(g2)
	db := int(b1) - int(b2)
	return (dr*dr*2 + dg*dg*4 + db*db*3) / 9
}

func averageHex(src image.Image) string {
	if src == nil {
		return ""
	}
	small := imaging.Resize(src, 8, 8, imaging.Box)
	var rSum, gSum, bSum, n uint64
	b := small.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			rr, gg, bb, aa := small.At(x, y).RGBA()
			if aa < 0x4000 {
				continue
			}
			rSum += uint64(rr)
			gSum += uint64(gg)
			bSum += uint64(bb)
			n++
		}
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", uint8(rSum/n>>8), uint8(gSum/n>>8), uint8(bSum/n>>8))
}

func dominantHex(src image.Image) string { return averageHex(src) }

func svgPalette(raw []byte) (string, []ColorInfo) {
	counts := map[string]int{}
	bg := normalizeHex(firstSVGFill(raw))
	if bg != "" {
		counts[bg] += 6
	}
	for _, m := range svgAllRe.FindAllSubmatch(raw, 400) {
		s := string(m[1])
		if s == "" {
			s = string(m[2])
		}
		if h := normalizeHex(s); h != "" {
			counts[h]++
		}
	}
	if len(counts) == 0 {
		return "", nil
	}
	type kv struct {
		hex string
		n   int
	}
	list := make([]kv, 0, len(counts))
	total := 0
	for h, n := range counts {
		list = append(list, kv{h, n})
		total += n
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].n != list[j].n {
			return list[i].n > list[j].n
		}
		return list[i].hex < list[j].hex
	})
	if len(list) > paletteSize {
		list = list[:paletteSize]
	}
	colors := make([]ColorInfo, 0, len(list))
	for _, e := range list {
		colors = append(colors, ColorInfo{Hex: e.hex, Pct: round2(float64(e.n) / float64(total) * 100)})
	}
	if bg == "" {
		bg = colors[0].Hex
	}
	return bg, colors
}

func firstSVGFill(raw []byte) string {
	low := bytes.ToLower(raw)
	if i := bytes.Index(low, []byte("<rect")); i >= 0 {
		chunk := raw[i:]
		if len(chunk) > 400 {
			chunk = chunk[:400]
		}
		if m := svgFillRe.FindSubmatch(chunk); m != nil {
			return string(m[1])
		}
	}
	if m := svgFillRe.FindSubmatch(raw); m != nil {
		return string(m[1])
	}
	return ""
}

func normalizeHex(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch len(s) {
	case 4:
		if s[0] == '#' {
			return fmt.Sprintf("#%c%c%c%c%c%c", s[1], s[1], s[2], s[2], s[3], s[3])
		}
		return ""
	case 7:
		if validBg(s) {
			return s
		}
	case 9:
		if s[0] == '#' && validHexRun(s[1:7]) {
			return s[1:7]
		}
	}
	return ""
}

func validHexRun(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func validBg(s string) bool {
	return len(s) == 7 && s[0] == '#' && validHexRun(s[1:])
}

func svgDimensions(raw []byte) (int, int) {
	root := raw
	if m := svgRootRe.Find(raw); m != nil {
		root = m
	}
	num := func(re *regexp.Regexp) float64 {
		m := re.FindSubmatch(root)
		if m == nil {
			return 0
		}
		f, _ := strconv.ParseFloat(string(m[1]), 64)
		return f
	}
	w, h := num(svgWHRe), num(svgHRe)
	if w > 0 && h > 0 {
		return int(w), int(h)
	}
	if m := svgVBRe.FindSubmatch(root); m != nil {
		vw, _ := strconv.ParseFloat(string(m[3]), 64)
		vh, _ := strconv.ParseFloat(string(m[4]), 64)
		if w <= 0 && vw > 0 {
			w = vw
		}
		if h <= 0 && vh > 0 {
			h = vh
		}
	}
	return int(w), int(h)
}

func FormatDuration(sec float64) string {
	if sec <= 0 {
		return ""
	}
	total := int(sec)
	h, m, s := total/3600, (total%3600)/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
