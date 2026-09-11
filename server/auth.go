package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"golang.org/x/crypto/bcrypt"
)

type ctxKey string

const (
	ctxUserKey   ctxKey = "user"
	ctxPolicyKey ctxKey = "policy"
)

type Auth struct {
	cfg       *Config
	core      CoreStore
	http      *http.Client
	keyCache  *lru.LRU[string, *authCacheEntry]
	userCache *lru.LRU[string, *User]
	subCache  *lru.LRU[string, *Subscription]
	states    *lru.LRU[string, string]
	authRate  *lru.LRU[string, int]
}

type authCacheEntry struct {
	KeyID string
	User  *User
	Sub   *Subscription
}

func NewAuth(cfg *Config, core CoreStore) *Auth {
	return &Auth{
		cfg:       cfg,
		core:      core,
		http:      &http.Client{Timeout: 15 * time.Second},
		keyCache:  lru.NewLRU[string, *authCacheEntry](20000, nil, 60*time.Second),
		userCache: lru.NewLRU[string, *User](20000, nil, 60*time.Second),
		subCache:  lru.NewLRU[string, *Subscription](20000, nil, 60*time.Second),
		states:    lru.NewLRU[string, string](10000, nil, 10*time.Minute),
		authRate:  lru.NewLRU[string, int](10000, nil, 10*time.Minute),
	}
}

func (a *Auth) InvalidateKeyCache() { a.keyCache.Purge() }

func (a *Auth) PurgeUser(id string) {
	a.userCache.Remove(id)
}

func (a *Auth) PurgeSub(id string) {
	a.subCache.Remove(id)
	a.keyCache.Purge()
}

func (a *Auth) isAdminEmail(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	for _, e := range a.cfg.AdminEmails {
		if strings.ToLower(strings.TrimSpace(e)) == email {
			return true
		}
	}
	return false
}

func (a *Auth) NewAPIKey(ctx context.Context, userID, name string) (plain string, rec *APIKey, err error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	plain = "vb_live_" + base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plain))
	if name == "" {
		name = "default"
	}
	rec = &APIKey{
		ID:        newID("key"),
		UserID:    userID,
		Name:      name,
		Prefix:    plain[:16] + "…",
		Hash:      hex.EncodeToString(sum[:]),
		CreatedAt: time.Now(),
	}
	if err := a.core.CreateAPIKey(ctx, rec); err != nil {
		return "", nil, err
	}
	return plain, rec, nil
}

func (a *Auth) IssueJWT(u *User) (string, error) {
	claims := jwt.MapClaims{
		"sub":   u.ID,
		"email": u.Email,
		"admin": u.IsAdmin,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(a.cfg.JWTTTL).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.cfg.JWTSecret)
}

func (a *Auth) parseJWT(tok string) (string, error) {
	t, err := jwt.Parse(tok, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("bad signing method")
		}
		return a.cfg.JWTSecret, nil
	})
	if err != nil || !t.Valid {
		return "", errors.New("invalid token")
	}
	claims, ok := t.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid claims")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", errors.New("invalid subject")
	}
	return sub, nil
}

func (a *Auth) cachedUser(ctx context.Context, id string) (*User, error) {
	if u, ok := a.userCache.Get(id); ok {
		return u, nil
	}
	u, err := a.core.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}
	a.userCache.Add(id, u)
	return u, nil
}

func (a *Auth) cachedSub(ctx context.Context, userID string) *Subscription {
	if s, ok := a.subCache.Get(userID); ok {
		return s
	}
	s, err := a.core.GetSubscription(ctx, userID)
	if err != nil || s == nil {
		s = &Subscription{UserID: userID, Tier: TierFree}
	}
	a.subCache.Add(userID, s)
	return s
}

