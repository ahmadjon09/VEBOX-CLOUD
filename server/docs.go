package main

import (
	"net/http"
)

func (app *App) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	base := app.cfg.PublicURL
	spec := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "VEBOX API",
			"version":     Version,
			"description": "Private media storage and delivery API: instant image uploads (RAM-first), CDN delivery, signed share links, public status page. Auth: API key, GitHub OAuth, or email + password. Formats: all common images (no GIF) and M4A/MP3 audio. Video is not supported. The public delivery surface is intentionally minimal: GET /cdn/{id} (with optional ?w=) and GET /preview/{id} only — every other endpoint needs an API key or a session.",
		},
		"servers": []map[string]string{{"url": base}},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"ApiKeyAuth": map[string]string{"type": "apiKey", "in": "header", "name": "X-API-Key"},
				"BearerAuth": map[string]string{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"},
			},
			"schemas": map[string]any{
				"Image": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]string{"type": "string", "example": "k7m2xQ9a"},
						"bg": map[string]string{"type": "string", "example": "#c87828", "description": "Average background color (#rrggbb) used as a loading placeholder"},
						"colors": map[string]any{"type": "array", "description": "Up to 4 dominant colors of the image, most used first",
							"items": map[string]any{"type": "object", "properties": map[string]any{
								"hex": map[string]string{"type": "string", "example": "#c87828"},
								"pct": map[string]any{"type": "number", "format": "double", "example": 31.4},
							}}},
						"filename":   map[string]string{"type": "string"},
						"mime_type":  map[string]string{"type": "string"},
						"kind":       map[string]any{"type": "string", "enum": []string{"image", "audio"}},
						"size_bytes": map[string]string{"type": "integer", "format": "int64"},
						"size_kb":    map[string]string{"type": "number", "format": "double"},
						"width":      map[string]string{"type": "integer"},
						"height":     map[string]string{"type": "integer"},
						"duration":   map[string]string{"type": "number", "description": "audio only, seconds"},
						"checksum":   map[string]string{"type": "string"},
						"visibility": map[string]string{"type": "string"},
						"created_at": map[string]string{"type": "string", "format": "date-time"},
						"urls": map[string]any{"type": "object", "properties": map[string]any{
							"cdn": map[string]string{"type": "string"}, "preview": map[string]string{"type": "string"},
							"original": map[string]string{"type": "string"}, "hd": map[string]string{"type": "string"},
							"thumbnail": map[string]string{"type": "string"}, "download": map[string]string{"type": "string"},
						}},
					},
				},
				"User": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":         map[string]string{"type": "string"},
						"email":      map[string]string{"type": "string"},
						"name":       map[string]string{"type": "string"},
						"avatar_url": map[string]string{"type": "string"},
						"provider":   map[string]any{"type": "string", "enum": []string{"email", "github"}},
						"is_admin":   map[string]string{"type": "boolean"},
						"banned":     map[string]string{"type": "boolean"},
					},
				},
				"APIKey": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":           map[string]string{"type": "string"},
						"name":         map[string]string{"type": "string"},
						"prefix":       map[string]string{"type": "string"},
						"created_at":   map[string]string{"type": "string", "format": "date-time"},
						"last_used_at": map[string]string{"type": "string", "format": "date-time"},
					},
				},
				"Error": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ok": map[string]string{"type": "boolean"},
						"error": map[string]any{"type": "object", "properties": map[string]any{
							"code":      map[string]string{"type": "string"},
							"message":   map[string]string{"type": "string"},
							"status":    map[string]string{"type": "integer"},
							"timestamp": map[string]string{"type": "string", "format": "date-time"},
						}},
					},
				},
			},
		},
		"security": []map[string]any{{"ApiKeyAuth": []string{}}},
		"paths": map[string]any{
			"/v1/auth/signup": map[string]any{
				"post": map[string]any{
					"summary":  "Create an account with email + password",
					"security": []map[string]any{},
					"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{
						"schema": map[string]any{"type": "object", "properties": map[string]any{
							"name":     map[string]string{"type": "string"},
							"email":    map[string]string{"type": "string"},
							"password": map[string]any{"type": "string", "minLength": 8},
						}, "required": []string{"email", "password"}}}}},
					"responses": map[string]any{
						"201": map[string]any{"description": "Created (returns user + JWT token)"},
						"400": map[string]any{"description": "Invalid email / weak password"},
						"409": map[string]any{"description": "Email already registered"},
						"429": map[string]any{"description": "Too many attempts"},
					},
				},
			},
			"/v1/auth/login": map[string]any{
				"post": map[string]any{
					"summary":  "Sign in with email + password",
					"security": []map[string]any{},
					"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{
						"schema": map[string]any{"type": "object", "properties": map[string]any{
							"email":    map[string]string{"type": "string"},
							"password": map[string]string{"type": "string"},
						}, "required": []string{"email", "password"}}}}},
					"responses": map[string]any{
						"200": map[string]any{"description": "OK (returns user + JWT token)"},
						"401": map[string]any{"description": "Invalid credentials"},
						"403": map[string]any{"description": "Account suspended"},
						"429": map[string]any{"description": "Too many attempts"},
					},
				},
			},
			"/v1/auth/change-password": map[string]any{
				"post": map[string]any{
					"summary": "Change password (email accounts only)",
					"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{
						"schema": map[string]any{"type": "object", "properties": map[string]any{
							"current": map[string]string{"type": "string"},
							"new":     map[string]any{"type": "string", "minLength": 8},
						}, "required": []string{"current", "new"}}}}},
					"responses": map[string]any{"200": map[string]any{"description": "OK"}, "400": map[string]any{"description": "Weak password"}, "401": map[string]any{"description": "Wrong current password"}},
				},
			},
			"/v1/status": map[string]any{
				"get": map[string]any{
					"summary":   "Public status page data (uptime, checks, 90-day history, incidents)",
					"security":  []map[string]any{},
					"responses": map[string]any{"200": map[string]any{"description": "OK"}, "503": map[string]any{"description": "Degraded"}},
				},
			},
			"/auth/session": map[string]any{
				"get": map[string]any{
					"summary":   "Current session + available login providers (email, github if configured)",
					"security":  []map[string]any{},
					"responses": map[string]any{"200": map[string]any{"description": "OK"}},
				},
			},
			"/auth/github/login": map[string]any{
				"get": map[string]any{
					"summary":    "Start GitHub OAuth (redirects to github.com)",
					"security":   []map[string]any{},
					"parameters": []map[string]any{{"name": "next", "in": "query", "schema": map[string]string{"type": "string"}}},
					"responses":  map[string]any{"302": map[string]any{"description": "Redirect to GitHub"}},
				},
			},
			"/auth/logout": map[string]any{
				"post": map[string]any{"summary": "Clear session cookie",
					"responses": map[string]any{"200": map[string]any{"description": "OK"}}},
			},
			"/v1/images": map[string]any{
				"post": map[string]any{
					"summary":     "Upload media (image or M4A/MP3 audio)",
					"description": "Accepts multipart/form-data (field: file) or a raw byte stream. Supported images: JPEG, PNG, WebP, BMP, TIFF, ICO, SVG, AVIF, HEIC. Supported audio: M4A and MP3. GIF and video are NOT supported (415). A single file is limited to 50 MB on every plan. Images use RAM-first processing; audio is sent to storage once, byte-for-byte. The format is detected from the file bytes. Visibility can be set with the `visibility` or `mode` parameter (query string or form field): public | private. Default: the server's DEFAULT_VISIBILITY setting.",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"multipart/form-data": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"file":       map[string]any{"type": "string", "format": "binary", "description": "Image or M4A/MP3; maximum 50 MB per file on every plan."},
										"visibility": map[string]any{"type": "string", "enum": []string{"public", "private"}, "default": "private", "description": "Omitted -> server DEFAULT_VISIBILITY"},
										"mode":       map[string]any{"type": "string", "enum": []string{"public", "private"}, "description": "Alias of visibility"},
									},
									"required": []string{"file"},
								},
							},
						},
					},
					"responses": map[string]any{
						"201": map[string]any{"description": "Created"},
						"402": map[string]any{"description": "Quota exceeded"},
						"413": map[string]any{"description": "File too large"},
						"415": map[string]any{"description": "Unsupported format (GIF, video, …)"},
						"429": map[string]any{"description": "Rate limit exceeded"},
						"503": map[string]any{"description": "Server busy"},
					},
				},
				"get": map[string]any{
					"summary":     "List media links only (no metadata)",
					"description": "Compact response on purpose: every row contains only {id, kind, visibility, urls}. Call GET /v1/images/details?ids=... or GET /v1/images/{id} for size, dimensions, colors and the rest.",
					"parameters": []map[string]any{
						{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer", "default": 30, "maximum": 100}},
						{"name": "offset", "in": "query", "schema": map[string]any{"type": "integer", "default": 0}},
						{"name": "kind", "in": "query", "schema": map[string]any{"type": "string", "enum": []string{"all", "image", "audio", "svg", "gif"}, "default": "all"}},
						{"name": "visibility", "in": "query", "schema": map[string]any{"type": "string", "enum": []string{"public", "private"}}},
						{"name": "q", "in": "query", "schema": map[string]string{"type": "string"}, "description": "Case-insensitive file name search"},
						{"name": "sort", "in": "query", "schema": map[string]any{"type": "string", "enum": []string{"newest", "oldest", "largest", "smallest", "views", "name"}, "default": "newest"}},
					},
					"responses": map[string]any{"200": map[string]any{"description": "OK — {ok, data: [{id, kind, visibility, urls}], pagination}"}},
				},
				"patch": map[string]any{
					"summary":     "Bulk visibility change",
					"description": "Set visibility for up to 100 files at once (?ids=a,b,c&visibility=public|private).",
					"parameters": []map[string]any{
						{"name": "ids", "in": "query", "required": true, "schema": map[string]any{"type": "string"}, "description": "Comma separated ids (max 100)"},
						{"name": "visibility", "in": "query", "required": true, "schema": map[string]any{"type": "string", "enum": []string{"public", "private"}}},
					},
					"responses": map[string]any{"200": map[string]any{"description": "Updated count"}, "400": map[string]any{"description": "Bad request"}},
				},
				"delete": map[string]any{
					"summary":     "Bulk delete",
					"description": "Permanently delete up to 100 files (?ids=a,b,c). Returns deleted_count and failed_count.",
					"parameters": []map[string]any{
						{"name": "ids", "in": "query", "required": true, "schema": map[string]any{"type": "string"}, "description": "Comma separated ids (max 100)"},
					},
					"responses": map[string]any{"200": map[string]any{"description": "Deleted"}, "400": map[string]any{"description": "Bad request"}},
				},
			},
			"/v1/images/details": map[string]any{
				"get": map[string]any{
					"summary":     "Batch metadata for up to 100 of your own files",
					"description": "Kept separate from the list endpoint so that listing stays cheap: returns full PublicImage objects (size, dimensions, bg, colors, urls) only for the given ids.",
					"parameters": []map[string]any{
						{"name": "ids", "in": "query", "required": true, "schema": map[string]any{"type": "string"}, "description": "Comma separated ids (max 100)"},
					},
					"responses": map[string]any{"200": map[string]any{"description": "OK — {ok, data: [Image]}"}, "400": map[string]any{"description": "ids missing"}},
				},
			},
			"/v1/images/{id}": map[string]any{
				"parameters": []map[string]any{
					{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"get": map[string]any{"summary": "Get media metadata (bg + dominant colors included)",
					"responses": map[string]any{"200": map[string]any{"description": "OK"}, "404": map[string]any{"description": "Not found"}}},
				"patch": map[string]any{"summary": "Change visibility",
					"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{
						"schema": map[string]any{"type": "object", "properties": map[string]any{
							"visibility": map[string]any{"type": "string", "enum": []string{"public", "private"}}}}}}},
					"responses": map[string]any{"200": map[string]any{"description": "OK"}, "404": map[string]any{"description": "Not found"}}},
				"delete": map[string]any{"summary": "Delete a file",
					"responses": map[string]any{"200": map[string]any{"description": "Deleted"}, "404": map[string]any{"description": "Not found"}}},
			},
			"/v1/images/{id}/share": map[string]any{
				"parameters": []map[string]any{
					{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"post": map[string]any{
					"summary":     "Create a signed share link",
					"description": "Returns /s/{id}?exp=...&sig=... which opens the file WITHOUT an API key until it expires (1 minute to 30 days).",
					"requestBody": map[string]any{"required": false, "content": map[string]any{"application/json": map[string]any{
						"schema": map[string]any{"type": "object", "properties": map[string]any{
							"ttl_seconds": map[string]any{"type": "integer", "default": 86400, "minimum": 60, "maximum": 2592000}}}}}},
					"responses": map[string]any{"200": map[string]any{"description": "Signed URL"}, "404": map[string]any{"description": "Not found"}},
				},
			},
			"/v1/profile": map[string]any{
				"get": map[string]any{"summary": "Current user profile",
					"responses": map[string]any{"200": map[string]any{"description": "OK"}}},
				"patch": map[string]any{
					"summary": "Update profile (name)",
					"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{
						"schema": map[string]any{"type": "object", "properties": map[string]any{
							"name": map[string]any{"type": "string", "maxLength": 80}}}}}},
					"responses": map[string]any{"200": map[string]any{"description": "OK"}},
				},
			},
			"/v1/profile/avatar": map[string]any{
				"post": map[string]any{
					"summary": "Upload a new avatar (stored like regular images, max 5 MB)",
					"requestBody": map[string]any{"required": true, "content": map[string]any{
						"multipart/form-data": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{
							"file": map[string]any{"type": "string", "format": "binary", "description": "Image (GIF/video not allowed)"},
						}}}},
					},
					"responses": map[string]any{"200": map[string]any{"description": "OK (returns updated user)"}, "413": map[string]any{"description": "File too large"}, "415": map[string]any{"description": "Unsupported format"}},
				},
			},
			"/v1/keys": map[string]any{
				"get": map[string]any{"summary": "List your API keys (never shows full keys)",
					"responses": map[string]any{"200": map[string]any{"description": "OK"}}},
				"post": map[string]any{
					"summary": "Create an API key (plain key shown ONCE)",
					"requestBody": map[string]any{"required": false, "content": map[string]any{"application/json": map[string]any{
						"schema": map[string]any{"type": "object", "properties": map[string]any{
							"name": map[string]string{"type": "string"}}}}}},
					"responses": map[string]any{"201": map[string]any{"description": "Created"}},
				},
				"delete": map[string]any{
					"summary":     "Permanently delete ALL your API keys",
					"description": "Irreversible: key rows are removed from the database.",
					"responses":   map[string]any{"200": map[string]any{"description": "Deleted"}},
				},
			},
			"/v1/keys/{id}": map[string]any{
				"parameters": []map[string]any{
					{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"delete": map[string]any{
					"summary":     "Permanently delete ONE API key",
					"description": "Irreversible: the key row is removed from the database, not just revoked.",
					"responses":   map[string]any{"200": map[string]any{"description": "Deleted"}, "404": map[string]any{"description": "Not found"}},
				},
			},
			"/v1/me": map[string]any{
				"get": map[string]any{"summary": "Account status, plan and usage",
					"responses": map[string]any{"200": map[string]any{"description": "OK"}}},
			},
			"/v1/analytics": map[string]any{
				"get": map[string]any{"summary": "Usage analytics",
					"parameters": []map[string]any{{"name": "days", "in": "query", "schema": map[string]any{"type": "integer", "default": 7, "minimum": 1, "maximum": 90}}},
					"responses":  map[string]any{"200": map[string]any{"description": "OK"}, "402": map[string]any{"description": "Upgrade required (extended details)"}}},
			},
			"/cdn/{id}": map[string]any{
				"get": map[string]any{
					"summary":     "Public CDN delivery — the only keyless image endpoint (no key required for public files)",
					"security":    []map[string]any{},
					"description": "HD version with long-lived immutable caching. Optional ?w= resize for images (depends on owner's plan). Metadata is served from a two-level cache (in-process LRU + Redis) so the first byte is fast; hotlink protection may still apply.",
					"parameters": []map[string]any{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
						{"name": "w", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 1, "maximum": 4096}},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Binary stream"},
						"304": map[string]any{"description": "Not modified"},
						"404": map[string]any{"description": "Not found or private"},
					},
				},
			},
			"/preview/{id}": map[string]any{
				"get": map[string]any{
					"summary":     "Tiny color swatch (no key required for public images)",
					"description": "8x8 SVG built from the dominant colors of the image — used as the loading placeholder, costs no storage bandwidth.",
					"security":    []map[string]any{},
					"parameters": []map[string]any{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]any{"200": map[string]any{"description": "8×8 SVG color swatch"}},
				},
			},
			"/i/{id}": map[string]any{
				"get": map[string]any{
					"summary":  "Stream a file (public: no key; private: owner's key)",
					"security": []map[string]any{{}, {"ApiKeyAuth": []string{}}},
					"parameters": []map[string]any{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
						{"name": "v", "in": "query", "schema": map[string]any{"type": "string", "enum": []string{"hd", "thumb"}}},
						{"name": "w", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 1, "maximum": 4096}},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Binary stream"},
						"304": map[string]any{"description": "Not modified"},
						"206": map[string]any{"description": "Partial content"},
					},
				},
			},
			"/d/{id}": map[string]any{
				"get": map[string]any{
					"summary":     "Download with original filename",
					"security":    []map[string]any{},
					"description": "Streams the file with Content-Disposition: attachment. Private files need api_key or sig.",
					"parameters": []map[string]any{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Binary stream (attachment)"},
						"206": map[string]any{"description": "Partial content"},
						"404": map[string]any{"description": "Not found or private"},
					},
				},
			},
			"/s/{id}": map[string]any{
				"get": map[string]any{
					"summary":     "Signed share link (no key needed)",
					"security":    []map[string]any{},
					"description": "Opens a private file with a time limited HMAC signature created by POST /v1/images/{id}/share.",
					"parameters": []map[string]any{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
						{"name": "exp", "in": "query", "required": true, "schema": map[string]any{"type": "integer"}},
						{"name": "sig", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Binary stream"},
						"206": map[string]any{"description": "Partial content"},
						"404": map[string]any{"description": "Bad or expired signature"},
					},
				},
			},
			"/healthz": map[string]any{
				"get": map[string]any{
					"summary":   "Health check (public: only status)",
					"security":  []map[string]any{},
					"responses": map[string]any{"200": map[string]any{"description": "OK"}, "503": map[string]any{"description": "Degraded"}},
				},
			},
		},
	}
	w.Header().Set("Cache-Control", "public, max-age=600")
	writeJSON(w, http.StatusOK, spec)
}
