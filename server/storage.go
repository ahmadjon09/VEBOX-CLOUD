package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

type StorageService struct {
	cfg   *Config
	core  CoreStore
	meta  MetaStore
	vault *VaultClient
	gov   *MemoryGovernor
	rc    *RedisCache

	hot       *lru.LRU[string, []byte]
	metaCache *lru.LRU[string, *ImageMeta]
	tickets   sync.Map

	staging      *lru.LRU[string, []byte]
	stagingBytes atomic.Int64
	stagingCap   int64
	dedup        *lru.LRU[string, string]
	dedupMu      sync.Mutex
	usageCache   *lru.LRU[string, *UsageCounter]

	pendingMu sync.Mutex
	pending   map[string]*uploadJob
	jobs      chan *uploadJob
	closeOnce sync.Once
	wg        sync.WaitGroup

	viewMu   sync.Mutex
	viewBuf  map[string]int64
	viewStop chan struct{}
}

type uploadJob struct {
	id         string
	userID     string
	pol        TierPolicy
	raw        []byte
	width      int
	filename   string
	visibility string
	checksum   string
	meta       *ImageMeta
	origSize   int64
	createdAt  time.Time
	dead       atomic.Bool
}

func NewStorageService(cfg *Config, core CoreStore, meta MetaStore, vault *VaultClient, gov *MemoryGovernor, rc *RedisCache) *StorageService {
	stagingCap := int64(512 << 20)
	if cfg.MemLimitBytes > 0 {
		stagingCap = int64(cfg.MemLimitBytes) / 2
	}
	if stagingCap < 64<<20 {
		stagingCap = 64 << 20
	}
	return &StorageService{
		cfg: cfg, core: core, meta: meta, vault: vault, gov: gov, rc: rc,
		hot:        lru.NewLRU[string, []byte](4000, nil, 30*time.Minute),
		metaCache:  lru.NewLRU[string, *ImageMeta](cfg.LRUSize, nil, 15*time.Minute),
		staging:    lru.NewLRU[string, []byte](8000, nil, 30*time.Minute),
		stagingCap: stagingCap,
		dedup:      lru.NewLRU[string, string](20000, nil, 24*time.Hour),
		usageCache: lru.NewLRU[string, *UsageCounter](20000, nil, 60*time.Second),
		pending:    make(map[string]*uploadJob),
		jobs:       make(chan *uploadJob, 256),
		viewBuf:    make(map[string]int64),
		viewStop:   make(chan struct{}),
	}
}

func (s *StorageService) metaKey(id string) string { return "m:" + id }

func (s *StorageService) varKey(id string, width int) string {
	return "v:" + id + ":w" + strconv.Itoa(width)
}

func (s *StorageService) remember(m *ImageMeta) {
	if m == nil || m.ID == "" {
		return
	}
	cp := *m
	s.metaCache.Add(m.ID, &cp)
	s.tickets.Store(m.ID, &cp)
}

func (s *StorageService) store(ctx context.Context, m *ImageMeta) {
	s.remember(m)
	if !s.rc.On() || m == nil || m.ID == "" {
		return
	}
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	s.rc.SetAsync(s.metaKey(m.ID), b, s.rc.cfg.MetaTTL)
}

func (s *StorageService) forget(id string) {
	if id == "" {
		return
	}
	s.metaCache.Remove(id)
	s.tickets.Delete(id)
	s.hot.Remove(hotKey(id, "thumb"))
	for _, k := range s.hot.Keys() {
		if strings.HasPrefix(k, id+":w") {
			s.hot.Remove(k)
		}
	}
	if s.rc.On() {
		s.rc.Del(s.metaKey(id), s.varKey(id, 0))
	}
}

func (s *StorageService) mintMediaID() string {
	for i := 0; i < 16; i++ {
		id := shortID(mediaIDLen)
		if _, ok := s.tickets.Load(id); ok {
			continue
		}
		if _, ok := s.metaCache.Get(id); ok {
			continue
		}
		return id
	}
	return shortID(mediaIDLen + 4)
}

func (s *StorageService) StartWorkers(ctx context.Context) {
	n := 4
	if s.cfg.MaxHeavyJobs > 0 && s.cfg.MaxHeavyJobs < n {
		n = s.cfg.MaxHeavyJobs
	}
	for i := 0; i < n; i++ {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for j := range s.jobs {
				s.processJob(j)
			}
		}()
	}
	s.wg.Add(1)
	go s.viewFlusher()
	log.Printf("Write-behind workerlar: %d ta (RAM-first yuklash faol)", n)
}

