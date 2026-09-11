package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ErrNotFound = errors.New("not found")

type CoreStore interface {
	UpsertOAuthUser(ctx context.Context, u *User) (*User, error)
	GetUser(ctx context.Context, id string) (*User, error)
	ListUsers(ctx context.Context, limit, offset int) ([]User, error)
	SetAdmin(ctx context.Context, userID string, admin bool) error
	SetBanned(ctx context.Context, userID string, banned bool) error
	UpdateProfile(ctx context.Context, userID, name, avatarURL string) (*User, error)
	SetPasswordHash(ctx context.Context, userID, hash string) error
	FindUserByEmail(ctx context.Context, email string) (*User, error)

	CreateAPIKey(ctx context.Context, k *APIKey) error
	ListAPIKeys(ctx context.Context, userID string) ([]APIKey, error)
	RevokeAPIKey(ctx context.Context, userID, keyID string) error
	DeleteAPIKey(ctx context.Context, userID, keyID string) error
	DeleteAllAPIKeys(ctx context.Context, userID string) error
	FindByKeyHash(ctx context.Context, hash string) (*APIKey, *User, error)
	TouchAPIKey(ctx context.Context, keyID string) error

	GetSubscription(ctx context.Context, userID string) (*Subscription, error)
	SaveSubscription(ctx context.Context, s *Subscription) error

	GetUsage(ctx context.Context, userID string) (*UsageCounter, error)
	AddUsage(ctx context.Context, userID string, bytes int64, files int64) error

	SystemStats(ctx context.Context) (map[string]any, error)

	Close() error
}

type MetaStore interface {
	InsertImage(ctx context.Context, m *ImageMeta) error
	GetImage(ctx context.Context, id string) (*ImageMeta, error)
	ListImages(ctx context.Context, userID string, limit, offset int, f ListFilter) ([]ImageMeta, int64, error)
	DeleteImage(ctx context.Context, userID, id string) (*ImageMeta, error)
	SetVisibility(ctx context.Context, userID, id, visibility string) (*ImageMeta, error)
	IncViewsBy(ctx context.Context, id string, n int64)
	Analytics(ctx context.Context, userID string, since time.Time) (map[string]any, error)
	UpsertStatusDay(ctx context.Context, date string, totalSec, upSec int64) error
	StatusHistory(ctx context.Context, since time.Time) ([]StatusDay, error)
	AddIncident(ctx context.Context, inc *StatusIncident) error
	RecentIncidents(ctx context.Context, limit int) ([]StatusIncident, error)

	SystemStats(ctx context.Context) (map[string]any, error)

	Close() error
}

type PostgresStore struct{ db *sql.DB }

const pgSchema = `
CREATE TABLE IF NOT EXISTS users (
  id           TEXT PRIMARY KEY,
  email        TEXT UNIQUE NOT NULL,
  name         TEXT NOT NULL DEFAULT '',
  avatar_url   TEXT NOT NULL DEFAULT '',
  provider     TEXT NOT NULL,
  provider_id  TEXT NOT NULL,
  is_admin     BOOLEAN NOT NULL DEFAULT FALSE,
  banned       BOOLEAN NOT NULL DEFAULT FALSE,
  password_hash TEXT NOT NULL DEFAULT '',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Email/parol login qo'shilganda eski installatsiyalarga kolumna kerak.
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS users_provider_uidx ON users(provider, provider_id);

CREATE TABLE IF NOT EXISTS api_keys (
  id           TEXT PRIMARY KEY,
  user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT 'default',
  prefix       TEXT NOT NULL,
  hash         TEXT UNIQUE NOT NULL,
  revoked      BOOLEAN NOT NULL DEFAULT FALSE,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS api_keys_user_idx ON api_keys(user_id);

CREATE TABLE IF NOT EXISTS subscriptions (
  user_id              TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  tier                 TEXT NOT NULL DEFAULT 'free',
  expires_at           TIMESTAMPTZ,
  custom_rate_per_min  INTEGER,
  custom_burst         INTEGER,
  custom_max_upload_kb BIGINT,
  custom_max_audio_kb  BIGINT,
  custom_storage_mb    BIGINT,
  custom_monthly_files BIGINT,
  storage_rules        JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS custom_max_audio_kb BIGINT;

CREATE TABLE IF NOT EXISTS usage_counters (
  user_id       TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  storage_bytes BIGINT NOT NULL DEFAULT 0,
  file_count    BIGINT NOT NULL DEFAULT 0,
  month_key     TEXT NOT NULL DEFAULT '',
  month_files   BIGINT NOT NULL DEFAULT 0,
  month_bytes   BIGINT NOT NULL DEFAULT 0
);`

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(c); err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, pgSchema); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

func (p *PostgresStore) Close() error { return p.db.Close() }

