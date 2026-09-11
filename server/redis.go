package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// redisRetryEvery is how often we retry a Redis instance that was unreachable
// at start-up. Managed caches (Render, Upstash, …) are often still warming up
// while the web service boots, so a single failed dial must not disable the
// cache for the whole lifetime of the process.
const redisRetryEvery = 15 * time.Second

var ErrRedisDisabled = errors.New("redis disabled")

type RedisConfig struct {
	URL         string
	Prefix      string
	Budget      int64
	MaxValue    int64
	MetaTTL     time.Duration
	VariantTTL  time.Duration
	DialTimeout time.Duration
	OpTimeout   time.Duration
	PoolSize    int
}

func (c RedisConfig) On() bool { return c.URL != "" }

type redisAddr struct {
	host    string
	user    string
	pass    string
	db      int
	needTLS bool
}

func parseRedisURL(raw string) (redisAddr, error) {
	var out redisAddr
	s := strings.TrimSpace(raw)
	// Dashboards (Render, Heroku, …) often keep the quotes around the value.
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	if s == "" {
		return out, errors.New("empty url")
	}
	if !strings.Contains(s, "://") {
		s = "redis://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return out, err
	}
	switch u.Scheme {
	case "redis":
	case "rediss":
		out.needTLS = true
	default:
		return out, fmt.Errorf("redis: nodirma sxema %q", u.Scheme)
	}
	out.host = u.Host
	if out.host == "" {
		return out, errors.New("redis: host yo'q")
	}
	if _, _, e := net.SplitHostPort(out.host); e != nil {
		out.host += ":6379"
	}
	if u.User != nil {
		out.user = u.User.Username()
		if p, ok := u.User.Password(); ok {
			out.pass = p
		}
	}
	if p := strings.Trim(u.Path, "/"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n >= 0 {
			out.db = n
		}
	}
	return out, nil
}

type redisConn struct {
	c net.Conn
	r *bufio.Reader
}

func (c *redisConn) close() { _ = c.c.Close() }

type RedisCache struct {
	cfg RedisConfig
	ad  redisAddr

	pool chan *redisConn
	wbuf chan redisWrite

	enabled   atomic.Bool
	downUntil atomic.Int64

	hits    atomic.Int64
	misses  atomic.Int64
	sets    atomic.Int64
	skipped atomic.Int64
	errs    atomic.Int64
	used    atomic.Int64

	usedMemory   atomic.Int64
	serverMaxMem atomic.Int64
	policy       atomic.Value
	lastErr      atomic.Value
	startedAt    time.Time

	startOnce sync.Once
}

func NewRedisCache(ctx context.Context, cfg RedisConfig) *RedisCache {
	rc := &RedisCache{cfg: cfg, pool: make(chan *redisConn, cfg.PoolSize), wbuf: make(chan redisWrite, 128)}
	rc.policy.Store("")
	rc.lastErr.Store("")
	rc.startedAt = time.Now()
	if !cfg.On() {
		return rc
	}
	ad, err := parseRedisURL(cfg.URL)
	if err != nil {
		rc.noteErr(err)
		log.Printf("REDIS_URL noto'g'ri (%v) — L2 kesh butunlay o'chirildi", err)
		return rc
	}
	rc.ad = ad
	if rc.connect(ctx) {
		rc.startWorkers()
	}
	// Retry in the background even when the first dial failed: the cache may
	// simply not have been ready yet.
	go rc.reconnector(ctx)
	return rc
}

// connect dials once and marks the cache healthy on success. Safe to call
// repeatedly; it never spawns goroutines.
func (rc *RedisCache) connect(ctx context.Context) bool {
	c, err := rc.dial(ctx)
	if err != nil {
		rc.noteErr(err)
		return false
	}
	rc.release(c)
	rc.downUntil.Store(0)
	rc.enabled.Store(true)
	rc.lastErr.Store("")
	return true
}

// startWorkers launches the write-behind and stats goroutines exactly once,
// after the first successful connection.
func (rc *RedisCache) startWorkers() {
	rc.startOnce.Do(func() {
		go rc.writer()
		go rc.refresher()
	})
}