func (s *StorageService) Close() {
	s.closeOnce.Do(func() {
		close(s.jobs)
		close(s.viewStop)
	})
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		log.Printf("[persist] %d ta fayl hali DB'ga yozilmagan — server to'xtadi", len(s.pendingSnapshot()))
	}
	s.flushViews()
}

type UploadResult struct {
	Image    PublicImage `json:"image"`
	StoredHD int64       `json:"stored_hd_bytes"`
	Elapsed  int64       `json:"processing_ms"`
}

func (s *StorageService) Upload(ctx context.Context, user *User, pol TierPolicy, filename string, raw []byte, visibility string) (*UploadResult, error) {
	start := time.Now()
	if !validVisibility(visibility) {
		visibility = s.cfg.DefaultVisible
	}
	info, err := DetectMedia(raw)
	if err != nil {
		return nil, err
	}
	if info.Kind == KindVideo {
		return nil, ErrVideoNotSupported
	}
	if !IsSupported(info) {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, info.Mime)
	}
	name := EnsureExt(sanitizeFilename(filename), info.Mime)
	if info.Kind == KindAudio && !s.cfg.AllowAudio {
		return nil, ErrAudioDisabled
	}

	usage, err := s.UsageCached(ctx, user.ID)
	if err != nil {
		usage = &UsageCounter{UserID: user.ID}
	}
	quotaPolicy := s.applyServerCaps(pol)
	if qe := CheckUploadQuota(quotaPolicy, usage, int64(len(raw)), info.Kind); qe != nil {
		return nil, qe
	}

	proc, err := ProcessImageFast(raw)
	if err != nil {
		return nil, err
	}

	if proc.IsAudio() {
		return s.uploadStreamed(ctx, user, pol, name, proc, visibility, start)
	}

	hdSize := int64(len(proc.HD))
	id := s.mintMediaID()
	if !s.stageBlob(hotKey(id, "hd"), proc.HD) {
		return s.uploadSync(ctx, user, pol, name, raw, visibility, start)
	}

	m := &ImageMeta{
		ID:         id,
		UserID:     user.ID,
		Filename:   name,
		MimeType:   proc.MimeType,
		Kind:       string(proc.Kind),
		Duration:   proc.Duration,
		SizeBytes:  hdSize,
		SizeKB:     round2(float64(hdSize) / 1024),
		Width:      proc.Width,
		Height:     proc.Height,
		Bg:         proc.Bg,
		Colors:     proc.Colors,
		Checksum:   proc.Checksum,
		Visibility: visibility,
		Tier:       pol.Name,
		CreatedAt:  time.Now().UTC(),
	}
	s.remember(m)

	job := &uploadJob{
		id: id, userID: user.ID, pol: pol, raw: proc.Original,
		width: proc.Width, filename: name, visibility: visibility,
		checksum: proc.Checksum, meta: m, origSize: hdSize, createdAt: m.CreatedAt,
	}
	s.pendingAdd(id, job)
	s.usageBump(user.ID, hdSize, 1)

	select {
	case s.jobs <- job:
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.processJob(job)
		}()
	}

	return &UploadResult{
		Image:    m.Public(s.cfg.LinkBase()),
		StoredHD: m.SizeBytes,
		Elapsed:  time.Since(start).Milliseconds(),
	}, nil
}

func (s *StorageService) applyServerCaps(pol TierPolicy) TierPolicy {
	out := pol
	if out.MaxAudioKB > 0 && s.cfg.MaxAudioMB > 0 {
		if c := int64(s.cfg.MaxAudioMB) * 1024; c < out.MaxAudioKB {
			out.MaxAudioKB = c
		}
	}
	return out
}