func extractAPIKey(r *http.Request) string {
	key := strings.TrimSpace(r.Header.Get("X-API-Key"))
	if key == "" {
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer vb_live_") {
			key = strings.TrimPrefix(h, "Bearer ")
		}
	}
	if key == "" {
		key = r.URL.Query().Get("api_key")
	}
	return key
}

type authErr struct {
	status int
	code   string
	msg    string
}

func (a *Auth) resolveAPIKey(r *http.Request, key string) (*authCacheEntry, *authErr) {
	sum := sha256.Sum256([]byte(key))
	hash := hex.EncodeToString(sum[:])

	entry, ok := a.keyCache.Get(hash)
	if !ok {
		rec, user, err := a.core.FindByKeyHash(r.Context(), hash)
		if err != nil || rec.Revoked {
			return nil, &authErr{http.StatusUnauthorized, "invalid_api_key", "API kalit yaroqsiz yoki bekor qilingan"}
		}
		sub := a.cachedSub(r.Context(), user.ID)
		entry = &authCacheEntry{KeyID: rec.ID, User: user, Sub: sub}
		a.keyCache.Add(hash, entry)
		go func(id string) {
			c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = a.core.TouchAPIKey(c, id)
		}(rec.ID)
	}
	if entry.User.Banned {
		return nil, &authErr{http.StatusForbidden, "account_suspended", "Hisob to'xtatilgan"}
	}
	return entry, nil
}

func withEntry(r *http.Request, e *authCacheEntry) *http.Request {
	ctx := context.WithValue(r.Context(), ctxUserKey, e.User)
	ctx = context.WithValue(ctx, ctxPolicyKey, EffectivePolicy(e.Sub))
	return r.WithContext(ctx)
}

func (a *Auth) RequireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := extractAPIKey(r)
		if key == "" {
			writeErr(w, http.StatusUnauthorized, "missing_api_key", "X-API-Key header talab qilinadi")
			return
		}
		entry, e := a.resolveAPIKey(r, key)
		if e != nil {
			writeErr(w, e.status, e.code, e.msg)
			return
		}
		next.ServeHTTP(w, withEntry(r, entry))
	})
}

func (a *Auth) OptionalAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key := extractAPIKey(r); key != "" {
			if entry, e := a.resolveAPIKey(r, key); e == nil {
				r = withEntry(r, entry)
			} else if u := a.SessionUser(r); u != nil {
				sub := a.cachedSub(r.Context(), u.ID)
				r = withEntry(r, &authCacheEntry{User: u, Sub: sub})
			}
		} else if u := a.SessionUser(r); u != nil {
			sub := a.cachedSub(r.Context(), u.ID)
			r = withEntry(r, &authCacheEntry{User: u, Sub: sub})
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Auth) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key := extractAPIKey(r); key != "" {
			entry, e := a.resolveAPIKey(r, key)
			if e == nil {
				next.ServeHTTP(w, withEntry(r, entry))
				return
			}
			if sessionToken(r) == "" {
				writeErr(w, e.status, e.code, e.msg)
				return
			}
		}
		if sessionToken(r) == "" {
			writeErr(w, http.StatusUnauthorized, "unauthenticated",
				"Kiritilgan kalit yo'q: X-API-Key header yoki sessiya kerak")
			return
		}
		a.RequireJWT(next).ServeHTTP(w, r)
	})
}

func (a *Auth) RequireJWT(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := sessionToken(r)
		if tok == "" {
			writeErr(w, http.StatusUnauthorized, "unauthenticated", "Tizimga kiring")
			return
		}
		uid, err := a.parseJWT(tok)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "invalid_session", "Sessiya yaroqsiz")
			return
		}
		u, err := a.cachedUser(r.Context(), uid)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unknown_user", "Foydalanuvchi topilmadi")
			return
		}
		if u.Banned {
			writeErr(w, http.StatusForbidden, "account_suspended", "Hisob to'xtatilgan")
			return
		}
		sub := a.cachedSub(r.Context(), u.ID)
		ctx := context.WithValue(r.Context(), ctxUserKey, u)
		ctx = context.WithValue(ctx, ctxPolicyKey, EffectivePolicy(sub))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *Auth) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFrom(r.Context())
		if u == nil || !u.IsAdmin {
			writeErr(w, http.StatusForbidden, "admin_only", "Faqat administratorlar uchun")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sessionToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") && !strings.HasPrefix(h, "Bearer vb_live_") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
}