func (rc *RedisCache) reconnector(ctx context.Context) {
	t := time.NewTicker(redisRetryEvery)
	defer t.Stop()
	addr := safeAddr(rc.ad.host, rc.ad.needTLS)
	up := rc.On()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if !rc.cfg.On() {
				return
			}
			if !rc.On() || rc.paused() {
				if !rc.connect(ctx) {
					if up {
						up = false
						log.Printf("Redis bilan aloqa uzildi (%s): %s — ichki kesh bilan ishlashda davom etiladi",
							addr, rc.LastError())
					}
					continue
				}
				rc.startWorkers()
			}
			if !up {
				up = true
				log.Printf("Redis ulandi (%s) — L2 kesh yoqildi", addr)
			}
		}
	}
}

// LastError returns the most recent Redis error, or "" when there is none.
func (rc *RedisCache) LastError() string {
	if rc == nil {
		return ""
	}
	if e, ok := rc.lastErr.Load().(string); ok {
		return e
	}
	return ""
}

func (rc *RedisCache) On() bool { return rc != nil && rc.enabled.Load() }

func (rc *RedisCache) noteErr(err error) {
	if err == nil {
		return
	}
	rc.errs.Add(1)
	rc.lastErr.Store(err.Error())
	rc.downUntil.Store(time.Now().Add(20 * time.Second).UnixNano())
}

func (rc *RedisCache) paused() bool { return time.Now().UnixNano() < rc.downUntil.Load() }

func (rc *RedisCache) dial(ctx context.Context) (*redisConn, error) {
	d := net.Dialer{Timeout: rc.cfg.DialTimeout}
	cctx, cancel := context.WithTimeout(ctx, rc.cfg.DialTimeout)
	defer cancel()
	var raw net.Conn
	var err error
	if rc.ad.needTLS {
		dl := net.Dialer{Timeout: rc.cfg.DialTimeout}
		raw, err = tls.DialWithDialer(&dl, "tcp", rc.ad.host, &tls.Config{
			ServerName: tlsServerName(rc.ad.host),
			MinVersion: tls.VersionTLS12,
		})
	} else {
		raw, err = d.DialContext(cctx, "tcp", rc.ad.host)
	}
	if err != nil {
		return nil, err
	}
	c := &redisConn{c: raw, r: bufio.NewReader(raw)}
	if rc.ad.pass != "" {
		args := []string{"AUTH"}
		if rc.ad.user != "" {
			args = append(args, rc.ad.user)
		}
		args = append(args, rc.ad.pass)
		if _, err := c.do(rc.cfg.OpTimeout, args...); err != nil {
			c.close()
			return nil, err
		}
	}
	if rc.ad.db != 0 {
		if _, err := c.do(rc.cfg.OpTimeout, "SELECT", strconv.Itoa(rc.ad.db)); err != nil {
			c.close()
			return nil, err
		}
	}
	return c, nil
}

func tlsServerName(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}

func (rc *RedisCache) get() (*redisConn, error) {
	if !rc.On() || rc.paused() {
		return nil, ErrRedisDisabled
	}
	select {
	case c := <-rc.pool:
		if c != nil {
			return c, nil
		}
	default:
	}
	return rc.dial(context.Background())
}

func (rc *RedisCache) release(c *redisConn) {
	if c == nil {
		return
	}
	select {
	case rc.pool <- c:
	default:
		c.close()
	}
}

func (rc *RedisCache) Get(key string) ([]byte, bool) {
	if !rc.On() || rc.paused() {
		return nil, false
	}
	c, err := rc.get()
	if err != nil {
		rc.noteErr(err)
		return nil, false
	}
	v, err := c.do(rc.cfg.OpTimeout, "GET", rc.cfg.Prefix+key)
	if err != nil {
		c.close()
		rc.noteErr(err)
		return nil, false
	}
	rc.release(c)
	b, ok := v.([]byte)
	if !ok || len(b) == 0 {
		rc.misses.Add(1)
		return nil, false
	}
	rc.hits.Add(1)
	return b, true
}