func (s *StorageService) uploadStreamed(ctx context.Context, user *User, pol TierPolicy, name string, proc *ProcessedImage, visibility string, start time.Time) (*UploadResult, error) {
	ref, err := s.putDedup(ctx, name, proc.Checksum, proc.Original)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		log.Printf("[upload] streamed put failed (kind=%s, %d bytes): %v", proc.Kind, len(proc.Original), err)
		return nil, fmt.Errorf("%w", ErrVaultUnavailable)
	}

	id := s.mintMediaID()
	size := int64(len(proc.Original))
	m := &ImageMeta{
		ID: id, UserID: user.ID, Filename: name, MimeType: proc.MimeType,
		Kind: string(proc.Kind), Duration: proc.Duration,
		SizeBytes: size, SizeKB: round2(float64(size) / 1024),
		Width: proc.Width, Height: proc.Height, Bg: proc.Bg, Colors: proc.Colors,
		Checksum: proc.Checksum, Visibility: visibility, Tier: pol.Name,
		CreatedAt: time.Now().UTC(), VaultRef: ref,
	}
	if err := s.meta.InsertImage(ctx, m); err != nil {
		log.Printf("[upload] metadata insert failed (id=%s): %v", id, err)
		go func(ref string) {
			c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = s.vault.Delete(c, ref)
		}(ref)
		return nil, fmt.Errorf("metadata saqlanmadi: %w", err)
	}
	s.store(ctx, m)
	s.usageBump(user.ID, m.SizeBytes, 1)
	_ = s.core.AddUsage(ctx, user.ID, m.SizeBytes, 1)
	return &UploadResult{
		Image: m.Public(s.cfg.LinkBase()), StoredHD: m.SizeBytes,
		Elapsed: time.Since(start).Milliseconds(),
	}, nil
}

func (s *StorageService) putDedup(ctx context.Context, name, checksum string, data []byte) (string, error) {
	s.dedupMu.Lock()
	defer s.dedupMu.Unlock()
	if ref, ok := s.dedup.Get(checksum); ok && ref != "" {
		return ref, nil
	}
	ref, err := s.vault.Put(ctx, name, data)
	if err != nil {
		return "", err
	}
	s.dedup.Add(checksum, ref)
	return ref, nil
}

func (s *StorageService) uploadSync(ctx context.Context, user *User, pol TierPolicy, name string, raw []byte, visibility string, start time.Time) (*UploadResult, error) {
	release, ok := s.gov.AcquireHeavy(ctx)
	if !ok {
		return nil, ErrServerBusy
	}
	proc, err := ProcessImage(raw, s.hdw(pol), s.cfg.HDQuality, pol.HDProcessing)
	release()
	if err != nil {
		return nil, err
	}
	ref, err := s.putDedup(ctx, name, proc.Checksum, proc.HD)
	if err != nil {
		return nil, fmt.Errorf("%w", ErrVaultUnavailable)
	}
	id := s.mintMediaID()
	m := &ImageMeta{
		ID: id, UserID: user.ID, Filename: name, MimeType: proc.MimeType,
		Kind: string(proc.Kind), Duration: proc.Duration,
		SizeBytes: int64(len(proc.HD)), SizeKB: round2(float64(len(proc.HD)) / 1024),
		Width: proc.Width, Height: proc.Height, Bg: proc.Bg, Colors: proc.Colors,
		Checksum: proc.Checksum, Visibility: visibility, Tier: pol.Name,
		CreatedAt: time.Now().UTC(), VaultRef: ref,
	}
	if err := s.meta.InsertImage(ctx, m); err != nil {
		return nil, fmt.Errorf("metadata saqlanmadi: %w", err)
	}
	s.store(ctx, m)
	s.usageBump(user.ID, m.SizeBytes, 1)
	_ = s.core.AddUsage(ctx, user.ID, m.SizeBytes, 1)
	return &UploadResult{
		Image: m.Public(s.cfg.LinkBase()), StoredHD: m.SizeBytes,
		Elapsed: time.Since(start).Milliseconds(),
	}, nil
}

func (s *StorageService) hdw(pol TierPolicy) int {
	if s.cfg.HDMaxWidth > 0 && s.cfg.HDMaxWidth < pol.MaxHDWidth {
		return s.cfg.HDMaxWidth
	}
	return pol.MaxHDWidth
}

