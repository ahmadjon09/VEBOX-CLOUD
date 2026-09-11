package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrVaultUnavailable = errors.New("vault temporarily unavailable")
	ErrVaultObject      = errors.New("vault object error")
)

type vaultNode struct {
	token       string
	blockedTill atomic.Int64
	inflight    atomic.Int32
	ok          atomic.Int64
	fail        atomic.Int64
}

func (n *vaultNode) available() bool { return time.Now().UnixNano() > n.blockedTill.Load() }

func (n *vaultNode) block(d time.Duration) {
	n.blockedTill.Store(time.Now().Add(d).UnixNano())
}

type VaultClient struct {
	nodes    []*vaultNode
	chatID   string
	cooldown time.Duration
	http     *http.Client
	big      *http.Client
	timeout  time.Duration
	rr       atomic.Uint64
	linkTTL  time.Duration

	mu    sync.RWMutex
	links map[string]linkEntry
	rnd   *rand.Rand
	rndMu sync.Mutex
	demo  bool

	demoMu   sync.RWMutex
	demoBlob map[string][]byte
}

type linkEntry struct {
	url string
	exp time.Time
}

func NewVaultClient(tokens []string, chatID string, cooldown, timeout, linkTTL time.Duration) *VaultClient {
	v := &VaultClient{
		chatID:   chatID,
		cooldown: cooldown,
		linkTTL:  linkTTL,
		links:    make(map[string]linkEntry),
		rnd:      rand.New(rand.NewSource(time.Now().UnixNano())),
		demoBlob: make(map[string][]byte),
		timeout:  timeout,
	}
	tr := &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	v.http = &http.Client{Timeout: timeout, Transport: tr}
	v.big = &http.Client{Transport: tr}
	for _, t := range tokens {
		v.nodes = append(v.nodes, &vaultNode{token: t})
	}
	v.demo = len(v.nodes) == 0 || chatID == ""
	go v.gcLinks()
	return v
}

func (v *VaultClient) DemoMode() bool { return v.demo }

func (v *VaultClient) pick() (*vaultNode, error) {
	n := len(v.nodes)
	if n == 0 {
		return nil, ErrVaultUnavailable
	}
	start := int(v.rr.Add(1)) % n
	var best *vaultNode
	for i := 0; i < n; i++ {
		cand := v.nodes[(start+i)%n]
		if !cand.available() {
			continue
		}
		if best == nil || cand.inflight.Load() < best.inflight.Load() {
			best = cand
		}
	}
	if best == nil {
		return nil, ErrVaultUnavailable
	}
	return best, nil
}

func (v *VaultClient) PoolStats() []map[string]any {
	out := make([]map[string]any, 0, len(v.nodes))
	now := time.Now().UnixNano()
	for i, n := range v.nodes {
		blocked := int64(0)
		if t := n.blockedTill.Load(); t > now {
			blocked = int64(time.Duration(t - now).Seconds())
		}
		out = append(out, map[string]any{
			"node":          fmt.Sprintf("node-%02d", i+1),
			"available":     n.available(),
			"inflight":      int64(n.inflight.Load()),
			"ok":            n.ok.Load(),
			"failed":        n.fail.Load(),
			"blocked_for_s": blocked,
		})
	}
	return out
}

func (v *VaultClient) LinkCacheSize() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.links)
}

func (v *VaultClient) gcLinks() {
	t := time.NewTicker(5 * time.Minute)
	for range t.C {
		now := time.Now()
		v.mu.Lock()
		for k, e := range v.links {
			if now.After(e.exp) {
				delete(v.links, k)
			}
		}
		v.mu.Unlock()
	}
}

type apiResp struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

type remoteDoc struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size"`
}

type remoteMsg struct {
	MessageID int64       `json:"message_id"`
	Document  *remoteDoc  `json:"document"`
	Audio     *remoteDoc  `json:"audio"`
	Voice     *remoteDoc  `json:"voice"`
	Video     *remoteDoc  `json:"video"`
	Animation *remoteDoc  `json:"animation"`
	VideoNote *remoteDoc  `json:"video_note"`
	Sticker   *remoteDoc  `json:"sticker"`
	Photo     []remoteDoc `json:"photo"`
}

func fileIDFromMsg(msg remoteMsg) string {
	cands := []*remoteDoc{msg.Document, msg.Audio, msg.Voice, msg.Video, msg.Animation, msg.VideoNote, msg.Sticker}
	for _, d := range cands {
		if d != nil && d.FileID != "" {
			return d.FileID
		}
	}
	if n := len(msg.Photo); n > 0 && msg.Photo[n-1].FileID != "" {
		return msg.Photo[n-1].FileID
	}
	return ""
}

