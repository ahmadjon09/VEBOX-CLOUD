package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const Version = "2.0.0"

type Config struct {
	Port         string
	PublicURL    string
	WebURL       string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	PostgresDSN string
	MongoURI    string
	MongoDB     string
	StrictDB    bool

	JWTSecret          []byte
	JWTTTL             time.Duration
	GithubClientID     string
	GithubClientSecret string
	AdminEmails        []string

	VaultTokens   []string
	VaultChatID   string
	FloodCooldown time.Duration
	VaultTimeout  time.Duration
	FileLinkTTL   time.Duration

	MemLimitBytes uint64
	MemHighWater  float64
	MemCritical   float64
	MemTick       time.Duration
	MaxHeavyJobs  int

	HDMaxWidth   int
	HDQuality    int
	AllowAudio   bool
	MaxAudioMB   int
	PaletteCount int
	ShareTTL     time.Duration

	LRUSize int

	RedisURL         string
	RedisPrefix      string
	RedisMaxMB       int
	RedisMaxValueKB  int
	RedisMetaTTL     time.Duration
	RedisVariantTTL  time.Duration
	RedisOpTimeout   time.Duration
	RedisDialTimeout time.Duration

	PublicRatePerMin int
	PublicBurst      int
	PublicMaxAge     int
	DefaultVisible   string

	DemoMode     bool
	HasPublicURL bool

	KeepAliveEnabled  bool
	KeepAliveInterval time.Duration

	CORSOrigins []string

	HotlinkProtect bool
	HotlinkOrigins []string
	HotlinkHosts   []string
}

func LoadConfig() *Config {
	loadDotEnv(env("ENV_FILE", ".env"))

	c := &Config{
		Port:               env("PORT", "8080"),
		PublicURL:          strings.TrimRight(env("PUBLIC_URL", ""), "/"),
		WebURL:             strings.TrimRight(env("WEB_URL", ""), "/"),
		ReadTimeout:        envDur("READ_TIMEOUT", 300*time.Second),
		WriteTimeout:       envDur("WRITE_TIMEOUT", 180*time.Second),
		PostgresDSN:        env("POSTGRES_DSN", ""),
		MongoURI:           env("MONGO_URI", ""),
		MongoDB:            env("MONGO_DB", "vebox"),
		StrictDB:           envBool("STRICT_DB", false),
		JWTTTL:             envDur("JWT_TTL", 24*time.Hour),
		GithubClientID:     env("GITHUB_CLIENT_ID", ""),
		GithubClientSecret: env("GITHUB_CLIENT_SECRET", ""),
		AdminEmails:        envList("ADMIN_EMAILS"),
		VaultTokens:        envList("VAULT_TOKENS"),
		VaultChatID:        env("VAULT_CHAT_ID", ""),
		FloodCooldown:      envDur("FLOOD_COOLDOWN", 60*time.Second),
		VaultTimeout:       envDur("VAULT_TIMEOUT", 60*time.Second),
		FileLinkTTL:        envDur("FILE_LINK_TTL", 50*time.Minute),
		MemLimitBytes:      uint64(envInt("MEM_LIMIT_MB", 0)) * 1024 * 1024,
		MemHighWater:       envFloat("MEM_HIGH_WATER", 0.80),
		MemCritical:        envFloat("MEM_CRITICAL", 0.92),
		MemTick:            envDur("MEM_TICK", time.Second),
		MaxHeavyJobs:       envInt("MAX_HEAVY_JOBS", 0),
		HDMaxWidth:         envInt("HD_MAX_WIDTH", 1920),
		HDQuality:          envInt("HD_QUALITY", 82),
		AllowAudio:         envBool("ALLOW_AUDIO", true),
		MaxAudioMB:         envInt("MAX_AUDIO_MB", int(MaxAudioSizeMB)),
		PaletteCount:       clampInt(envInt("PALETTE_COLORS", paletteSize), 1, 8),
		ShareTTL:           envDur("SHARE_TTL", 24*time.Hour),
		LRUSize:            envInt("LRU_SIZE", 50000),
		RedisURL:           env("REDIS_URL", ""),
		RedisPrefix:        env("REDIS_PREFIX", "vb:"),
		RedisMaxMB:         envInt("REDIS_MAX_MB", 30),
		RedisMaxValueKB:    envInt("REDIS_MAX_VALUE_KB", 96),
		RedisMetaTTL:       envDur("REDIS_META_TTL", 24*time.Hour),
		RedisVariantTTL:    envDur("REDIS_VARIANT_TTL", 6*time.Hour),
		RedisOpTimeout:     envDur("REDIS_OP_TIMEOUT", 200*time.Millisecond),
		RedisDialTimeout:   envDur("REDIS_DIAL_TIMEOUT", 2*time.Second),
		PublicRatePerMin:   envInt("PUBLIC_RATE_PER_MIN", 600),
		PublicBurst:        envInt("PUBLIC_BURST", 200),
		PublicMaxAge:       envInt("PUBLIC_MAX_AGE", 31536000),
		DefaultVisible:     strings.ToLower(env("DEFAULT_VISIBILITY", "private")),
		KeepAliveEnabled:   envBool("KEEPALIVE", true),
		KeepAliveInterval:  envDur("KEEPALIVE_INTERVAL", 10*time.Minute),
		CORSOrigins:        envList("CORS_ORIGINS"),
		HotlinkProtect:     envBool("HOTLINK_PROTECT", true),
		HotlinkOrigins:     envList("HOTLINK_ALLOWED_ORIGINS"),
	}
	if c.WebURL != "" && !containsStr(c.CORSOrigins, c.WebURL) {
		c.CORSOrigins = append(c.CORSOrigins, c.WebURL)
	}
	if c.DefaultVisible != "public" {
		c.DefaultVisible = "private"
	}
	if c.ShareTTL < time.Minute {
		c.ShareTTL = time.Minute
	}
	if c.ShareTTL > 30*24*time.Hour {
		c.ShareTTL = 30 * 24 * time.Hour
	}
	if !strings.HasSuffix(c.RedisPrefix, ":") {
		c.RedisPrefix += ":"
	}
	if c.RedisMaxMB < 1 {
		c.RedisMaxMB = 30
	}
	if c.RedisMaxValueKB < 1 {
		c.RedisMaxValueKB = 1
	}

	c.DemoMode = envBool("DEMO_MODE", false) || (c.PostgresDSN == "" && c.MongoURI == "")

	secret := env("JWT_SECRET", "")
	switch {
	case secret != "" && len(secret) < 32:
		log.Fatalf("JWT_SECRET juda qisqa (%d belgi) — kamida 32 belgi bo'lsin", len(secret))
	case secret == "" && !c.DemoMode:
		log.Fatal("JWT_SECRET o'rnatilmagan — production rejimda majburiy (openssl rand -hex 32)")
	case secret == "":
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		secret = hex.EncodeToString(b)
		log.Println("JWT_SECRET yo'q — demo uchun tasodifiy kalit ishlatilmoqda")
	}
	c.JWTSecret = []byte(secret)

	c.HasPublicURL = c.PublicURL != ""
	if c.HasPublicURL {
		u, err := url.Parse(c.PublicURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" {
			log.Fatalf("PUBLIC_URL noto'g'ri: %q — masalan https://api.example.com", c.PublicURL)
		}
	} else {
		c.PublicURL = "http://localhost:" + c.Port
		if c.GithubClientID != "" {
			log.Println("DIQQAT: OAuth sozlangan, lekin PUBLIC_URL yo'q — callback manzili localhost bo'ladi")
		}
	}

	c.HotlinkHosts = hotlinkHosts(c)
	return c
}