const sessionCookie = "vb_session"

func (a *Auth) SessionUser(r *http.Request) *User {
	tok := sessionToken(r)
	if tok == "" {
		return nil
	}
	uid, err := a.parseJWT(tok)
	if err != nil {
		return nil
	}
	u, err := a.cachedUser(r.Context(), uid)
	if err != nil || u.Banned {
		return nil
	}
	return u
}

func (a *Auth) setSessionCookie(w http.ResponseWriter, tok string) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/",
		HttpOnly: true, Secure: a.cfg.IsHTTPS(),
		SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(a.cfg.JWTTTL),
	})
}

func (a *Auth) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: a.cfg.IsHTTPS(), SameSite: http.SameSiteLaxMode,
	})
}

func UserFrom(ctx context.Context) *User {
	u, _ := ctx.Value(ctxUserKey).(*User)
	return u
}

func PolicyFrom(ctx context.Context) TierPolicy {
	p, ok := ctx.Value(ctxPolicyKey).(TierPolicy)
	if !ok {
		return DefaultPolicies[TierFree]
	}
	return p
}

type oauthProvider struct {
	name     string
	authURL  string
	tokenURL string
	userURL  string
	scopes   string
	clientID string
	secret   string
}

func (a *Auth) provider(name string) (*oauthProvider, error) {
	switch name {
	case "github":
		if a.cfg.GithubClientID == "" {
			return nil, errors.New("github oauth sozlanmagan")
		}
		return &oauthProvider{
			name:     "github",
			authURL:  "https://github.com/login/oauth/authorize",
			tokenURL: "https://github.com/login/oauth/access_token",
			userURL:  "https://api.github.com/user",
			scopes:   "read:user user:email",
			clientID: a.cfg.GithubClientID, secret: a.cfg.GithubClientSecret,
		}, nil
	}
	return nil, errors.New("noma'lum provayder")
}

func (a *Auth) redirectBase() string {
	if b := strings.TrimRight(a.cfg.WebURL, "/"); b != "" {
		return b
	}
	return ""
}

func (a *Auth) mintOAuthState(next string) string {
	exp := time.Now().Add(10 * time.Minute).Unix()
	payload := base64.RawURLEncoding.EncodeToString([]byte(next)) + "." + strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, a.cfg.JWTSecret)
	mac.Write([]byte(payload))
	return payload + "." + hex.EncodeToString(mac.Sum(nil))
}

func (a *Auth) parseOAuthState(state string) (string, bool) {
	if state == "" {
		return "", false
	}
	if next, ok := a.states.Get(state); ok {
		a.states.Remove(state)
		return next, true
	}
	parts := strings.Split(state, ".")
	if len(parts) != 3 {
		return "", false
	}
	payload := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, a.cfg.JWTSecret)
	mac.Write([]byte(payload))
	sig, err := hex.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return "", false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	next, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	return string(next), true
}

func (a *Auth) LoginHandler(w http.ResponseWriter, r *http.Request, providerName string) {
	p, err := a.provider(providerName)
	if err != nil {
		writeErr(w, http.StatusNotImplemented, "oauth_not_configured",
			"Ushbu serverda "+providerName+" kirishi sozlanmagan — email orqali kiring")
		return
	}
	next := safeNext(r.URL.Query().Get("next"))
	state := a.mintOAuthState(next)
	a.states.Add(state, next)
	q := url.Values{
		"client_id":     {p.clientID},
		"redirect_uri":  {a.cfg.PublicURL + "/auth/" + p.name + "/callback"},
		"response_type": {"code"},
		"scope":         {p.scopes},
		"state":         {state},
	}
	http.Redirect(w, r, p.authURL+"?"+q.Encode(), http.StatusFound)
}