func (rc *RedisCache) Set(key string, val []byte, ttl time.Duration) {
	if !rc.On() || rc.paused() {
		return
	}
	size := int64(len(key) + len(val) + 96)
	if rc.cfg.MaxValue > 0 && int64(len(val)) > rc.cfg.MaxValue {
		rc.skipped.Add(1)
		return
	}
	if rc.cfg.Budget > 0 && rc.used.Load()+size > rc.cfg.Budget {
		if m := rc.usedMemory.Load(); m > 0 && m < rc.used.Load() {
			rc.used.Store(m)
		} else {
			rc.skipped.Add(1)
			return
		}
	}
	if rc.usedMemory.Load() > 0 && rc.cfg.Budget > 0 && rc.usedMemory.Load() > rc.cfg.Budget*9/10 {
		rc.skipped.Add(1)
		return
	}
	c, err := rc.get()
	if err != nil {
		rc.noteErr(err)
		return
	}
	secs := int(ttl / time.Second)
	if secs < 60 {
		secs = 60
	}
	_, err = c.do(rc.cfg.OpTimeout, "SET", rc.cfg.Prefix+key, string(val), "EX", strconv.Itoa(secs))
	if err != nil {
		c.close()
		rc.noteErr(err)
		return
	}
	rc.release(c)
	rc.used.Add(size)
	rc.sets.Add(1)
}

type redisWrite struct {
	key string
	val []byte
	ttl time.Duration
}

func (rc *RedisCache) SetAsync(key string, val []byte, ttl time.Duration) {
	if !rc.On() || rc.paused() {
		return
	}
	select {
	case rc.wbuf <- redisWrite{key: key, val: val, ttl: ttl}:
	default:
		rc.skipped.Add(1)
	}
}

func (rc *RedisCache) writer() {
	for w := range rc.wbuf {
		rc.Set(w.key, w.val, w.ttl)
	}
}

func (rc *RedisCache) Del(keys ...string) {
	if !rc.On() || rc.paused() || len(keys) == 0 {
		return
	}
	args := make([]string, 0, len(keys)+1)
	args = append(args, "DEL")
	for _, k := range keys {
		args = append(args, rc.cfg.Prefix+k)
	}
	c, err := rc.get()
	if err != nil {
		rc.noteErr(err)
		return
	}
	_, err = c.do(rc.cfg.OpTimeout, args...)
	if err != nil {
		c.close()
		rc.noteErr(err)
		return
	}
	rc.release(c)
}

func (rc *RedisCache) FlushNamespace() {
	if !rc.On() || rc.paused() {
		return
	}
	c, err := rc.get()
	if err != nil {
		rc.noteErr(err)
		return
	}
	defer rc.release(c)
	cursor := "0"
	removed := int64(0)
	for i := 0; i < 64; i++ {
		v, err := c.do(rc.cfg.OpTimeout, "SCAN", cursor, "MATCH", rc.cfg.Prefix+"*", "COUNT", "200")
		if err != nil {
			rc.noteErr(err)
			return
		}
		arr, ok := v.([]any)
		if !ok || len(arr) != 2 {
			return
		}
		next := redisString(arr[0])
		items, ok := arr[1].([]any)
		if !ok {
			break
		}
		if len(items) > 0 {
			args := make([]string, 0, len(items)+1)
			args = append(args, "DEL")
			for _, it := range items {
				args = append(args, redisString(it))
			}
			if _, err := c.do(rc.cfg.OpTimeout, args...); err != nil {
				rc.noteErr(err)
				return
			}
			removed += int64(len(items))
		}
		if next == "" || next == "0" {
			break
		}
		cursor = next
	}
	if removed > 0 {
		rc.used.Store(0)
	}
}

func redisString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	}
	return ""
}