type remoteFile struct {
	FilePath string `json:"file_path"`
}

func (v *VaultClient) Put(ctx context.Context, name string, data []byte) (string, error) {
	if v.demo {
		key := "demo:" + name + ":" + strconv.FormatInt(time.Now().UnixNano(), 36)
		v.demoMu.Lock()
		v.demoBlob[key] = append([]byte(nil), data...)
		v.demoMu.Unlock()
		return key, nil
	}

	node, err := v.pick()
	if err != nil {
		return "", err
	}
	ref, err := v.putVia(ctx, node, name, data)
	if err == nil {
		node.ok.Add(1)
		return ref, nil
	}
	node.fail.Add(1)
	log.Printf("[vault] put failed (%d bytes, name=%q): %v", len(data), name, err)
	var fw floodWait
	if errors.As(err, &fw) {
		d := time.Duration(fw.seconds) * time.Second
		if d < v.cooldown {
			d = v.cooldown
		}
		node.block(d)
	}
	return "", fmt.Errorf("%w: %v", ErrVaultUnavailable, err)
}

type floodWait struct{ seconds int }

func (f floodWait) Error() string { return "rate limited" }

func (v *VaultClient) putVia(ctx context.Context, node *vaultNode, name string, data []byte) (string, error) {
	node.inflight.Add(1)
	defer node.inflight.Add(-1)

	var head bytes.Buffer
	mw := multipart.NewWriter(&head)
	_ = mw.WriteField("chat_id", v.chatID)
	_ = mw.WriteField("disable_notification", "true")
	_ = mw.WriteField("disable_content_type_detection", "true")
	if _, err := mw.CreateFormFile("document", name); err != nil {
		return "", err
	}
	closing := []byte("\r\n--" + mw.Boundary() + "--\r\n")
	reqBody := io.MultiReader(
		bytes.NewReader(head.Bytes()),
		bytes.NewReader(data),
		bytes.NewReader(closing),
	)

	timeout := v.timeout
	if bySize := 30*time.Second + time.Duration(len(data)/(256*1024))*time.Second; bySize > timeout {
		timeout = bySize
	}
	if timeout > 5*time.Minute {
		timeout = 5 * time.Minute
	}
	pctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(pctx, http.MethodPost, v.endpoint(node, "sendDocument"), reqBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.ContentLength = int64(head.Len()) + int64(len(data)) + int64(len(closing))

	resp, err := v.big.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var ar apiResp
	_ = json.Unmarshal(body, &ar)
	if resp.StatusCode == http.StatusTooManyRequests || ar.ErrorCode == 429 {
		return "", floodWait{seconds: ar.Parameters.RetryAfter}
	}
	if !ar.OK {
		if ar.Description != "" {
			log.Printf("[vault] upstream rejected object: %d %s", resp.StatusCode, ar.Description)
		}
		return "", fmt.Errorf("%w: upstream %d", ErrVaultObject, resp.StatusCode)
	}
	var msg remoteMsg
	if err := json.Unmarshal(ar.Result, &msg); err != nil {
		return "", ErrVaultObject
	}
	fileID := fileIDFromMsg(msg)
	if fileID == "" {
		return "", ErrVaultObject
	}
	return makeRef(msg.MessageID, fileID), nil
}

const refSep = "#"

func makeRef(messageID int64, fileID string) string {
	if messageID <= 0 {
		return fileID
	}
	return strconv.FormatInt(messageID, 10) + refSep + fileID
}

func refParts(ref string) (fileID string, messageID int64) {
	i := strings.Index(ref, refSep)
	if i < 0 {
		return ref, 0
	}
	id, err := strconv.ParseInt(ref[:i], 10, 64)
	if err != nil || id <= 0 {
		return ref[i+1:], 0
	}
	return ref[i+1:], id
}

func (v *VaultClient) Open(ctx context.Context, ref string, rangeHeader string) (io.ReadCloser, http.Header, int, error) {
	if v.demo {
		v.demoMu.RLock()
		b, ok := v.demoBlob[ref]
		v.demoMu.RUnlock()
		if !ok {
			return nil, nil, 0, ErrVaultObject
		}
		h := http.Header{}
		h.Set("Accept-Ranges", "bytes")
		if rangeHeader != "" {
			lo, hi, valid := parseRange(rangeHeader, int64(len(b)))
			if !valid {
				h.Set("Content-Range", fmt.Sprintf("bytes */%d", len(b)))
				return io.NopCloser(strings.NewReader("")), h, http.StatusRequestedRangeNotSatisfiable, nil
			}
			h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", lo, hi, len(b)))
			h.Set("Content-Length", strconv.FormatInt(hi-lo+1, 10))
			return io.NopCloser(bytes.NewReader(b[lo : hi+1])), h, http.StatusPartialContent, nil
		}
		h.Set("Content-Length", strconv.Itoa(len(b)))
		return io.NopCloser(bytes.NewReader(b)), h, http.StatusOK, nil
	}

	link, err := v.resolveLink(ctx, ref)
	if err != nil {
		return nil, nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, nil, 0, err
	}
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	resp, err := v.http.Do(req)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("%w: %v", ErrVaultUnavailable, err)
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		v.mu.Lock()
		delete(v.links, ref)
		v.mu.Unlock()
		return nil, nil, 0, ErrVaultObject
	}
	h := http.Header{}
	for _, k := range []string{"Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified"} {
		if val := resp.Header.Get(k); val != "" {
			h.Set(k, val)
		}
	}
	return resp.Body, h, resp.StatusCode, nil
}

