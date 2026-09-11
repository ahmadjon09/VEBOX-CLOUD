package main

import "time"

type Tier string

const (
	TierFree     Tier = "free"
	TierPro      Tier = "pro"
	TierBusiness Tier = "business"
)

func (t Tier) Valid() bool {
	switch t {
	case TierFree, TierPro, TierBusiness:
		return true
	}
	return false
}

type User struct {
	ID         string    `json:"id"`
	Email      string    `json:"email"`
	Name       string    `json:"name"`
	AvatarURL  string    `json:"avatar_url"`
	Provider   string    `json:"provider"`
	ProviderID string    `json:"provider_id"`
	IsAdmin    bool      `json:"is_admin"`
	Banned     bool      `json:"banned"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	PasswordHash string `json:"-"`
}

func (u *User) HasPassword() bool { return u.PasswordHash != "" }

type APIKey struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Hash       string     `json:"-"`
	Revoked    bool       `json:"revoked"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type Subscription struct {
	UserID    string     `json:"user_id"`
	Tier      Tier       `json:"tier"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	CustomRatePerMin   *int   `json:"custom_rate_per_min,omitempty"`
	CustomBurst        *int   `json:"custom_burst,omitempty"`
	CustomMaxUploadKB  *int64 `json:"custom_max_upload_kb,omitempty"`
	CustomMaxAudioKB   *int64 `json:"custom_max_audio_kb,omitempty"`
	CustomStorageMB    *int64 `json:"custom_storage_mb,omitempty"`
	CustomMonthlyFiles *int64 `json:"custom_monthly_files,omitempty"`

	StorageRules map[string]any `json:"storage_rules,omitempty"`

	UpdatedAt time.Time `json:"updated_at"`
}

type UsageCounter struct {
	UserID       string `json:"user_id"`
	StorageBytes int64  `json:"storage_bytes"`
	FileCount    int64  `json:"file_count"`
	MonthKey     string `json:"month_key"`
	MonthFiles   int64  `json:"month_files"`
	MonthBytes   int64  `json:"month_bytes"`
}

type StatusDay struct {
	Date      string  `bson:"_id" json:"date"`
	Total     int64   `bson:"total" json:"total"`
	Up        int64   `bson:"up" json:"up"`
	UptimePct float64 `bson:"-" json:"uptime_pct"`
}

type StatusIncident struct {
	ID        string     `bson:"_id" json:"id"`
	Component string     `bson:"component" json:"component"`
	Summary   string     `bson:"summary" json:"summary"`
	StartedAt time.Time  `bson:"started_at" json:"started_at"`
	EndedAt   *time.Time `bson:"ended_at" json:"ended_at,omitempty"`
}

type ImageMeta struct {
	ID         string      `bson:"_id" json:"id"`
	UserID     string      `bson:"user_id" json:"user_id"`
	Filename   string      `bson:"filename" json:"filename"`
	MimeType   string      `bson:"mime_type" json:"mime_type"`
	Kind       string      `bson:"kind" json:"kind"`
	SizeBytes  int64       `bson:"size_bytes" json:"size_bytes"`
	SizeKB     float64     `bson:"size_kb" json:"size_kb"`
	Width      int         `bson:"width" json:"width"`
	Height     int         `bson:"height" json:"height"`
	Duration   float64     `bson:"duration" json:"duration"`
	Bg         string      `bson:"bg,omitempty" json:"bg,omitempty"`
	Colors     []ColorInfo `bson:"colors,omitempty" json:"colors,omitempty"`
	Checksum   string      `bson:"checksum" json:"checksum"`
	Visibility string      `bson:"visibility" json:"visibility"`
	Tier       Tier        `bson:"tier" json:"tier"`
	Views      int64       `bson:"views" json:"views"`
	CreatedAt  time.Time   `bson:"created_at" json:"created_at"`

	VaultRef string `bson:"vault_ref" json:"-"`
}

type ImageURLs struct {
	Original  string `json:"original"`
	HD        string `json:"hd"`
	Thumbnail string `json:"thumbnail"`
	CDN       string `json:"cdn"`
	Preview   string `json:"preview"`
	Download  string `json:"download"`
}

type PublicImage struct {
	ID          string      `json:"id"`
	Filename    string      `json:"filename"`
	MimeType    string      `json:"mime_type"`
	Kind        string      `json:"kind"`
	SizeBytes   int64       `json:"size_bytes"`
	SizeKB      float64     `json:"size_kb"`
	Width       int         `json:"width"`
	Height      int         `json:"height"`
	Duration    float64     `json:"duration,omitempty"`
	DurationFmt string      `json:"duration_fmt,omitempty"`
	Checksum    string      `json:"checksum"`
	Visibility  string      `json:"visibility"`
	Views       int64       `json:"views"`
	CreatedAt   time.Time   `json:"created_at"`
	Bg          string      `json:"bg,omitempty"`
	Colors      []ColorInfo `json:"colors,omitempty"`
	URLs        ImageURLs   `json:"urls"`
}

type MediaLink struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Visibility string    `json:"visibility"`
	URLs       ImageURLs `json:"urls"`
}

const (
	VisPublic  = "public"
	VisPrivate = "private"
)

func validVisibility(v string) bool { return v == VisPublic || v == VisPrivate }

func (m *ImageMeta) IsPublic() bool { return m.Visibility == VisPublic }

func (m *ImageMeta) URLs(baseURL string) ImageURLs {
	return ImageURLs{
		Original:  baseURL + "/i/" + m.ID,
		HD:        baseURL + "/i/" + m.ID + "?v=hd",
		Thumbnail: baseURL + "/i/" + m.ID + "?v=thumb",
		CDN:       baseURL + "/cdn/" + m.ID,
		Preview:   baseURL + "/preview/" + m.ID,
		Download:  baseURL + "/d/" + m.ID,
	}
}

func (m *ImageMeta) Public(baseURL string) PublicImage {
	p := PublicImage{
		ID: m.ID, Filename: m.Filename, MimeType: m.MimeType, Kind: m.Kind,
		SizeBytes: m.SizeBytes, SizeKB: m.SizeKB,
		Width: m.Width, Height: m.Height, Duration: m.Duration,
		Checksum: m.Checksum, Visibility: m.Visibility, Views: m.Views,
		CreatedAt: m.CreatedAt, Bg: m.Bg, Colors: m.Colors,
	}
	if p.Kind == "" {
		p.Kind = string(KindOfMime(m.MimeType))
		if p.Kind == "" {
			p.Kind = string(KindImage)
		}
	}
	if p.Visibility == "" {
		p.Visibility = VisPrivate
	}
	if m.Duration > 0 {
		p.DurationFmt = FormatDuration(m.Duration)
	}
	p.URLs = m.URLs(baseURL)
	return p
}

func (m *ImageMeta) Link(baseURL string) MediaLink {
	kind := m.Kind
	if kind == "" {
		kind = string(KindOfMime(m.MimeType))
		if kind == "" {
			kind = string(KindImage)
		}
	}
	vis := m.Visibility
	if vis == "" {
		vis = VisPrivate
	}
	return MediaLink{ID: m.ID, Kind: kind, Visibility: vis, URLs: m.URLs(baseURL)}
}

func (m *ImageMeta) IsVideo() bool {
	if m.Kind != "" {
		return m.Kind == string(KindVideo)
	}
	return IsVideoMime(m.MimeType)
}

func (m *ImageMeta) IsAudio() bool {
	if m.Kind != "" {
		return m.Kind == string(KindAudio)
	}
	return IsAudioMime(m.MimeType)
}

func (m *ImageMeta) IsStreamed() bool { return m.IsVideo() || m.IsAudio() }