func (p *PostgresStore) UpsertOAuthUser(ctx context.Context, u *User) (*User, error) {
	out, err := p.insertOAuthUser(ctx, u)
	if err == nil {
		return out, nil
	}
	if !isUniqueViolation(err) {
		return nil, err
	}
	if ex, e2 := p.findByProvider(ctx, u.Provider, u.ProviderID); e2 == nil {
		return p.patchOAuthProfile(ctx, ex.ID, u)
	}
	if u.Email != "" {
		if ex, e2 := p.FindUserByEmail(ctx, u.Email); e2 == nil {
			return p.patchOAuthProfile(ctx, ex.ID, u)
		}
	}
	return nil, err
}

func (p *PostgresStore) insertOAuthUser(ctx context.Context, u *User) (*User, error) {
	const q = `
INSERT INTO users (id,email,name,avatar_url,provider,provider_id,is_admin,password_hash,created_at,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now(),now())
ON CONFLICT (provider, provider_id) DO UPDATE
  SET email=EXCLUDED.email, name=EXCLUDED.name, avatar_url=EXCLUDED.avatar_url,
      is_admin=users.is_admin OR EXCLUDED.is_admin,
      password_hash=CASE WHEN EXCLUDED.password_hash != '' THEN EXCLUDED.password_hash ELSE users.password_hash END,
      updated_at=now()
RETURNING id,email,name,avatar_url,provider,provider_id,is_admin,banned,password_hash,created_at,updated_at;`
	out := &User{}
	err := p.db.QueryRowContext(ctx, q, u.ID, u.Email, u.Name, u.AvatarURL, u.Provider, u.ProviderID, u.IsAdmin, u.PasswordHash).
		Scan(&out.ID, &out.Email, &out.Name, &out.AvatarURL, &out.Provider, &out.ProviderID, &out.IsAdmin, &out.Banned, &out.PasswordHash, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_, _ = p.db.ExecContext(ctx, `INSERT INTO subscriptions(user_id,tier) VALUES($1,'free') ON CONFLICT DO NOTHING`, out.ID)
	_, _ = p.db.ExecContext(ctx, `INSERT INTO usage_counters(user_id) VALUES($1) ON CONFLICT DO NOTHING`, out.ID)
	return out, nil
}

func (p *PostgresStore) findByProvider(ctx context.Context, provider, providerID string) (*User, error) {
	u := &User{}
	err := p.db.QueryRowContext(ctx,
		`SELECT id,email,name,avatar_url,provider,provider_id,is_admin,banned,password_hash,created_at,updated_at FROM users WHERE provider=$1 AND provider_id=$2`,
		provider, providerID).
		Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.Provider, &u.ProviderID, &u.IsAdmin, &u.Banned, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return u, err
}

func (p *PostgresStore) patchOAuthProfile(ctx context.Context, id string, u *User) (*User, error) {
	const q = `
UPDATE users SET
  email = CASE WHEN $2 <> '' THEN $2 ELSE email END,
  name = CASE WHEN $3 <> '' THEN $3 ELSE name END,
  avatar_url = CASE WHEN $4 <> '' THEN $4 ELSE avatar_url END,
  is_admin = is_admin OR $5,
  provider = CASE WHEN $6 <> '' THEN $6 ELSE provider END,
  provider_id = CASE WHEN $7 <> '' THEN $7 ELSE provider_id END,
  password_hash = CASE WHEN $8 <> '' THEN $8 ELSE password_hash END,
  updated_at = now()
WHERE id=$1
RETURNING id,email,name,avatar_url,provider,provider_id,is_admin,banned,password_hash,created_at,updated_at;`
	out := &User{}
	err := p.db.QueryRowContext(ctx, q, id, u.Email, u.Name, u.AvatarURL, u.IsAdmin, u.Provider, u.ProviderID, u.PasswordHash).
		Scan(&out.ID, &out.Email, &out.Name, &out.AvatarURL, &out.Provider, &out.ProviderID, &out.IsAdmin, &out.Banned, &out.PasswordHash, &out.CreatedAt, &out.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_, _ = p.db.ExecContext(ctx, `INSERT INTO subscriptions(user_id,tier) VALUES($1,'free') ON CONFLICT DO NOTHING`, out.ID)
	_, _ = p.db.ExecContext(ctx, `INSERT INTO usage_counters(user_id) VALUES($1) ON CONFLICT DO NOTHING`, out.ID)
	return out, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pe *pq.Error
	if errors.As(err, &pe) {
		return pe.Code == "23505"
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "duplicate key") || strings.Contains(s, "unique constraint")
}

func (p *PostgresStore) GetUser(ctx context.Context, id string) (*User, error) {
	u := &User{}
	err := p.db.QueryRowContext(ctx, `SELECT id,email,name,avatar_url,provider,provider_id,is_admin,banned,password_hash,created_at,updated_at FROM users WHERE id=$1`, id).
		Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.Provider, &u.ProviderID, &u.IsAdmin, &u.Banned, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return u, err
}

func (p *PostgresStore) ListUsers(ctx context.Context, limit, offset int) ([]User, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id,email,name,avatar_url,provider,provider_id,is_admin,banned,created_at,updated_at FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.Provider, &u.ProviderID, &u.IsAdmin, &u.Banned, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (p *PostgresStore) UpdateProfile(ctx context.Context, userID, name, avatarURL string) (*User, error) {
	const q = `UPDATE users SET name=$2, avatar_url=$3, updated_at=now() WHERE id=$1
RETURNING id,email,name,avatar_url,provider,provider_id,is_admin,banned,created_at,updated_at`
	u := &User{}
	err := p.db.QueryRowContext(ctx, q, userID, name, avatarURL).
		Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.Provider, &u.ProviderID, &u.IsAdmin, &u.Banned, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (p *PostgresStore) SetPasswordHash(ctx context.Context, userID, hash string) error {
	_, err := p.db.ExecContext(ctx, `UPDATE users SET password_hash=$2, updated_at=now() WHERE id=$1`, userID, hash)
	return err
}

func (p *PostgresStore) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	u := &User{}
	err := p.db.QueryRowContext(ctx, `SELECT id,email,name,avatar_url,provider,provider_id,is_admin,banned,password_hash,created_at,updated_at FROM users WHERE email=$1`, email).
		Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.Provider, &u.ProviderID, &u.IsAdmin, &u.Banned, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (p *PostgresStore) SetAdmin(ctx context.Context, id string, admin bool) error {
	_, err := p.db.ExecContext(ctx, `UPDATE users SET is_admin=$2, updated_at=now() WHERE id=$1`, id, admin)
	return err
}

func (p *PostgresStore) SetBanned(ctx context.Context, id string, banned bool) error {
	_, err := p.db.ExecContext(ctx, `UPDATE users SET banned=$2, updated_at=now() WHERE id=$1`, id, banned)
	return err
}

func (p *PostgresStore) CreateAPIKey(ctx context.Context, k *APIKey) error {
	_, err := p.db.ExecContext(ctx, `INSERT INTO api_keys(id,user_id,name,prefix,hash) VALUES($1,$2,$3,$4,$5)`,
		k.ID, k.UserID, k.Name, k.Prefix, k.Hash)
	return err
}

func (p *PostgresStore) ListAPIKeys(ctx context.Context, userID string) ([]APIKey, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id,user_id,name,prefix,revoked,created_at,last_used_at FROM api_keys WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		var k APIKey
		var lu sql.NullTime
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.Prefix, &k.Revoked, &k.CreatedAt, &lu); err != nil {
			return nil, err
		}
		if lu.Valid {
			t := lu.Time
			k.LastUsedAt = &t
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (p *PostgresStore) RevokeAPIKey(ctx context.Context, userID, keyID string) error {
	_, err := p.db.ExecContext(ctx, `UPDATE api_keys SET revoked=TRUE WHERE id=$1 AND user_id=$2`, keyID, userID)
	return err
}

func (p *PostgresStore) DeleteAPIKey(ctx context.Context, userID, keyID string) error {
	res, err := p.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id=$1 AND user_id=$2`, keyID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *PostgresStore) DeleteAllAPIKeys(ctx context.Context, userID string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM api_keys WHERE user_id=$1`, userID)
	return err
}

func (p *PostgresStore) FindByKeyHash(ctx context.Context, hash string) (*APIKey, *User, error) {
	const q = `SELECT k.id,k.user_id,k.name,k.prefix,k.revoked,k.created_at,
	                  u.id,u.email,u.name,u.avatar_url,u.provider,u.provider_id,u.is_admin,u.banned,u.created_at,u.updated_at
	           FROM api_keys k JOIN users u ON u.id=k.user_id WHERE k.hash=$1`
	k := &APIKey{}
	u := &User{}
	err := p.db.QueryRowContext(ctx, q, hash).Scan(&k.ID, &k.UserID, &k.Name, &k.Prefix, &k.Revoked, &k.CreatedAt,
		&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.Provider, &u.ProviderID, &u.IsAdmin, &u.Banned, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil, ErrNotFound
	}
	return k, u, err
}

func (p *PostgresStore) TouchAPIKey(ctx context.Context, keyID string) error {
	_, err := p.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at=now() WHERE id=$1`, keyID)
	return err
}

func (p *PostgresStore) GetSubscription(ctx context.Context, userID string) (*Subscription, error) {
	const q = `SELECT user_id,tier,expires_at,custom_rate_per_min,custom_burst,custom_max_upload_kb,
	           custom_max_audio_kb,custom_storage_mb,custom_monthly_files,storage_rules,updated_at FROM subscriptions WHERE user_id=$1`
	s := &Subscription{}
	var exp sql.NullTime
	var rpm, burst sql.NullInt64
	var upkb, audiokb, stmb, mfiles sql.NullInt64
	var rules []byte
	err := p.db.QueryRowContext(ctx, q, userID).Scan(&s.UserID, &s.Tier, &exp, &rpm, &burst, &upkb, &audiokb, &stmb, &mfiles, &rules, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return &Subscription{UserID: userID, Tier: TierFree}, nil
	}
	if err != nil {
		return nil, err
	}
	if exp.Valid {
		t := exp.Time
		s.ExpiresAt = &t
	}
	if rpm.Valid {
		v := int(rpm.Int64)
		s.CustomRatePerMin = &v
	}
	if burst.Valid {
		v := int(burst.Int64)
		s.CustomBurst = &v
	}
	if upkb.Valid {
		v := upkb.Int64
		s.CustomMaxUploadKB = &v
	}
	if audiokb.Valid {
		v := audiokb.Int64
		s.CustomMaxAudioKB = &v
	}
	if stmb.Valid {
		v := stmb.Int64
		s.CustomStorageMB = &v
	}
	if mfiles.Valid {
		v := mfiles.Int64
		s.CustomMonthlyFiles = &v
	}
	if len(rules) > 0 {
		_ = json.Unmarshal(rules, &s.StorageRules)
	}
	return s, nil
}

func (p *PostgresStore) SaveSubscription(ctx context.Context, s *Subscription) error {
	rules, _ := json.Marshal(orEmptyMap(s.StorageRules))
	const q = `
INSERT INTO subscriptions(user_id,tier,expires_at,custom_rate_per_min,custom_burst,custom_max_upload_kb,
                          custom_max_audio_kb,custom_storage_mb,custom_monthly_files,storage_rules,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now())
ON CONFLICT (user_id) DO UPDATE SET tier=EXCLUDED.tier, expires_at=EXCLUDED.expires_at,
  custom_rate_per_min=EXCLUDED.custom_rate_per_min, custom_burst=EXCLUDED.custom_burst,
  custom_max_upload_kb=EXCLUDED.custom_max_upload_kb, custom_max_audio_kb=EXCLUDED.custom_max_audio_kb,
  custom_storage_mb=EXCLUDED.custom_storage_mb, custom_monthly_files=EXCLUDED.custom_monthly_files,
  storage_rules=EXCLUDED.storage_rules, updated_at=now()`
	_, err := p.db.ExecContext(ctx, q, s.UserID, string(s.Tier), s.ExpiresAt, nullInt(s.CustomRatePerMin),
		nullInt(s.CustomBurst), nullInt64(s.CustomMaxUploadKB), nullInt64(s.CustomMaxAudioKB),
		nullInt64(s.CustomStorageMB), nullInt64(s.CustomMonthlyFiles), rules)
	return err
}

func (p *PostgresStore) GetUsage(ctx context.Context, userID string) (*UsageCounter, error) {
	u := &UsageCounter{UserID: userID}
	err := p.db.QueryRowContext(ctx, `SELECT storage_bytes,file_count,month_key,month_files,month_bytes FROM usage_counters WHERE user_id=$1`, userID).
		Scan(&u.StorageBytes, &u.FileCount, &u.MonthKey, &u.MonthFiles, &u.MonthBytes)
	if err == sql.ErrNoRows {
		return u, nil
	}
	if u.MonthKey != monthKey() {
		u.MonthKey, u.MonthFiles, u.MonthBytes = monthKey(), 0, 0
	}
	return u, err
}

func (p *PostgresStore) AddUsage(ctx context.Context, userID string, bytes, files int64) error {
	const q = `
INSERT INTO usage_counters(user_id,storage_bytes,file_count,month_key,month_files,month_bytes)
VALUES($1,$2,$3,$4,$3,$2)
ON CONFLICT (user_id) DO UPDATE SET
  storage_bytes = GREATEST(usage_counters.storage_bytes + $2, 0),
  file_count    = GREATEST(usage_counters.file_count + $3, 0),
  month_key     = $4,
  month_files   = CASE WHEN usage_counters.month_key = $4 THEN GREATEST(usage_counters.month_files + $3,0) ELSE GREATEST($3,0) END,
  month_bytes   = CASE WHEN usage_counters.month_key = $4 THEN GREATEST(usage_counters.month_bytes + $2,0) ELSE GREATEST($2,0) END`
	_, err := p.db.ExecContext(ctx, q, userID, bytes, files, monthKey())
	return err
}

func (p *PostgresStore) SystemStats(ctx context.Context) (map[string]any, error) {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out := map[string]any{"engine": "postgresql", "connected": true}
	var dbName string
	var size int64
	if err := p.db.QueryRowContext(c, `SELECT current_database(), pg_database_size(current_database())`).
		Scan(&dbName, &size); err != nil {
		return nil, err
	}
	out["database"] = dbName
	out["size_bytes"] = size
	var ver string
	if p.db.QueryRowContext(c, `SHOW server_version`).Scan(&ver) == nil {
		out["server_version"] = ver
	}
	tables := []map[string]any{}
	for _, t := range []string{"users", "api_keys", "subscriptions", "usage_counters"} {
		var rows, bytes int64
		_ = p.db.QueryRowContext(c, `SELECT count(*) FROM "`+t+`"`).Scan(&rows)
		_ = p.db.QueryRowContext(c, `SELECT COALESCE(pg_total_relation_size('public.'+quote_ident($1)),0)::bigint`, t).Scan(&bytes)
		tables = append(tables, map[string]any{"name": t, "rows": rows, "bytes": bytes})
	}
	out["tables"] = tables
	if quota := envInt64("PG_QUOTA_BYTES", 0); quota > 0 {
		out["quota_bytes"] = quota
		out["free_bytes"] = max64(0, quota-size)
		out["used_percent"] = round2(float64(size) / float64(quota) * 100)
	}
	st := p.db.Stats()
	out["pool"] = map[string]any{
		"open": st.OpenConnections, "in_use": st.InUse, "idle": st.Idle,
		"wait_count": st.WaitCount, "max_open": st.MaxOpenConnections,
	}
	return out, nil
}

type MongoStore struct {
	cli       *mongo.Client
	db        *mongo.Database
	images    *mongo.Collection
	status    *mongo.Collection
	incidents *mongo.Collection
}

func NewMongoStore(ctx context.Context, uri, dbName string) (*MongoStore, error) {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cli, err := mongo.Connect(c, options.Client().ApplyURI(uri).SetMaxPoolSize(20))
	if err != nil {
		return nil, err
	}
	if err := cli.Ping(c, nil); err != nil {
		return nil, err
	}
	db := cli.Database(dbName)
	ms := &MongoStore{
		cli: cli, db: db, images: db.Collection("images"),
		status: db.Collection("status_daily"), incidents: db.Collection("status_incidents"),
	}
	_, _ = ms.incidents.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "started_at", Value: -1}},
	})
	_, _ = ms.images.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "checksum", Value: 1}}},
	})
	_ = db.Collection("access_logs").Drop(ctx)
	return ms, nil
}

func (m *MongoStore) Close() error { return m.cli.Disconnect(context.Background()) }

func (m *MongoStore) InsertImage(ctx context.Context, im *ImageMeta) error {
	_, err := m.images.InsertOne(ctx, im)
	return err
}

func (m *MongoStore) GetImage(ctx context.Context, id string) (*ImageMeta, error) {
	var im ImageMeta
	err := m.images.FindOne(ctx, bson.M{"_id": id}).Decode(&im)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &im, err
}

func listFilterMongo(f ListFilter) bson.M {
	flt := bson.M{}
	if f.Visibility != "" {
		flt["visibility"] = f.Visibility
	}
	switch f.Kind {
	case "image":
		flt["kind"] = bson.M{"$nin": []string{"audio", "video"}}
	case "audio":
		flt["kind"] = "audio"
	case "svg":
		flt["mime_type"] = MimeSVG
	case "gif":
		flt["mime_type"] = MimeGIF
	}
	if f.Q != "" {
		flt["filename"] = bson.M{"$regex": primitive.Regex{Pattern: regexp.QuoteMeta(f.Q), Options: "i"}}
	}
	return flt
}

func listSortMongo(sortKey string) bson.D {
	switch sortKey {
	case "oldest":
		return bson.D{{Key: "created_at", Value: 1}}
	case "largest":
		return bson.D{{Key: "size_bytes", Value: -1}}
	case "smallest":
		return bson.D{{Key: "size_bytes", Value: 1}}
	case "views":
		return bson.D{{Key: "views", Value: -1}}
	case "name":
		return bson.D{{Key: "filename", Value: 1}}
	default:
		return bson.D{{Key: "created_at", Value: -1}}
	}
}

func (m *MongoStore) ListImages(ctx context.Context, userID string, limit, offset int, f ListFilter) ([]ImageMeta, int64, error) {
	filter := listFilterMongo(f)
	filter["user_id"] = userID
	total, _ := m.images.CountDocuments(ctx, filter)
	cur, err := m.images.Find(ctx, filter, options.Find().
		SetSort(listSortMongo(f.Sort)).SetSkip(int64(offset)).SetLimit(int64(limit)))
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)
	var out []ImageMeta
	if err := cur.All(ctx, &out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (m *MongoStore) DeleteImage(ctx context.Context, userID, id string) (*ImageMeta, error) {
	var im ImageMeta
	err := m.images.FindOneAndDelete(ctx, bson.M{"_id": id, "user_id": userID}).Decode(&im)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &im, err
}

func (m *MongoStore) SetVisibility(ctx context.Context, userID, id, visibility string) (*ImageMeta, error) {
	var im ImageMeta
	err := m.images.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "user_id": userID},
		bson.M{"$set": bson.M{"visibility": visibility}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&im)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &im, err
}

func (m *MongoStore) IncViewsBy(ctx context.Context, id string, n int64) {
	if n == 0 {
		return
	}
	_, _ = m.images.UpdateByID(ctx, id, bson.M{"$inc": bson.M{"views": n}})
}

func (m *MongoStore) Analytics(ctx context.Context, userID string, since time.Time) (map[string]any, error) {
	match := bson.M{"created_at": bson.M{"$gte": since}}
	if userID != "" {
		match["user_id"] = userID
	}
	cur, err := m.images.Aggregate(ctx, []bson.M{
		{"$match": match},
		{"$group": bson.M{
			"_id":    bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$created_at"}},
			"upload": bson.M{"$sum": 1},
			"bytes":  bson.M{"$sum": "$size_bytes"},
			"view":   bson.M{"$sum": "$views"},
		}},
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	type row struct {
		ID     string `bson:"_id"`
		Upload int64  `bson:"upload"`
		Bytes  int64  `bson:"bytes"`
		View   int64  `bson:"view"`
	}
	var rows []row
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	buckets := make(map[string]map[string]int64, len(rows))
	for _, r := range rows {
		buckets[r.ID] = map[string]int64{"upload": r.Upload, "bytes": r.Bytes, "view": r.View}
	}
	return map[string]any{"timeline": buildTimeline(since, buckets)}, nil
}

func (m *MongoStore) SystemStats(ctx context.Context) (map[string]any, error) {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out := map[string]any{"engine": "mongodb", "connected": true, "database": m.db.Name()}
	var st bson.M
	if err := m.db.RunCommand(c, bson.D{{Key: "dbStats", Value: 1}, {Key: "scale", Value: 1}}).Decode(&st); err == nil {
		num := func(k string) int64 {
			if v, ok := st[k].(float64); ok {
				return int64(v)
			}
			if v, ok := st[k].(int32); ok {
				return int64(v)
			}
			if v, ok := st[k].(int64); ok {
				return v
			}
			return 0
		}
		out["data_bytes"] = num("dataSize")
		out["storage_bytes"] = num("storageSize")
		out["index_bytes"] = num("indexSize")
		out["collections"] = num("collections")
		out["objects"] = num("objects")
		if total := num("storageSize") + num("indexSize"); total > 0 {
			out["size_bytes"] = total
		}
	}
	if n, err := m.images.CountDocuments(c, bson.M{}); err == nil {
		out["images"] = n
	}
	var admin bson.M
	if err := m.cli.Database("admin").RunCommand(c, bson.D{{Key: "listDatabases", Value: 1}}).Decode(&admin); err == nil {
		if list, ok := admin["databases"].([]interface{}); ok {
			rows := make([]map[string]any, 0, len(list))
			var sum int64
			for _, it := range list {
				db, ok := it.(bson.M)
				if !ok {
					if db2, ok2 := it.(map[string]interface{}); ok2 {
						db = bson.M(db2)
						ok = true
					}
				}
				if !ok {
					continue
				}
				name, _ := db["name"].(string)
				size, _ := db["sizeOnDisk"].(int64)
				if f, okf := db["sizeOnDisk"].(float64); okf {
					size = int64(f)
				}
				sum += size
				rows = append(rows, map[string]any{"name": name, "size_bytes": size})
			}
			if len(rows) > 0 {
				out["databases"] = rows
				out["total_bytes"] = sum
			}
		}
	}
	return out, nil
}

func (m *MongoStore) UpsertStatusDay(ctx context.Context, date string, totalSec, upSec int64) error {
	_, err := m.status.UpdateByID(ctx, date, bson.M{
		"$max": bson.M{"total": totalSec, "up": upSec},
	}, options.Update().SetUpsert(true))
	return err
}

func (m *MongoStore) StatusHistory(ctx context.Context, since time.Time) ([]StatusDay, error) {
	cur, err := m.status.Find(ctx, bson.M{"_id": bson.M{"$gte": since.UTC().Format("2006-01-02")}},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := make([]StatusDay, 0)
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Total > 0 {
			out[i].UptimePct = math.Round(float64(out[i].Up)/float64(out[i].Total)*1000) / 10
		}
	}
	return out, nil
}

func (m *MongoStore) AddIncident(ctx context.Context, inc *StatusIncident) error {
	_, err := m.incidents.InsertOne(ctx, inc)
	return err
}

func (m *MongoStore) RecentIncidents(ctx context.Context, limit int) ([]StatusIncident, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	cur, err := m.incidents.Find(ctx, bson.M{}, options.Find().
		SetSort(bson.D{{Key: "started_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := make([]StatusIncident, 0)
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func buildTimeline(since time.Time, buckets map[string]map[string]int64) []map[string]any {
	out := make([]map[string]any, 0, 91)
	day := time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	now := time.Now().UTC()
	for i := 0; i < 92 && !day.After(now); i++ {
		key := day.Format("2006-01-02")
		b := buckets[key]
		if b == nil {
			b = map[string]int64{}
		}
		out = append(out, map[string]any{
			"date":   key,
			"view":   b["view"] + b["stream"],
			"upload": b["upload"],
			"delete": b["delete"],
			"bytes":  b["bytes"],
		})
		day = day.AddDate(0, 0, 1)
	}
	return out
}

type MemStore struct {
	mu        sync.RWMutex
	users     map[string]*User
	keys      map[string]*APIKey
	subs      map[string]*Subscription
	usage     map[string]*UsageCounter
	images    map[string]*ImageMeta
	statusDay map[string]StatusDay
	incidents []StatusIncident
}

func NewMemStore() *MemStore {
	return &MemStore{
		users: map[string]*User{}, keys: map[string]*APIKey{},
		subs: map[string]*Subscription{}, usage: map[string]*UsageCounter{},
		images: map[string]*ImageMeta{}, statusDay: map[string]StatusDay{},
	}
}

func (s *MemStore) Close() error { return nil }

func (s *MemStore) UpsertOAuthUser(_ context.Context, u *User) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	apply := func(ex *User) *User {
		if u.Email != "" {
			ex.Email = u.Email
		}
		if u.Name != "" {
			ex.Name = u.Name
		}
		if u.AvatarURL != "" {
			ex.AvatarURL = u.AvatarURL
		}
		ex.IsAdmin = ex.IsAdmin || u.IsAdmin
		if u.Provider != "" {
			ex.Provider, ex.ProviderID = u.Provider, u.ProviderID
		}
		if u.PasswordHash != "" {
			ex.PasswordHash = u.PasswordHash
		}
		ex.UpdatedAt = time.Now()
		cp := *ex
		return &cp
	}
	for _, ex := range s.users {
		if ex.Provider == u.Provider && ex.ProviderID == u.ProviderID {
			return apply(ex), nil
		}
	}
	if u.Email != "" {
		for _, ex := range s.users {
			if strings.EqualFold(ex.Email, u.Email) {
				return apply(ex), nil
			}
		}
	}
	u.CreatedAt, u.UpdatedAt = time.Now(), time.Now()
	cp := *u
	s.users[u.ID] = &cp
	s.subs[u.ID] = &Subscription{UserID: u.ID, Tier: TierFree, UpdatedAt: time.Now()}
	s.usage[u.ID] = &UsageCounter{UserID: u.ID, MonthKey: monthKey()}
	out := cp
	return &out, nil
}

func (s *MemStore) GetUser(_ context.Context, id string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if u, ok := s.users[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, ErrNotFound
}

func (s *MemStore) ListUsers(_ context.Context, limit, offset int) ([]User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []User
	for _, u := range s.users {
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return page(out, limit, offset), nil
}

func (s *MemStore) SetAdmin(_ context.Context, id string, admin bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.users[id]; ok {
		u.IsAdmin = admin
		return nil
	}
	return ErrNotFound
}

func (s *MemStore) SetBanned(_ context.Context, id string, banned bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.users[id]; ok {
		u.Banned = banned
		return nil
	}
	return ErrNotFound
}

func (s *MemStore) UpdateProfile(_ context.Context, userID, name, avatarURL string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.users[userID]; ok {
		u.Name, u.AvatarURL, u.UpdatedAt = name, avatarURL, time.Now()
		cp := *u
		return &cp, nil
	}
	return nil, ErrNotFound
}

func (s *MemStore) SetPasswordHash(_ context.Context, userID, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.users[userID]; ok {
		u.PasswordHash = hash
		return nil
	}
	return ErrNotFound
}

func (s *MemStore) FindUserByEmail(_ context.Context, email string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.users {
		if strings.EqualFold(u.Email, email) {
			cp := *u
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (s *MemStore) DeleteAPIKey(_ context.Context, userID, keyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for h, k := range s.keys {
		if k.ID == keyID && k.UserID == userID {
			delete(s.keys, h)
			return nil
		}
	}
	return ErrNotFound
}

func (s *MemStore) DeleteAllAPIKeys(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for h, k := range s.keys {
		if k.UserID == userID {
			delete(s.keys, h)
		}
	}
	return nil
}

func (s *MemStore) CreateAPIKey(_ context.Context, k *APIKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *k
	s.keys[k.Hash] = &cp
	return nil
}

func (s *MemStore) ListAPIKeys(_ context.Context, userID string) ([]APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []APIKey
	for _, k := range s.keys {
		if k.UserID == userID {
			out = append(out, *k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *MemStore) RevokeAPIKey(_ context.Context, userID, keyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range s.keys {
		if k.ID == keyID && k.UserID == userID {
			k.Revoked = true
			return nil
		}
	}
	return ErrNotFound
}

func (s *MemStore) FindByKeyHash(_ context.Context, hash string) (*APIKey, *User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, ok := s.keys[hash]
	if !ok {
		return nil, nil, ErrNotFound
	}
	u, ok := s.users[k.UserID]
	if !ok {
		return nil, nil, ErrNotFound
	}
	kc, uc := *k, *u
	return &kc, &uc, nil
}

func (s *MemStore) TouchAPIKey(_ context.Context, keyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, k := range s.keys {
		if k.ID == keyID {
			k.LastUsedAt = &now
		}
	}
	return nil
}

func (s *MemStore) GetSubscription(_ context.Context, userID string) (*Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sub, ok := s.subs[userID]; ok {
		cp := *sub
		return &cp, nil
	}
	return &Subscription{UserID: userID, Tier: TierFree}, nil
}

func (s *MemStore) SaveSubscription(_ context.Context, sub *Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *sub
	cp.UpdatedAt = time.Now()
	s.subs[sub.UserID] = &cp
	return nil
}

func (s *MemStore) GetUsage(_ context.Context, userID string) (*UsageCounter, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if u, ok := s.usage[userID]; ok {
		cp := *u
		if cp.MonthKey != monthKey() {
			cp.MonthKey, cp.MonthFiles, cp.MonthBytes = monthKey(), 0, 0
		}
		return &cp, nil
	}
	return &UsageCounter{UserID: userID, MonthKey: monthKey()}, nil
}

func (s *MemStore) AddUsage(_ context.Context, userID string, bytes, files int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.usage[userID]
	if !ok {
		u = &UsageCounter{UserID: userID, MonthKey: monthKey()}
		s.usage[userID] = u
	}
	if u.MonthKey != monthKey() {
		u.MonthKey, u.MonthFiles, u.MonthBytes = monthKey(), 0, 0
	}
	u.StorageBytes = max64(0, u.StorageBytes+bytes)
	u.FileCount = max64(0, u.FileCount+files)
	u.MonthBytes = max64(0, u.MonthBytes+bytes)
	u.MonthFiles = max64(0, u.MonthFiles+files)
	return nil
}

func (s *MemStore) InsertImage(_ context.Context, m *ImageMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *m
	s.images[m.ID] = &cp
	return nil
}

func (s *MemStore) GetImage(_ context.Context, id string) (*ImageMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if m, ok := s.images[id]; ok {
		cp := *m
		return &cp, nil
	}
	return nil, ErrNotFound
}

func (s *MemStore) ListImages(_ context.Context, userID string, limit, offset int, f ListFilter) ([]ImageMeta, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ImageMeta
	for _, m := range s.images {
		if m.UserID == userID && f.Match(m) {
			out = append(out, *m)
		}
	}
	sortImageMeta(out, f.Sort)
	return page(out, limit, offset), int64(len(out)), nil
}

func (s *MemStore) DeleteImage(_ context.Context, userID, id string) (*ImageMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.images[id]
	if !ok || m.UserID != userID {
		return nil, ErrNotFound
	}
	delete(s.images, id)
	return m, nil
}

func (s *MemStore) SetVisibility(_ context.Context, userID, id, visibility string) (*ImageMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.images[id]
	if !ok || m.UserID != userID {
		return nil, ErrNotFound
	}
	m.Visibility = visibility
	cp := *m
	return &cp, nil
}

func (s *MemStore) IncViewsBy(_ context.Context, id string, n int64) {
	if n == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.images[id]; ok {
		m.Views += n
	}
}

func (s *MemStore) Analytics(_ context.Context, userID string, since time.Time) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	buckets := map[string]map[string]int64{}
	for _, m := range s.images {
		if userID != "" && m.UserID != userID {
			continue
		}
		if m.CreatedAt.Before(since) {
			continue
		}
		k := m.CreatedAt.UTC().Format("2006-01-02")
		b := buckets[k]
		if b == nil {
			b = map[string]int64{}
			buckets[k] = b
		}
		b["upload"]++
		b["bytes"] += m.SizeBytes
		b["view"] += m.Views
	}
	return map[string]any{"timeline": buildTimeline(since, buckets)}, nil
}

func (s *MemStore) SystemStats(_ context.Context) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var size int64
	for _, m := range s.images {
		size += m.SizeBytes
	}
	return map[string]any{
		"engine":     "memory",
		"connected":  true,
		"database":   "in-memory",
		"users":      int64(len(s.users)),
		"images":     int64(len(s.images)),
		"api_keys":   int64(len(s.keys)),
		"size_bytes": size,
		"data_bytes": size,
		"note":       "demo rejim — ma'lumotlar faqat RAM'da",
	}, nil
}

func (s *MemStore) UpsertStatusDay(_ context.Context, date string, totalSec, upSec int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.statusDay[date]
	d.Date = date
	d.Total += totalSec
	d.Up += upSec
	s.statusDay[date] = d
	return nil
}

func (s *MemStore) StatusHistory(_ context.Context, since time.Time) ([]StatusDay, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cut := since.UTC().Format("2006-01-02")
	out := make([]StatusDay, 0)
	for date, d := range s.statusDay {
		if date >= cut {
			d.UptimePct = 100
			if d.Total > 0 {
				d.UptimePct = math.Round(float64(d.Up)/float64(d.Total)*1000) / 10
			}
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}

func (s *MemStore) AddIncident(_ context.Context, inc *StatusIncident) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *inc
	s.incidents = append([]StatusIncident{cp}, s.incidents...)
	if len(s.incidents) > 100 {
		s.incidents = s.incidents[:100]
	}
	return nil
}

func (s *MemStore) RecentIncidents(_ context.Context, limit int) ([]StatusIncident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.incidents) {
		limit = len(s.incidents)
	}
	if limit == 0 {
		return []StatusIncident{}, nil
	}
	out := make([]StatusIncident, limit)
	copy(out, s.incidents[:limit])
	return out, nil
}

func page[T any](in []T, limit, offset int) []T {
	if offset >= len(in) {
		return nil
	}
	end := offset + limit
	if end > len(in) {
		end = len(in)
	}
	return in[offset:end]
}

func monthKey() string { return time.Now().UTC().Format("2006-01") }

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func nullInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func orEmptyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func isDSNPostgres(dsn string) bool {
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") || strings.Contains(dsn, "host=")
}