func (v *VaultClient) Delete(ctx context.Context, ref string) error {
	if ref == "" {
		return nil
	}
	v.mu.Lock()
	delete(v.links, ref)
	v.mu.Unlock()

	if v.demo {
		v.demoMu.Lock()
		delete(v.demoBlob, ref)
		v.demoMu.Unlock()
		return nil
	}

	_, messageID := refParts(ref)
	if messageID <= 0 {
		return nil
	}

	node, err := v.pick()
	if err != nil {
		return err
	}
	form := url.Values{}
	form.Set("chat_id", v.chatID)
	form.Set("message_id", strconv.FormatInt(messageID, 10))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		v.endpoint(node, "deleteMessage"), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.http.Do(req)
	if err != nil {
		node.fail.Add(1)
		return fmt.Errorf("%w: %v", ErrVaultUnavailable, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	var ar apiResp
	_ = json.Unmarshal(body, &ar)
	if resp.StatusCode == http.StatusTooManyRequests || ar.ErrorCode == 429 {
		d := time.Duration(ar.Parameters.RetryAfter) * time.Second
		if d < v.cooldown {
			d = v.cooldown
		}
		node.block(d)
		return floodWait{seconds: ar.Parameters.RetryAfter}
	}
	if !ar.OK {
		node.fail.Add(1)
		return ErrVaultObject
	}
	node.ok.Add(1)
	return nil
}

func (v *VaultClient) Fetch(ctx context.Context, ref string, limit int64) ([]byte, error) {
	rc, _, _, err := v.Open(ctx, ref, "")
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, limit))
}

func (v *VaultClient) resolveLink(ctx context.Context, ref string) (string, error) {
	v.mu.RLock()
	if e, ok := v.links[ref]; ok && time.Now().Before(e.exp) {
		v.mu.RUnlock()
		return e.url, nil
	}
	v.mu.RUnlock()

	var lastErr error
	for attempt := 0; attempt < len(v.nodes)+1; attempt++ {
		node, err := v.pick()
		if err != nil {
			lastErr = err
			break
		}
		fileID, _ := refParts(ref)
		u := v.endpoint(node, "getFile") + "?file_id=" + url.QueryEscape(fileID)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := v.http.Do(req)
		if err != nil {
			node.fail.Add(1)
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()

		var ar apiResp
		_ = json.Unmarshal(body, &ar)
		if resp.StatusCode == http.StatusTooManyRequests || ar.ErrorCode == 429 {
			d := time.Duration(ar.Parameters.RetryAfter) * time.Second
			if d < v.cooldown {
				d = v.cooldown
			}
			node.block(d)
			lastErr = floodWait{}
			continue
		}
		if !ar.OK {
			lastErr = ErrVaultObject
			continue
		}
		var f remoteFile
		if err := json.Unmarshal(ar.Result, &f); err != nil || f.FilePath == "" {
			lastErr = ErrVaultObject
			continue
		}
		link := v.fileURL(node, f.FilePath)
		v.mu.Lock()
		v.links[ref] = linkEntry{url: link, exp: time.Now().Add(v.linkTTL)}
		v.mu.Unlock()
		node.ok.Add(1)
		return link, nil
	}
	return "", fmt.Errorf("%w: %v", ErrVaultUnavailable, lastErr)
}

func (v *VaultClient) endpoint(n *vaultNode, method string) string {
	return "https://api." + upstreamHost() + "/bot" + n.token + "/" + method
}

func (v *VaultClient) fileURL(n *vaultNode, path string) string {
	return "https://api." + upstreamHost() + "/file/bot" + n.token + "/" + path
}

func upstreamHost() string { return strings.Join([]string{"telegram", "org"}, ".") }
