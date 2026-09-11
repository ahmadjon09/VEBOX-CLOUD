package main

import (
	"strconv"
	"sync"
	"time"
)

const (
	MaxImageSizeMB int64 = 50
	MaxAudioSizeMB int64 = 50

	MaxImageSizeKB int64 = MaxImageSizeMB * 1024
	MaxAudioSizeKB int64 = MaxAudioSizeMB * 1024

	MaxFileSizeBytes int64 = MaxImageSizeMB * 1024 * 1024
)

func kindCeilingKB(kind MediaKind) int64 {
	if kind == KindAudio {
		return MaxAudioSizeKB
	}
	return MaxImageSizeKB
}

type TierPolicy struct {
	Name             Tier  `json:"tier"`
	RatePerMin       int   `json:"rate_per_min"`
	Burst            int   `json:"burst"`
	MaxUploadKB      int64 `json:"max_upload_kb"`
	MaxAudioKB       int64 `json:"max_audio_kb"`
	AllowAudio       bool  `json:"allow_audio"`
	StorageMB        int64 `json:"storage_mb"`
	MonthlyFiles     int64 `json:"monthly_files"`
	MaxHDWidth       int   `json:"max_hd_width"`
	HDProcessing     bool  `json:"hd_processing"`
	Priority         int   `json:"priority"`
	OnTheFlyResize   bool  `json:"on_the_fly_resize"`
	Analytics        bool  `json:"analytics"`
	UnlimitedTraffic bool  `json:"unlimited_traffic"`
	Customizable     bool  `json:"customizable"`
}

func (p TierPolicy) MaxBytesFor(kind MediaKind) int64 {
	ceiling := kindCeilingKB(kind)
	limitKB := p.MaxUploadKB
	if kind == KindAudio && p.MaxAudioKB > 0 {
		limitKB = p.MaxAudioKB
	}
	if limitKB <= 0 || limitKB > ceiling {
		limitKB = ceiling
	}
	return limitKB * 1024
}

func publicPolicy() TierPolicy {
	return TierPolicy{
		Name: TierFree, RatePerMin: 120, Burst: 60,
		MaxUploadKB: MaxImageSizeKB, MaxAudioKB: MaxAudioSizeKB,
		AllowAudio: true,
		StorageMB:  0, MonthlyFiles: 0, MaxHDWidth: 4096, HDProcessing: true, Priority: 0,
		OnTheFlyResize: true, Analytics: true, UnlimitedTraffic: true,
	}
}

var DefaultPolicies = map[Tier]TierPolicy{
	TierFree: publicPolicy(),
	TierPro:  publicPolicy(),
	TierBusiness: {
		Name: TierBusiness, RatePerMin: 20000, Burst: 10000,
		MaxUploadKB: MaxImageSizeKB, MaxAudioKB: MaxAudioSizeKB,
		AllowAudio: true,
		StorageMB:  0, MonthlyFiles: 0, MaxHDWidth: 8192, HDProcessing: true, Priority: 2,
		OnTheFlyResize: true, Analytics: true, UnlimitedTraffic: true, Customizable: true,
	},
}

var TierPrice = map[Tier]string{
	TierFree:     "0",
	TierPro:      "0",
	TierBusiness: "0",
}

func EffectivePolicy(s *Subscription) TierPolicy {
	tier := TierFree
	if s != nil && s.Tier == TierBusiness {
		tier = TierBusiness
	}
	if s != nil && s.ExpiresAt != nil && time.Now().After(*s.ExpiresAt) {
		tier = TierFree
	}
	p := DefaultPolicies[tier]
	if s == nil {
		return p
	}
	if s.CustomRatePerMin != nil && *s.CustomRatePerMin > 0 {
		p.RatePerMin = *s.CustomRatePerMin
	}
	if s.CustomBurst != nil && *s.CustomBurst > 0 {
		p.Burst = *s.CustomBurst
	}
	if s.CustomMaxUploadKB != nil && *s.CustomMaxUploadKB > 0 {
		p.MaxUploadKB = *s.CustomMaxUploadKB
	}
	if s.CustomMaxAudioKB != nil && *s.CustomMaxAudioKB > 0 {
		p.MaxAudioKB = *s.CustomMaxAudioKB
	}
	if s.CustomStorageMB != nil && *s.CustomStorageMB > 0 {
		p.StorageMB = *s.CustomStorageMB
	}
	if s.CustomMonthlyFiles != nil && *s.CustomMonthlyFiles > 0 {
		p.MonthlyFiles = *s.CustomMonthlyFiles
	}
	if v, ok := s.StorageRules["max_hd_width"].(float64); ok && v > 0 {
		p.MaxHDWidth = int(v)
	}
	if v, ok := s.StorageRules["hd_processing"].(bool); ok {
		p.HDProcessing = v
	}
	if v, ok := s.StorageRules["on_the_fly_resize"].(bool); ok {
		p.OnTheFlyResize = v
	}
	if v, ok := s.StorageRules["allow_audio"].(bool); ok {
		p.AllowAudio = v
	}
	if v, ok := s.StorageRules["unlimited_traffic"].(bool); ok {
		p.UnlimitedTraffic = v
	}
	if p.MaxUploadKB <= 0 || p.MaxUploadKB > MaxImageSizeKB {
		p.MaxUploadKB = MaxImageSizeKB
	}
	if p.MaxAudioKB <= 0 || p.MaxAudioKB > MaxAudioSizeKB {
		p.MaxAudioKB = MaxAudioSizeKB
	}
	return p
}