func (s *StorageService) processJob(j *uploadJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if j.dead.Load() {
		return
	}

	hdKey := hotKey(j.id, "hd")
	if cur, ok := s.staging.Get(hdKey); ok && cur != nil {
		if j.pol.HDProcessing {
			release, okSlot := s.gov.AcquireHeavy(ctx)
			if okSlot {
				if hd, outMime, err := ProcessImageHD(j.raw, j.meta.MimeType, j.width, s.hdw(j.pol), s.cfg.HDQuality); err == nil && hd != nil {
					s.stageSwap(hdKey, hd)
					delta := int64(len(hd)) - j.origSize
					j.origSize = int64(len(hd))
					j.meta.SizeBytes = int64(len(hd))
					j.meta.SizeKB = round2(float64(len(hd)) / 1024)
					if outMime != "" {
						j.meta.MimeType = outMime
					}
					s.remember(j.meta)
					if delta != 0 {
						s.usageBump(j.userID, delta, 0)
					}
				}
				release()
			}
		}
	}
	if j.dead.Load() {
		return
	}

	var ref string
	if b, ok := s.staging.Get(hdKey); ok {
		ref, _ = s.putDedup(ctx, j.filename, j.checksum, b)
	}
	if ref != "" {
		j.meta.VaultRef = ref
		s.store(ctx, j.meta)
	}

	dbWritten := false
	for _, wait := range []time.Duration{0, 1 * time.Second, 4 * time.Second, 12 * time.Second} {
		if j.dead.Load() {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if err := s.meta.InsertImage(ctx, j.meta); err == nil {
			_ = s.core.AddUsage(ctx, j.userID, j.meta.SizeBytes, 1)
			dbWritten = true
			break
		}
	}

	flushed := dbWritten && ref != ""
	s.pendingRemove(j.id)
	if flushed {
		return
	}
	s.forget(j.id)
	s.stagingRemove(hdKey, j.meta.SizeBytes)
	s.usageBump(j.userID, -j.meta.SizeBytes, -1)
	if dbWritten {
		_, _ = s.meta.DeleteImage(ctx, j.userID, j.id)
		_ = s.core.AddUsage(ctx, j.userID, -j.meta.SizeBytes, -1)
		log.Printf("[persist] vault push muvaffaqiyatsiz — bazadagi yozuv bekor qilindi: %s", j.id)
	} else {
		log.Printf("[persist] DIQQAT: %s hali bazaga tayyor emas — xotiradan olib tashlandi", j.id)
	}
}

func (s *StorageService) stageBlob(key string, b []byte) bool {
	size := int64(len(b))
	if size > s.stagingCap {
		return false
	}
	for s.stagingBytes.Load()+size > s.stagingCap {
		if !s.stagingRemoveOldest() {
			return false
		}
	}
	s.staging.Add(key, b)
	s.stagingBytes.Add(size)
	return true
}

func (s *StorageService) stageSwap(key string, b []byte) {
	var delta int64
	if old, ok := s.staging.Get(key); ok {
		delta = int64(len(b)) - int64(len(old))
	}
	s.staging.Add(key, b)
	s.stagingBytes.Add(delta)
}

func (s *StorageService) stagingRemove(key string, size int64) {
	if _, ok := s.staging.Get(key); ok {
		s.staging.Remove(key)
		s.stagingBytes.Add(-size)
	}
}

func (s *StorageService) stagingRemoveOldest() bool {
	_, v, ok := s.staging.RemoveOldest()
	if ok {
		s.stagingBytes.Add(-int64(len(v)))
		return true
	}
	return false
}

func (s *StorageService) pendingAdd(id string, j *uploadJob) {
	s.pendingMu.Lock()
	s.pending[id] = j
	s.pendingMu.Unlock()
}

func (s *StorageService) pendingRemove(id string) *uploadJob {
	s.pendingMu.Lock()
	j := s.pending[id]
	delete(s.pending, id)
	s.pendingMu.Unlock()
	return j
}

func (s *StorageService) pendingSnapshot() []*uploadJob {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	out := make([]*uploadJob, 0, len(s.pending))
	for _, j := range s.pending {
		out = append(out, j)
	}
	return out
}

func (s *StorageService) PurgeLocalCaches() {
	s.metaCache.Purge()
	clearSyncMap(&s.tickets)
	s.hot.Purge()
	s.usageCache.Purge()
}

// clearSyncMap removes every entry from m. sync.Map has no Purge method and
// Clear only exists from Go 1.23 on, so this keeps working on older toolchains.
func clearSyncMap(m *sync.Map) {
	m.Range(func(k, _ any) bool {
		m.Delete(k)
		return true
	})
}

func (s *StorageService) QueueStats() map[string]any {
	return map[string]any{
		"pending":      len(s.pendingSnapshot()),
		"queue_len":    len(s.jobs),
		"queue_cap":    cap(s.jobs),
		"staging":      s.staging.Len(),
		"staging_cap":  s.stagingCap,
		"staged_bytes": s.stagingBytes.Load(),
		"meta_cache":   s.metaCache.Len(),
		"dedup":        s.dedup.Len(),
	}
}

func (s *StorageService) UsageCached(ctx context.Context, userID string) (*UsageCounter, error) {
	if u, ok := s.usageCache.Get(userID); ok && u != nil {
		return u, nil
	}
	u, err := s.core.GetUsage(ctx, userID)
	if err != nil {
		u = &UsageCounter{UserID: userID}
	}
	u.MonthKey, u.MonthFiles, u.MonthBytes = checkMonth(u)
	s.usageCache.Add(userID, u)
	return u, nil
}

func (s *StorageService) usageBump(userID string, bytes, files int64) {
	u, _ := s.usageCache.Get(userID)
	if u == nil {
		u = &UsageCounter{UserID: userID}
		s.usageCache.Add(userID, u)
	}
	u.MonthKey, u.MonthFiles, u.MonthBytes = checkMonth(u)
	u.StorageBytes = max64(0, u.StorageBytes+bytes)
	u.FileCount = max64(0, u.FileCount+files)
	u.MonthBytes = max64(0, u.MonthBytes+bytes)
	u.MonthFiles = max64(0, u.MonthFiles+files)
}

func checkMonth(u *UsageCounter) (string, int64, int64) {
	if u.MonthKey != monthKey() {
		return monthKey(), 0, 0
	}
	return u.MonthKey, u.MonthFiles, u.MonthBytes
}

var (
	ErrServerBusy        = errors.New("server band, keyinroq urinib ko'ring")
	ErrForbidden         = errors.New("ruxsat yo'q")
	ErrVideoNotSupported = errors.New("video qo'llab-quvvatlanmaydi")
	ErrAudioDisabled     = errors.New("audio yuklash ushbu serverda o'chirilgan")
	ErrBadSignature      = errors.New("imzo yaroqsiz yoki muddati o'tgan")
)

func (s *StorageService) Get(ctx context.Context, id string) (*ImageMeta, error) {
	if m, ok := s.metaCache.Get(id); ok {
		return m, nil
	}
	if v, ok := s.tickets.Load(id); ok {
		m := v.(*ImageMeta)
		s.metaCache.Add(id, m)
		return m, nil
	}
	if s.rc.On() {
		if b, ok := s.rc.Get(s.metaKey(id)); ok {
			m := &ImageMeta{}
			if json.Unmarshal(b, m) == nil && m.ID != "" {
				s.remember(m)
				return m, nil
			}
		}
	}
	m, err := s.meta.GetImage(ctx, id)
	if err != nil {
		return nil, err
	}
	s.store(ctx, m)
	return m, nil
}

func sortImageMeta(items []ImageMeta, sortKey string) {
	sort.Slice(items, func(i, j int) bool {
		switch sortKey {
		case "oldest":
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		case "largest":
			return items[i].SizeBytes > items[j].SizeBytes
		case "smallest":
			return items[i].SizeBytes < items[j].SizeBytes
		case "views":
			return items[i].Views > items[j].Views
		case "name":
			return strings.ToLower(items[i].Filename) < strings.ToLower(items[j].Filename)
		default:
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
	})
}

func (s *StorageService) listMeta(ctx context.Context, userID string, limit, offset int, f ListFilter) ([]ImageMeta, int64, error) {
	items, total, err := s.meta.ListImages(ctx, userID, limit, offset, f)
	if err != nil {
		return nil, 0, err
	}

	seen := make(map[string]bool, len(items))
	for i := range items {
		seen[items[i].ID] = true
	}
	var pend []ImageMeta
	s.pendingMu.Lock()
	for _, j := range s.pending {
		if j.userID == userID && !seen[j.id] && f.Match(j.meta) {
			pend = append(pend, *j.meta)
		}
	}
	s.pendingMu.Unlock()
	if len(pend) > 0 {
		items = append(items, pend...)
		total += int64(len(pend))
		sortImageMeta(items, f.Sort)
		items = page(items, limit, offset)
	}
	return items, total, nil
}

func (s *StorageService) List(ctx context.Context, userID string, limit, offset int, f ListFilter) ([]MediaLink, int64, error) {
	items, total, err := s.listMeta(ctx, userID, limit, offset, f)
	if err != nil {
		return nil, 0, err
	}
	base := s.cfg.LinkBase()
	out := make([]MediaLink, 0, len(items))
	for i := range items {
		out = append(out, items[i].Link(base))
	}
	return out, total, nil
}

func (s *StorageService) Details(ctx context.Context, userID string, admin bool, ids []string) []PublicImage {
	base := s.cfg.LinkBase()
	out := make([]PublicImage, 0, len(ids))
	for _, id := range ids {
		m, err := s.Get(ctx, id)
		if err != nil || m == nil {
			continue
		}
		if m.UserID != userID && !admin {
			continue
		}
		out = append(out, m.Public(base))
	}
	return out
}

type ListFilter struct {
	Q          string
	Kind       string
	Visibility string
	Sort       string
}

func (f ListFilter) Active() bool {
	return f.Q != "" || f.Kind != "" || f.Visibility != "" || (f.Sort != "" && f.Sort != "newest")
}

func (f ListFilter) Match(m *ImageMeta) bool {
	if f.Visibility != "" && m.Visibility != f.Visibility {
		return false
	}
	if f.Q != "" && !strings.Contains(strings.ToLower(m.Filename), strings.ToLower(f.Q)) {
		return false
	}
	switch f.Kind {
	case "", "all":
	case "image":
		if m.IsVideo() || m.IsAudio() {
			return false
		}
	case "audio":
		if !m.IsAudio() {
			return false
		}
	case "gif":
		if m.MimeType != MimeGIF {
			return false
		}
	case "svg":
		if m.MimeType != MimeSVG {
			return false
		}
	case "public":
		if !m.IsPublic() {
			return false
		}
	case "private":
		if m.IsPublic() {
			return false
		}
	}
	return true
}

func (s *StorageService) SetVisibility(ctx context.Context, userID, id, visibility string) (*ImageMeta, error) {
	if !validVisibility(visibility) {
		return nil, ErrForbidden
	}
	if j, ok := s.pendingMark(id); ok && j.userID == userID {
		j.meta.Visibility = visibility
		s.store(ctx, j.meta)
		cp := *j.meta
		return &cp, nil
	}
	m, err := s.meta.SetVisibility(ctx, userID, id, visibility)
	if err != nil {
		return nil, err
	}
	s.store(ctx, m)
	return m, nil
}

func (s *StorageService) Delete(ctx context.Context, userID, id string) error {
	if j, ok := s.pendingMark(id); ok && j.userID == userID {
		j.dead.Store(true)
		s.pendingRemove(id)
		s.forget(id)
		s.stagingRemove(hotKey(id, "hd"), j.meta.SizeBytes)
		s.usageBump(userID, -j.meta.SizeBytes, -1)
		s.purgeObject(id, j.meta)
		return nil
	}

	m, err := s.meta.DeleteImage(ctx, userID, id)
	if err != nil {
		return err
	}
	s.forget(id)
	s.stagingRemove(hotKey(id, "hd"), m.SizeBytes)
	_ = s.core.AddUsage(ctx, userID, -m.SizeBytes, -1)
	s.usageBump(userID, -m.SizeBytes, -1)
	s.purgeObject(id, m)
	return nil
}

func (s *StorageService) purgeObject(id string, m *ImageMeta) {
	ref := m.VaultRef
	if ref == "" || s.vault == nil {
		return
	}
	if j, ok := s.pendingMark(id); ok && j.meta != nil && j.meta.VaultRef != "" {
		ref = j.meta.VaultRef
	}
	go func() {
		delays := []time.Duration{0, 2 * time.Second, 10 * time.Second}
		for i, d := range delays {
			if d > 0 {
				time.Sleep(d)
			}
			c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			err := s.vault.Delete(c, ref)
			cancel()
			if err == nil {
				return
			}
			if i == len(delays)-1 {
				log.Printf("[vault] o'chirish muvaffaqiyatsiz (%s): %v", id, err)
			}
		}
	}()
}

func (s *StorageService) pendingMark(id string) (*uploadJob, bool) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	j, ok := s.pending[id]
	return j, ok
}

func hotKey(id, variant string) string { return id + ":" + variant }

func sanitizeFilename(n string) string {
	n = strings.TrimSpace(n)
	n = strings.ReplaceAll(n, "/", "_")
	n = strings.ReplaceAll(n, "\\", "_")
	n = strings.ReplaceAll(n, refSep, "_")
	n = strings.ReplaceAll(n, "\x00", "")
	if n == "" {
		n = "image"
	}
	if len(n) > 120 {
		n = n[:120]
	}
	return n
}

func (s *StorageService) Deliver(w http.ResponseWriter, r *http.Request, m *ImageMeta, variant string, width int, pol TierPolicy) {
	ctx := r.Context()

	etag := `"` + etagToken(m.Checksum) + "-" + variant + `"`
	if width > 0 {
		etag = `"` + etagToken(m.Checksum) + "-" + variant + "-w" + strconv.Itoa(width) + `"`
	}
	w.Header().Set("ETag", etag)
	if m.IsPublic() {
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d, immutable", s.cfg.PublicMaxAge))
	} else {
		w.Header().Set("Cache-Control", "private, max-age=3600")
	}
	w.Header().Set("X-Image-Id", m.ID)
	w.Header().Set("X-Image-Size-Bytes", strconv.FormatInt(m.SizeBytes, 10))
	w.Header().Set("X-Image-Size-KB", fmt.Sprintf("%.2f", m.SizeKB))
	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	if variant == "thumb" {
		if svg := mediaSwatch(m); svg != "" {
			w.Header().Set("Content-Type", "image/svg+xml")
			w.Header().Set("X-Delivery", "bg")
			w.Header().Set("Content-Length", strconv.Itoa(len(svg)))
			_, _ = io.WriteString(w, svg)
			s.trackView(m)
			return
		}
	}

	if s.gov.Busy() {
		if svg := mediaSwatch(m); svg != "" {
			w.Header().Set("Content-Type", "image/svg+xml")
			w.Header().Set("X-Delivery", "degraded-preview")
			w.Header().Set("Retry-After", "5")
			_, _ = io.WriteString(w, svg)
			s.trackView(m)
			return
		}
	}

	if b, ok := s.staging.Get(hotKey(m.ID, "hd")); ok {
		w.Header().Set("Content-Type", m.MimeType)
		w.Header().Set("Accept-Ranges", "bytes")
		setMediaSecurityHeaders(w, m)
		if rng := r.Header.Get("Range"); rng != "" {
			lo, hi, ok2 := parseRange(rng, int64(len(b)))
			if ok2 {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", lo, hi, len(b)))
				w.Header().Set("Content-Length", strconv.FormatInt(hi-lo+1, 10))
				w.Header().Set("X-Delivery", "ram-range")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(b[lo : hi+1])
				s.trackView(m)
				return
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(b)))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("X-Delivery", "ram")
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
		s.trackView(m)
		return
	}

	if width > 0 && pol.OnTheFlyResize && !m.IsStreamed() {
		key := hotKey(m.ID, "w"+strconv.Itoa(width))
		if b, ok := s.hot.Get(key); ok {
			writeVariant(w, "image/jpeg", b, "transform-cache")
			s.trackView(m)
			return
		}
		if s.rc.On() {
			if b, ok := s.rc.Get(s.varKey(m.ID, width)); ok {
				s.hot.Add(key, b)
				writeVariant(w, "image/jpeg", b, "redis")
				s.trackView(m)
				return
			}
		}
		release, ok := s.gov.AcquireHeavy(ctx)
		if ok {
			var raw []byte
			var ferr error
			if b, ok := s.staging.Get(hotKey(m.ID, "hd")); ok {
				raw = b
			} else {
				raw, ferr = s.vault.Fetch(ctx, m.VaultRef, 32<<20)
			}
			if ferr == nil && raw != nil {
				out, mime, err := TransformOnTheFly(raw, width, s.cfg.HDQuality)
				release()
				if err == nil {
					if mime == "" {
						mime = m.MimeType
					}
					s.hot.Add(key, out)
					s.rc.SetAsync(s.varKey(m.ID, width), out, s.rc.cfg.VariantTTL)
					writeVariant(w, mime, out, "transform")
					s.trackView(m)
					return
				}
			} else {
				release()
			}
		}
	}

	rc, hdr, status, err := s.vault.Open(ctx, m.VaultRef, r.Header.Get("Range"))
	if err != nil {
		if svg := mediaSwatch(m); svg != "" {
			w.Header().Set("Content-Type", "image/svg+xml")
			w.Header().Set("X-Delivery", "fallback-preview")
			_, _ = io.WriteString(w, svg)
			s.trackView(m)
			return
		}
		writeMissingImageStatus(w, http.StatusBadGateway)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", m.MimeType)
	w.Header().Set("Accept-Ranges", "bytes")
	setMediaSecurityHeaders(w, m)
	for _, k := range []string{"Content-Length", "Content-Range"} {
		if v := hdr.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.Header().Set("X-Delivery", "stream")
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)

	buf := streamBufPool.Get().([]byte)
	defer streamBufPool.Put(buf)
	_, _ = io.CopyBuffer(newFlushWriter(w), rc, buf)
	s.trackView(m)
}

func etagToken(sum string) string {
	if len(sum) >= 16 {
		return sum[:16]
	}
	if sum == "" {
		return "0000000000000000"
	}
	return sum
}

func writeVariant(w http.ResponseWriter, mime string, b []byte, how string) {
	if mime == "" {
		mime = "image/jpeg"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Delivery", how)
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	_, _ = w.Write(b)
}

var swatchPos = [4][2]string{{"0", "0"}, {"1", "0"}, {"0", "1"}, {"1", "1"}}

func mediaSwatch(m *ImageMeta) string {
	n := 0
	var body strings.Builder
	for _, c := range m.Colors {
		if n >= 4 || !validBg(c.Hex) {
			continue
		}
		body.WriteString(`<rect x="` + swatchPos[n][0] + `" y="` + swatchPos[n][1] + `" width="1" height="1" fill="` + c.Hex + `"/>`)
		n++
	}
	if n < 2 {
		return bgSwatch(m.Bg)
	}
	return `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8" viewBox="0 0 2 2" preserveAspectRatio="none">` +
		body.String() + `</svg>`
}

func bgSwatch(bg string) string {
	if !validBg(bg) {
		return ""
	}
	return `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8" fill="` + bg + `"/></svg>`
}

func (s *StorageService) trackView(m *ImageMeta) {
	s.viewMu.Lock()
	s.viewBuf[m.ID]++
	s.viewMu.Unlock()
}

func (s *StorageService) viewFlusher() {
	defer s.wg.Done()
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.viewStop:
			s.flushViews()
			return
		case <-t.C:
			s.flushViews()
		}
	}
}

func (s *StorageService) flushViews() {
	s.viewMu.Lock()
	buf := s.viewBuf
	s.viewBuf = make(map[string]int64, len(buf))
	s.viewMu.Unlock()
	if len(buf) == 0 {
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for id, n := range buf {
		s.meta.IncViewsBy(c, id, n)
	}
}

func setMediaSecurityHeaders(w http.ResponseWriter, m *ImageMeta) {
	switch m.MimeType {
	case MimeSVG:
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'; "+
				"font-src 'none'; script-src 'none'; sandbox")
		w.Header().Set("X-Content-Type-Options", "nosniff")
	}
	if m.IsStreamed() {
		w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
		w.Header().Set("Timing-Allow-Origin", "*")
	}
}

func parseRange(hdr string, size int64) (lo, hi int64, ok bool) {
	const pre = "bytes="
	if !strings.HasPrefix(hdr, pre) {
		return 0, 0, false
	}
	spec := strings.TrimSpace(strings.TrimPrefix(hdr, pre))
	if i := strings.IndexByte(spec, ','); i >= 0 {
		spec = strings.TrimSpace(spec[:i])
	}
	dash := strings.IndexByte(spec, '-')
	if dash < 0 {
		return 0, 0, false
	}
	startStr, endStr := strings.TrimSpace(spec[:dash]), strings.TrimSpace(spec[dash+1:])
	switch {
	case startStr == "" && endStr == "":
		return 0, 0, false
	case startStr == "":
		n, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		if n > size {
			n = size
		}
		return size - n, size - 1, true
	case endStr == "":
		n, err := strconv.ParseInt(startStr, 10, 64)
		if err != nil || n < 0 || n >= size {
			return 0, 0, false
		}
		return n, size - 1, true
	default:
		a, err1 := strconv.ParseInt(startStr, 10, 64)
		b, err2 := strconv.ParseInt(endStr, 10, 64)
		if err1 != nil || err2 != nil || a < 0 || b < a || a >= size {
			return 0, 0, false
		}
		if b >= size {
			b = size - 1
		}
		return a, b, true
	}
}

func (s *StorageService) SignShare(id string, expiresAt time.Time) string {
	return shareSignature(s.cfg.JWTSecret, id, expiresAt.Unix())
}

func shareSignature(secret []byte, id string, exp int64) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("vebox-share:v1:" + id + ":" + strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *StorageService) VerifyShare(id, expStr, sig string) bool {
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || exp <= 0 || time.Now().Unix() > exp {
		return false
	}
	want := shareSignature(s.cfg.JWTSecret, id, exp)
	return hmac.Equal([]byte(want), []byte(strings.TrimSpace(sig)))
}

func (s *StorageService) ShareLink(m *ImageMeta, ttl time.Duration) (url string, expiresAt time.Time) {
	expiresAt = time.Now().Add(ttl).UTC()
	sig := s.SignShare(m.ID, expiresAt)
	return s.cfg.LinkBase() + "/s/" + m.ID + "?exp=" +
		strconv.FormatInt(expiresAt.Unix(), 10) + "&sig=" + sig, expiresAt
}

func (s *StorageService) BulkDelete(ctx context.Context, userID string, ids []string) (deleted []string, failed []string) {
	for _, id := range ids {
		if err := s.Delete(ctx, userID, id); err != nil {
			failed = append(failed, id)
			continue
		}
		deleted = append(deleted, id)
	}
	return deleted, failed
}