func (rc *RedisCache) Info(ctx context.Context) (map[string]string, error) {
	c, err := rc.get()
	if err != nil {
		return nil, err
	}
	v, err := c.doCtx(ctx, rc.cfg.OpTimeout, "INFO", "memory")
	if err != nil {
		c.close()
		rc.noteErr(err)
		return nil, err
	}
	rc.release(c)
	txt, ok := v.(string)
	if !ok {
		if b, ok2 := v.([]byte); ok2 {
			txt = string(b)
		}
	}
	out := map[string]string{}
	for _, line := range strings.Split(txt, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, val, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		out[k] = strings.TrimSpace(val)
	}
	return out, nil
}

func (rc *RedisCache) refresher() {
	t := time.NewTicker(45 * time.Second)
	defer t.Stop()
	for range t.C {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		info, err := rc.Info(ctx)
		cancel()
		if err != nil {
			continue
		}
		if v, ok := info["used_memory"]; ok {
			n, _ := strconv.ParseInt(v, 10, 64)
			rc.usedMemory.Store(n)
		}
		if v, ok := info["maxmemory"]; ok {
			n, _ := strconv.ParseInt(v, 10, 64)
			rc.serverMaxMem.Store(n)
		}
		if v, ok := info["maxmemory_policy"]; ok {
			rc.policy.Store(v)
		}
	}
}

func (rc *RedisCache) Stats() map[string]any {
	if rc == nil {
		return map[string]any{"enabled": false}
	}
	out := map[string]any{
		"enabled":          rc.On(),
		"addr":             safeAddr(rc.ad.host, rc.ad.needTLS),
		"connected":        rc.On() && !rc.paused(),
		"hits":             rc.hits.Load(),
		"misses":           rc.misses.Load(),
		"sets":             rc.sets.Load(),
		"skipped":          rc.skipped.Load(),
		"errors":           rc.errs.Load(),
		"budget_bytes":     rc.cfg.Budget,
		"cached_bytes":     rc.used.Load(),
		"used_memory":      rc.usedMemory.Load(),
		"server_maxmemory": rc.serverMaxMem.Load(),
		"policy":           rc.policy.Load(),
		"meta_ttl":         rc.cfg.MetaTTL.String(),
		"variant_ttl":      rc.cfg.VariantTTL.String(),
		"max_value_bytes":  rc.cfg.MaxValue,
		"uptime":           time.Since(rc.startedAt).Round(time.Second).String(),
	}
	if e, ok := rc.lastErr.Load().(string); ok && e != "" {
		out["last_error"] = e
	}
	if rc.hits.Load()+rc.misses.Load() > 0 {
		out["hit_rate"] = round2(float64(rc.hits.Load()) / float64(rc.hits.Load()+rc.misses.Load()) * 100)
	}
	return out
}

func safeAddr(host string, tlsOn bool) string {
	scheme := "redis://"
	if tlsOn {
		scheme = "rediss://"
	}
	return scheme + host
}

func (c *redisConn) do(timeout time.Duration, args ...string) (any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return c.doCtx(ctx, timeout, args...)
}

func (c *redisConn) doCtx(ctx context.Context, timeout time.Duration, args ...string) (any, error) {
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	if err := c.c.SetDeadline(deadline); err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("*" + strconv.Itoa(len(args)) + "\r\n")
	for _, a := range args {
		b.WriteString("$" + strconv.Itoa(len(a)) + "\r\n" + a + "\r\n")
	}
	if _, err := c.c.Write([]byte(b.String())); err != nil {
		return nil, err
	}
	return readReply(c.r)
}

func readReply(r *bufio.Reader) (any, error) {
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	if line == "" {
		return nil, errors.New("redis: javob bo'sh")
	}
	switch line[0] {
	case '+':
		return line[1:], nil
	case '-':
		return nil, errors.New("redis: " + line[1:])
	case ':':
		n, err := strconv.ParseInt(line[1:], 10, 64)
		if err != nil {
			return nil, err
		}
		return n, nil
	case '$':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err := readFull(r, buf); err != nil {
			return nil, err
		}
		return buf[:n], nil
	case '*':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			v, err := readReply(r)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	}
	return nil, fmt.Errorf("redis: kutilmagan javob %q", line)
}

func readLine(r *bufio.Reader) (string, error) {
	s, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(s, "\r\n"), nil
}

func readFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