func (a *Auth) CallbackHandler(w http.ResponseWriter, r *http.Request, providerName string) {
	p, err := a.provider(providerName)
	if err != nil {
		writeErr(w, http.StatusNotImplemented, "oauth_not_configured", err.Error())
		return
	}
	state := r.URL.Query().Get("state")
	next, ok := a.parseOAuthState(state)
	if !ok {
		a.loginFailed(w, r, "invalid_state", "Kirish sessiyasi eskirgan. Qaytadan urinib ko'ring")
		return
	}

	if e := r.URL.Query().Get("error"); e != "" {
		a.loginFailed(w, r, "oauth_denied", "Kirish bekor qilindi")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		a.loginFailed(w, r, "missing_code", "Authorization code yo'q")
		return
	}
	token, err := a.exchange(r.Context(), p, code)
	if err != nil {
		a.loginFailed(w, r, "oauth_exchange_failed", "Provayder bilan bog'lanib bo'lmadi")
		return
	}
	prof, err := a.fetchProfile(r.Context(), p, token)
	if err != nil {
		a.loginFailed(w, r, "oauth_profile_failed", "Profil ma'lumotini olib bo'lmadi")
		return
	}
	prof.ID = newID("usr")
	prof.IsAdmin = a.isAdminEmail(prof.Email)

	user, err := a.core.UpsertOAuthUser(r.Context(), prof)
	if err != nil {
		a.loginFailed(w, r, "user_persist_failed", "Foydalanuvchini saqlab bo'lmadi")
		return
	}
	a.userCache.Add(user.ID, user)
	plain := ""
	if keys, _ := a.core.ListAPIKeys(r.Context(), user.ID); len(keys) == 0 {
		plain, _, _ = a.NewAPIKey(r.Context(), user.ID, "default")
	}
	jwtTok, err := a.IssueJWT(user)
	if err != nil {
		a.loginFailed(w, r, "token_failed", "Token yaratib bo'lmadi")
		return
	}
	a.setSessionCookie(w, jwtTok)

	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "message": "Muvaffaqiyatli kirdingiz",
			"user": user, "token": jwtTok, "api_key": plain,
			"note": "api_key faqat shu javobda ko'rsatiladi va qayta ko'rsatilmaydi",
		})
		return
	}
	if next == "" {
		next = a.redirectBase() + "/console"
	}
	frag := url.Values{}
	frag.Set("token", jwtTok)
	if plain != "" {
		frag.Set("api_key", plain)
	}
	next += "#" + frag.Encode()
	http.Redirect(w, r, next, http.StatusFound)
}

func (a *Auth) loginFailed(w http.ResponseWriter, r *http.Request, code, msg string) {
	if wantsJSON(r) {
		writeErr(w, http.StatusBadRequest, code, msg)
		return
	}
	http.Redirect(w, r, a.redirectBase()+"/login?auth_error="+url.QueryEscape(code), http.StatusFound)
}

func (a *Auth) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	if u := a.SessionUser(r); u != nil {
		a.PurgeUser(u.ID)
	}
	a.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *Auth) SessionHandler(w http.ResponseWriter, r *http.Request) {
	providers := []string{"email"}
	if a.cfg.GithubClientID != "" {
		providers = append(providers, "github")
	}
	u := a.SessionUser(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"authenticated": u != nil,
		"user":          u,
		"providers":     providers,
		"demo":          a.cfg.DemoMode,
	})
}

var emailRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._%+\-]{0,253}@[a-z0-9.\-]{1,253}\.[a-z]{2,}$`)

func validEmail(e string) string {
	e = strings.ToLower(strings.TrimSpace(e))
	if !emailRe.MatchString(e) {
		if _, err := mail.ParseAddress(e); err != nil {
			return ""
		}
		return strings.ToLower(e)
	}
	return e
}

func (a *Auth) authBruteCheck(w http.ResponseWriter, r *http.Request) bool {
	ip := clientIP(r)
	n := 0
	if v, ok := a.authRate.Get(ip); ok {
		n = v
	}
	if n >= 20 {
		writeErr(w, http.StatusTooManyRequests, "auth_rate_limited",
			"Juda ko'p urinish — 10 daqiqadan keyin qayta urinib ko'ring")
		return false
	}
	a.authRate.Add(ip, n+1)
	return true
}

func (a *Auth) SignupHandler(w http.ResponseWriter, r *http.Request) {
	if !a.authBruteCheck(w, r) {
		return
	}
	var body struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	email := validEmail(body.Email)
	if email == "" {
		writeErr(w, http.StatusBadRequest, "invalid_email", "Email noto'g'ri")
		return
	}
	if len(body.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "weak_password", "Parol kamida 8 belgi bo'lsin")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = strings.SplitN(email, "@", 2)[0]
	} else {
		name = clipRunes(name, 80)
	}

	if _, err := a.core.FindUserByEmail(r.Context(), email); err == nil {
		writeErr(w, http.StatusConflict, "email_taken", "Bu email allaqachon ro'yxatdan o'tgan")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash_failed", "Parol saqlanmadi")
		return
	}

	u := &User{
		ID: newID("usr"), Email: email, Name: name,
		Provider: "email", ProviderID: "email:" + email,
		IsAdmin: a.isAdminEmail(email), PasswordHash: string(hash),
	}
	u, err = a.core.UpsertOAuthUser(r.Context(), u)
	if err != nil {
		writeErr(w, http.StatusConflict, "signup_failed", "Hisob yaratilmadi — email band bo'lishi mumkin")
		return
	}
	tok, err := a.IssueJWT(u)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token_failed", "Token yaratib bo'lmadi")
		return
	}
	a.setSessionCookie(w, tok)
	a.userCache.Add(u.ID, u)
	writeJSON(w, http.StatusCreated, map[string]any{
		"ok": true, "user": u, "token": tok,
	})
}

func (a *Auth) LoginEmailHandler(w http.ResponseWriter, r *http.Request) {
	if !a.authBruteCheck(w, r) {
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	email := validEmail(body.Email)
	if email == "" || body.Password == "" {
		writeErr(w, http.StatusBadRequest, "invalid_credentials", "Email va parol kiriting")
		return
	}
	u, err := a.core.FindUserByEmail(r.Context(), email)
	if err != nil || !u.HasPassword() {
		writeErr(w, http.StatusUnauthorized, "invalid_credentials", "Email yoki parol noto'g'ri")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(body.Password)) != nil {
		writeErr(w, http.StatusUnauthorized, "invalid_credentials", "Email yoki parol noto'g'ri")
		return
	}
	if u.Banned {
		writeErr(w, http.StatusForbidden, "account_suspended", "Hisob to'xtatilgan")
		return
	}
	tok, err := a.IssueJWT(u)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token_failed", "Token yaratib bo'lmadi")
		return
	}
	a.setSessionCookie(w, tok)
	a.userCache.Add(u.ID, u)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "user": u, "token": tok,
	})
}

func (a *Auth) ChangePasswordHandler(w http.ResponseWriter, r *http.Request) {
	u := a.SessionUser(r)
	if u == nil {
		if key := extractAPIKey(r); key != "" {
			if entry, e := a.resolveAPIKey(r, key); e == nil {
				u = entry.User
			}
		}
	}
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "unauthenticated", "Tizimga kiring")
		return
	}
	if !u.HasPassword() {
		writeErr(w, http.StatusBadRequest, "no_password",
			"GitHub hisobida parol yo'q — parol faqat email orqali ro'yxatdan o'tgan hisoblar uchun")
		return
	}
	var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(body.Current)) != nil {
		writeErr(w, http.StatusUnauthorized, "wrong_password", "Joriy parol noto'g'ri")
		return
	}
	if len(body.New) < 8 {
		writeErr(w, http.StatusBadRequest, "weak_password", "Yangi parol kamida 8 belgi bo'lsin")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.New), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash_failed", "Parol saqlanmadi")
		return
	}
	if err := a.core.SetPasswordHash(r.Context(), u.ID, string(hash)); err != nil {
		writeErr(w, http.StatusInternalServerError, "save_failed", "Parol yangilanmadi")
		return
	}
	fresh, _ := a.core.GetUser(r.Context(), u.ID)
	if fresh != nil {
		a.userCache.Add(u.ID, fresh)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Parol yangilandi"})
}

func wantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/html") {
		return false
	}
	return accept == "" || strings.Contains(accept, "application/json") || strings.Contains(accept, "*/*")
}

func safeNext(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "/") {
		return ""
	}
	if strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "/\\") || strings.Contains(raw, "://") {
		return ""
	}
	if strings.ContainsAny(raw, " \t\r\n\"'<>\\") {
		return ""
	}
	return raw
}

func (a *Auth) exchange(ctx context.Context, p *oauthProvider, code string) (string, error) {
	form := url.Values{
		"client_id":     {p.clientID},
		"client_secret": {p.secret},
		"code":          {code},
		"redirect_uri":  {a.cfg.PublicURL + "/auth/" + p.name + "/callback"},
		"grant_type":    {"authorization_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tr struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("token javobi buzuq")
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("access_token olinmadi: %s", tr.Error)
	}
	return tr.AccessToken, nil
}

func (a *Auth) fetchProfile(ctx context.Context, p *oauthProvider, token string) (*User, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.userURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("profil olinmadi (%d)", resp.StatusCode)
	}

	u := &User{Provider: p.name}
	switch p.name {
	case "github":
		var g struct {
			ID        int64  `json:"id"`
			Login     string `json:"login"`
			Name      string `json:"name"`
			Email     string `json:"email"`
			AvatarURL string `json:"avatar_url"`
		}
		if err := json.Unmarshal(body, &g); err != nil {
			return nil, err
		}
		u.ProviderID = fmt.Sprintf("%d", g.ID)
		u.Name, u.AvatarURL = g.Name, g.AvatarURL
		u.Email = strings.ToLower(g.Email)
		if u.Name == "" {
			u.Name = g.Login
		}
		if u.Email == "" {
			u.Email = a.githubPrimaryEmail(ctx, token)
		}
		if u.Email == "" {
			u.Email = g.Login + "@users.noreply.github.com"
		}
	}
	if u.ProviderID == "" {
		return nil, errors.New("provider ID topilmadi")
	}
	return u, nil
}

func (a *Auth) githubPrimaryEmail(ctx context.Context, token string) string {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := a.http.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var list []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&list); err != nil {
		return ""
	}
	for _, e := range list {
		if e.Primary && e.Verified {
			return strings.ToLower(e.Email)
		}
	}
	return ""
}

func newID(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

const (
	mediaIDAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	mediaIDLen      = 8
)

func shortID(n int) string {
	if n <= 0 {
		n = mediaIDLen
	}
	b := make([]byte, n)
	_, _ = rand.Read(b)
	out := make([]byte, n)
	mod := len(mediaIDAlphabet)
	for i := range b {
		out[i] = mediaIDAlphabet[int(b[i])%mod]
	}
	return string(out)
}

func validMediaID(id string) bool {
	n := len(id)
	if n < mediaIDLen || n > 40 {
		return false
	}
	for i := 0; i < n; i++ {
		c := id[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		default:
			return false
		}
	}
	return true
}

func constantTimeEq(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}