func humanBytes(n int64) string {
	if n <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if v < 10 && i > 0 {
		return strconv.FormatFloat(v, 'f', 1, 64) + " " + units[i]
	}
	return strconv.FormatFloat(v, 'f', 0, 64) + " " + units[i]
}

type QuotaError struct {
	Code    string
	Message string
	Limit   int64
	Used    int64
}

func (e *QuotaError) Error() string { return e.Message }

func CheckUploadQuota(p TierPolicy, u *UsageCounter, sizeBytes int64, kind MediaKind) *QuotaError {
	if kind == KindAudio && !p.AllowAudio {
		return &QuotaError{Code: "audio_not_allowed",
			Message: "Bu tarifda audio yuklash mavjud emas",
			Limit:   0, Used: sizeBytes}
	}
	if limit := p.MaxBytesFor(kind); sizeBytes > limit {
		return &QuotaError{Code: "file_too_large",
			Message: "Fayl hajmi tarif limitidan oshib ketdi (" + kindName(kind) + " uchun " +
				humanBytes(limit) + ")",
			Limit: limit, Used: sizeBytes}
	}
	if p.StorageMB > 0 && u != nil && u.StorageBytes+sizeBytes > p.StorageMB*1024*1024 {
		return &QuotaError{Code: "storage_quota_exceeded",
			Message: "Saqlash kvotasi tugadi",
			Limit:   p.StorageMB * 1024 * 1024, Used: u.StorageBytes}
	}
	if p.MonthlyFiles > 0 && u != nil && u.MonthFiles+1 > p.MonthlyFiles {
		return &QuotaError{Code: "monthly_limit_exceeded",
			Message: "Oylik yuklash limiti tugadi",
			Limit:   p.MonthlyFiles, Used: u.MonthFiles}
	}
	return nil
}

func kindName(kind MediaKind) string {
	if kind == KindAudio {
		return "audio"
	}
	return "rasm"
}

type bucket struct {
	tokens   float64
	capacity float64
	refill   float64
	last     time.Time
}

type RateLimiter struct {
	shards [64]struct {
		mu sync.Mutex
		m  map[string]*bucket
	}
}

func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{}
	for i := range rl.shards {
		rl.shards[i].m = make(map[string]*bucket)
	}
	go rl.gc()
	return rl
}

func shardIdx(key string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return h % 64
}

func (rl *RateLimiter) Allow(key string, ratePerMin, burst int) (bool, int, time.Duration) {
	if ratePerMin <= 0 {
		ratePerMin = 60
	}
	if burst <= 0 {
		burst = ratePerMin
	}
	s := &rl.shards[shardIdx(key)]
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	b, ok := s.m[key]
	if !ok {
		b = &bucket{tokens: float64(burst), capacity: float64(burst),
			refill: float64(ratePerMin) / 60.0, last: now}
		s.m[key] = b
	}
	b.capacity = float64(burst)
	b.refill = float64(ratePerMin) / 60.0

	elapsed := now.Sub(b.last).Seconds()
	b.last = now
	b.tokens += elapsed * b.refill
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	if b.tokens >= 1 {
		b.tokens--
		return true, int(b.tokens), 0
	}
	need := (1 - b.tokens) / b.refill
	return false, 0, time.Duration(need * float64(time.Second))
}

func (rl *RateLimiter) gc() {
	t := time.NewTicker(5 * time.Minute)
	for range t.C {
		cutoff := time.Now().Add(-15 * time.Minute)
		for i := range rl.shards {
			s := &rl.shards[i]
			s.mu.Lock()
			for k, b := range s.m {
				if b.last.Before(cutoff) {
					delete(s.m, k)
				}
			}
			s.mu.Unlock()
		}
	}
}

func TierSnapshot(p TierPolicy, u *UsageCounter) map[string]any {
	used, files := int64(0), int64(0)
	var monthFiles int64
	if u != nil {
		used, files, monthFiles = u.StorageBytes, u.FileCount, u.MonthFiles
	}
	return map[string]any{
		"tier":   p.Name,
		"limits": p,
		"usage": map[string]any{
			"storage_bytes":       used,
			"storage_kb":          round2(float64(used) / 1024),
			"storage_quota_bytes": p.StorageMB * 1024 * 1024,
			"file_count":          files,
			"month_files":         monthFiles,
			"month_files_quota":   p.MonthlyFiles,
		},
	}
}