func (c *Config) Redis() RedisConfig {
	return RedisConfig{
		URL:         c.RedisURL,
		Prefix:      c.RedisPrefix,
		Budget:      int64(c.RedisMaxMB) * 1024 * 1024,
		MaxValue:    int64(c.RedisMaxValueKB) * 1024,
		MetaTTL:     c.RedisMetaTTL,
		VariantTTL:  c.RedisVariantTTL,
		DialTimeout: c.RedisDialTimeout,
		OpTimeout:   c.RedisOpTimeout,
		PoolSize:    4,
	}
}

func (c *Config) RequestTimeout() time.Duration {
	t := c.ReadTimeout
	if t <= 0 {
		t = 300 * time.Second
	}
	if t < 60*time.Second {
		t = 60 * time.Second
	}
	return t + 30*time.Second
}

func (c *Config) LinkBase() string {
	if c.HasPublicURL {
		return c.PublicURL
	}
	return ""
}

func (c *Config) IsHTTPS() bool { return strings.HasPrefix(c.PublicURL, "https://") }

func (c *Config) OAuthEnabled() bool { return c.GithubClientID != "" }

func hotlinkHosts(c *Config) []string {
	out := []string{}
	add := func(raw string) {
		if h := urlHost(raw); h != "" && !containsStr(out, h) {
			out = append(out, h)
		}
	}
	add(c.PublicURL)
	add(c.WebURL)
	for _, o := range c.CORSOrigins {
		add(o)
	}
	for _, o := range c.HotlinkOrigins {
		add(o)
	}
	return out
}

func urlHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		} else if i := strings.Index(v, " #"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		if k == "" {
			continue
		}
		if _, exists := os.LookupEnv(k); exists {
			continue
		}
		_ = os.Setenv(k, v)
		n++
	}
	if n > 0 {
		log.Printf("%s dan %d ta sozlama yuklandi", path, n)
	}
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envBool(k string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(k))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func envList(k string) []string {
	parts := strings.FieldsFunc(os.Getenv(k), func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == ' '
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envInt(k string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(k))); err == nil {
		return n
	}
	return def
}

func envInt64(key string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func envFloat(k string, def float64) float64 {
	if f, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(k)), 64); err == nil {
		return f
	}
	return def
}

func envDur(k string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(k))); err == nil {
		return d
	}
	return def
}
