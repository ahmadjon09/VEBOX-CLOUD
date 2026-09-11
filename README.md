# VEBOX — Media Cloud API

Rasm va M4A/MP3 audio yuklash uchun ochiq API: bitta `POST`, javobda tayyor CDN
havola. Har rasm uchun fon rangi va dominant ranglar palitrasi, konsol,
analitika, imzolangan havolalar, admin panelda server/DB/bot holati va ochiq
status sahifasi bilan. Redis — qo'shimcha (aux) kesh.

## Tuzilma

Ikkita mustaqil loyiha — har biri alohida serverda joylashadi (Node tomoni — bitta npm workspace):

```
server/      Go backend (standalone, bitta paket). API + media yetkazib berish.
web/         React frontend. Vite + React 19 + Tailwind 4.
netlify.toml Netlify konfiguratsiyasi (faqat web/ build qilinadi).
```

`web/` — npm workspace, shuning uchun **bitta** lockfile repo ildizida
(`package-lock.json`) va barcha `npm` buyruqlari ildizda bajariladi. Ildizda
`package.json` bo'lgani uchun npm har doim workspace root'ni izlaydi: `cd web`
ichida `npm ci` ishlamaydi, agar ildizda lockfile bo'lmasa (`EUSAGE`).
Shu sababli `web/package-lock.json` yaratilmaydi.

### Frontend strukturasi (`web/src`)

```
App.tsx              router — har sahifa alohida chunk (React.lazy)
components/          umumiy UI: Layout, ui (tugma/modal/toast), VsWindow,
                     MediaGrid, Palette (ranglar), AudioPlayer, UploadZone,
                     Charts, Seo, Logo
components/landing/  landing bloklari: HeroTerminal, MediaWall,
                     StatusPanel, CodeExample, Pricing
pages/               har route uchun sahifa (konsolda modal ham route)
providers/           React konteyneri (AuthProvider)
services/            api.ts (axios + localStorage), media.ts (SWR so'rovlari)
hooks/               useHotkeys
utils/               format.ts (son/sana/hajm), tiers.ts (tarif raqamlari),
                     host.ts (faqat-status rejimi)
i18n/                translations.ts (uz/en/ru lug'at) + context.tsx
test/                smoke, api, media va i18n coverage testlari
```

### Backend (`server/`)

Bitta Go paketi, fayllar funksiya bo'yicha: `main.go` (router/bootstrap),
`config.go` (barcha ENV shu yerdan), `auth.go`, `storage.go` (upload
pipeline), `stream.go` (delivery/handlers), `media.go` (format aniqlash),
`image.go` (rasm qayta ishlash, palitra), `tier.go` (tariflar + rate limiter),
`redis.go` (aux kesh), `hotlink.go` (referer himoyasi), `status.go`,
`store.go` (Postgres/Mongo/Mem), `vault.go` (saqlash pooli), `memory.go`,
`sysinfo_linux.go`/`sysinfo_other.go`, `admin.go`, `docs.go` (OpenAPI),
`models.go`.

## Nima yuklash mumkin

- **Rasmlar:** JPEG, PNG, WebP, BMP, TIFF, ICO, SVG, AVIF, HEIC/HEIF.
  GIF — yo'q.
- **Audio:** M4A va MP3 (bayt-ma-bayt, transkod yo'q). MP3 ID3 tagi, CRC
  bilan/bilan-kadrsiz freym ketma-ketligi va VBR to'g'ri aniqlanadi.
- **Video:** yo'q — `415 video_not_supported`. `ALLOW_VIDEO` o'qilmaydi.
- **HD:** JPEG/PNG avtomatik optimallashtiriladi, profildagi sozlamadan
  o'chiriladi.
- Har rasm uchun **`bg`** (fon rangi) va **`colors`** — dominant ranglar
  palitrasi (`PALETTE_COLORS`, 1..8, standart 4). Fayl sahifasida chiziqlar
  ko'rinishida, fayllar ro'yxatida nuqtalar ko'rinishida chiqadi.

Format faqat fayl baytlaridan aniqlanadi — kengaytma va `Content-Type`
g'iroq. Har fayl maksimal 50 MB.

## Limitlar (haqiqiy raqamlar, `server/tier.go`)

Xizmat **mutlaqo tekin**. Ochiq chegara faqat:

| | |
|---|---|
| Narx | 0$ |
| Bitta fayl | 50 MB |
| So'rov/daqiqa (kalit bilan) | 120 (burst 60) |
| So'rov/daqiqa (anonim CDN) | `PUBLIC_RATE_PER_MIN` = 600 (IP bo'yicha) |
| Saqlash / fayl/oy | cheksiz |
| Trafik | cheksiz |
| HD / analitika / `?w=` | ochiq |

Yashirin `business` limiti faqat admin panelda, landing'da yo'q.
Frontend raqamlari: `web/src/utils/tiers.ts`.

## Auth

- **GitHub OAuth** — konsolga kirish (bir tugma).
- **Email + parol** — signup/login (bcrypt).
- **API kalit** — skriptlar uchun: `X-API-Key: …` header yoki
  `Authorization: Bearer …`. Kalitlar SHA-256 hash bilan saqlanadi,
  ochiq ko'rinishi faqat yaratish paytida bir marta ko'rsatiladi.

Konsol sessiyasi (JWT cookie yoki Bearer token) va API kalit ikkalasi ham
`/v1/*` endpointlarini qabul qiladi.

### GitHub OAuth qanday ishlaydi

Frontend alohida domenda (masalan `vebox.uz`) bo'lsa, GitHub tugmasi
to'g'ridan-to'g'ri API domeniga (`https://api.vebox.uz/auth/github/login`)
olib boradi — nisbiy `/auth/github/login` frontend hosting'da 404 beradi.
Callback'dan keyin server sessiya cookie'sini API domeniga yozadi, frontend
esa boshqa domendagi cookie'ni o'qiy olmaydi. Shuning uchun JWT (va birinchi
kirishdagi yangi API kalit) URL fragmentida (`#token=…&api_key=…`) qaytariladi:
konsol ularni bir marta o'qib localStorage'ga yozadi (Bearer/X-API-Key) va
manzil satridan darhol o'chiradi. Bitta domenda ishlaganda cookie yetarli —
token fragmentga qo'shilmaydi.

To'g'ri ishlashi uchun serverda quyidagilar sozlangan bo'lishi shart:

| Sozlama | Qiymat |
|---|---|
| `PUBLIC_URL` | `https://api.vebox.uz` |
| `WEB_URL` | `https://vebox.uz` (CORS'ga avtomatik qo'shiladi + OAuth'dan keyingi redirect) |
| GitHub App → callback URL | `https://api.vebox.uz/auth/github/callback` |

## Yetkazib berish

Har fayl `visibility` ga ega: **public** (`DEFAULT_VISIBILITY`) yoki
**private**.

| Manzil | Kim ochadi | Nima |
|---|---|---|
| `GET /cdn/{id}` | kalitsiz (public) | HD fayl, `max-age=1y immutable`, ETag/Range |
| `GET /cdn/{id}?w=640` | kalitsiz (public) | joyni o'zida qayta o'lchangan nusxa, Redis'ga keshlanadi |
| `GET /preview/{id}` | kalitsiz (public) | 8×8 SVG — ranglar palitrasi (loading foni) |
| `GET /s/{id}?exp=&sig=` | kalitsiz, muddatgacha | imzolangan vaqtinchalik havola |
| `GET /i/{id}?v=hd&w=` | faqat kalit/sessiya | universal oqim, Range/206 |
| `GET /d/{id}` | faqat kalit/sessiya | asl nom bilan yuklab olish |
| private fayl | faqat egasi yoki admin | boshqalarga 404 |

`/cdn`, `/preview`, `/s` — kalitsiz ishlaydigan yagona ochiq yuza; `GET
/v1/images` ro'yxatida ham fayllar shu `urls` bilan beriladi. Kalitsiz
`/i` yoki `/d` so'rovi `401` qaytaradi (JSON so'ralsa JSON, `Accept: image/*`
bo'lsa placeholder SVG).

### Ro'yxat faqat havolalar uchun

`GET /v1/images` javobida har fayl bo'yicha **faqat** `id`, `kind`,
`visibility` va `urls` bor — meta bilan dump qilinmaydi, shu sababli ro'yxat
juda tez va kichik javob beradi (`ETag` + 304 qo'llab-quvvatlanadi):

```json
{ "ok": true, "data": [
  { "id": "k7m2xQ9a", "kind": "image", "visibility": "public",
    "urls": { "cdn": "https://api.vebox.uz/cdn/k7m2xQ9a",
              "preview": "https://api.vebox.uz/preview/k7m2xQ9a",
              "download": "https://api.vebox.uz/d/k7m2xQ9a" } }
], "pagination": { "next_cursor": null } }
```

Hajm, o'lcham, palitra va boshqa metani fayl sahifasi ochilganda alohida
so'rov oladi: `GET /v1/images/details?ids=a,b` (100 tagacha id, faqat so'rov
egasi own fayllari). Filtrlar: `kind`, `visibility`, `q`, `sort`, `limit`
(≤100), `cursor`.

## Redis — faqat qo'shimcha kesh

Redis — **aux** qatlam, manba emas: Postgres/Mongo va jarayon ichidagi LRU
asosiy ma'lumotni ushlab turadi, Redis o'chsa ham hech narsa buzilmaydi
(har so'rov 200ms timeout bilan fail-open).

| ENV | Ma'nosi |
|---|---|
| `REDIS_URL` | `redis://host:6379` yoki `rediss://…` (bo'sh = Redis yo'q) |
| `REDIS_MAX_MB` | budjet — **30 MB**; to'lsa eng eski keylar tashlanadi |
| `REDIS_PREFIX` | barcha keylar prefiksi (`vb:`) |
| `REDIS_META_TTL` | fayl metasi (24h) |
| `REDIS_VARIANT_TTL` | `?w=` qayta o'lchangan nusxalar (6h) |
| `REDIS_MAX_VALUE_KB` | bitta qiymat chegarasi (96 KB) — katta bloblar Redis'ga kirmaydi |
| `REDIS_OP_TIMEOUT` | bitta amaliyot (200ms) |
| `REDIS_DIAL_TIMEOUT` | ulanish (1s) |

30 MB uchun kichik muhim: Redis'da faqat kichik JSON metalar va variantlar
saqlanadi, fayl baytlari saqlanmaydi. Admin panelda kalitlar soni, band
baytlar, hit foizi va xatolar ko'rinadi; `POST /v1/admin/cache/invalidate`
butun namespace'ni tozalaydi.

## Admin panel (`/admin`)

Faol admin (`ADMIN_EMAILS` yoki `role=admin`) `/admin` sahifasida jonli
server holatini ko'radi (`GET /v1/admin/system`, 20 soniyada bir yangilanadi):

- **System** — uptime, Go versiyasi, OS/arch, CPU soni, goroutine, GC
  pauza, PID, hostname, disk hajmi va bo'sh joyi;
- **Memory governor** — limit/foiz, heap, sys, heavy slotlar, `state`
  (`ok`/`degraded`); `/healthz` ham shu holatni qaytaradi;
- **Databases** — Postgres: umumiylashgan hajm, jadvallar va satrlar, pool
  holati; Mongo: `dbStats`, bo'sh joy, koleksiyalar soni;
- **Bots (saqlash pooli)** — har token uchun available, inflight, ok/failed,
  flooding`dan chiqish vaqti (`blocked_for_s`);
- **Jobs** — navbat uzunligi/sig'imi, staging, dedup va meta kesh;
- **Redis** — ulangganligi, hit %, keylar, band bayt/byudjet, xatolar;
- **Tiers/Config** — haqiqiy limitlar va o'chirilgan (`allow_audio`,
  `hd_max_width`, `max_audio_mb`) sozlamalar; keshni tozalash tugmasi.

## Xavfsizlik

- **Hotlink himoyasi** — ochiq media yo'llari (`/cdn`, `/preview`) `Referer`/
  `Origin` ni tekshiradi: ruxsat etilgan manbalar — API hosti, `WEB_URL`,
  `CORS_ORIGINS` va `HOTLINK_ALLOWED_ORIGINS`. Referer'siz so'rovlar (curl,
  ilovalar) va sessiya/kalit bilan kelgan egalar ta'sirlanmaydi. Imzolangan
  `/s/` havolalar cheklovdan tashqari. O'chirish: `HOTLINK_PROTECT=0`.
- **CORS** — `CORS_ORIGINS` bo'sh bo'lsa cross-origin umuman ochilmaydi.
- **Rate limiting** — har foydalanuvchiga tarif limiti, anonim media
  so'rovlariga `PUBLIC_RATE_PER_MIN` (IP bo'yicha), login/signup'ga
  brutfors qopqog'i (IP, 10 daqiqada 20 urinish).
- **Input validation** — format baytlardan aniqlanadi, hajm chegarasi
  pre-flight + `MaxBytesReader`, JSON tanasi 1 MB limit, Content-Disposition
  fayl nomi saralanadi, qidiruv/nom maydonlari rune-himoyali chegaralanadi.
- **Xavfsiz sarlavhalar** — `X-Content-Type-Options`, `X-Frame-Options`,
  `Referrer-Policy`, `Permissions-Policy`, COOP; HTTPS'da `Strict-Transport-
  Security`; API javoblariga `default-src 'none'` CSP; foydalanuvchi SVG'lari
  sandbox CSP bilan beriladi. Frontend build'iga ham CSP meta qo'shiladi
  (`vite.config.ts`).
- **Maxfiylik** — `JWT_SECRET` production'da majburiy (>=32 belgi);
  kalitlar faqat hash; saqlash qatlami (vault) nomi hech qayerda ochilmaydi;
  private fayl mavjudligi oshkor qilinmaydi (404); OAuth `next` manzilida
  open-redirect himoyasi; sessiya cookie'si HttpOnly+SameSite=Lax.
  Serverda hech qanday secret commit qilinmaydi — barchasi ENV (`server/.env`).

## Frontend (`web/`)

- Vite 7 + React 19 + TypeScript + Tailwind CSS 4
- **SWR** (cache/revalidate) va **Axios** (HTTP) — boshqa state kutubxonasi yo'q
- Ikonalar: **lucide-react** (interfeysda emoji yo'q)
- Har sahifa router'da alohida route; modal ham route (URL'da turadi)
- Diagrammalar — recharts (Analitika, lazy chunk)
- i18n: o'zbek / english / russian (`web/src/i18n/translations.ts`) — har
  matn uchun kalit, uch tilda ham mavjudligi va kalitning ishlatilishi
  `web/test/i18n.test.ts` bilan tekshiriladi
- Interfeys — VS Code dark uslubi: title-bar, editor tablar, status bar
- SEO: har sahifada title/description/canonical + Schema.org
- Landing'dagi status kartasi ochiq `/v1/status` endpointidan jonli ma'lumot
  oladi (backend bo'lmasa "offline" holatini ko'rsatadi, soxta raqam yo'q)

### Ishga tushirish

```bash
npm ci               # repo ildizida (yoki: npm install)
npm run dev          # vite 5173; /v1, /auth, /cdn… proksi orqali 127.0.0.1:8080 ga
```

Backend boshqa portda bo'lsa: `VITE_DEV_PROXY=http://127.0.0.1:8080 npm run
dev`. API alohida domenda bo'lsa `web/.env` ichida `VITE_API_BASE=https://
api.example.com` (bo'sh qator — nisbiy manzil, ya'ni shu sayt orqali).

### Test va build

```bash
npm run typecheck    # tsc --noEmit
npm test             # smoke + api + media + i18n coverage (jsdom)
npm run build        # web/dist/ (CSP meta build'da qo'shiladi)
```

### Netlify (frontend)

Konfig — `netlify.toml`: `npm run build` ildizda, publish `web/dist`, Node 22.
Deploy sozlamalari UI'da ham bo'lsa, `netlify.toml` ustun turadi. Frontend
API'ni `VITE_API_BASE` orqali oladi (bo'sh bo'lsa nisbiy manzil — `[[redirects]]`
yordamida Netlify'dan API'ga proksi qo'shiladi):

```bash
netlify build                # lokal CLI bilan tekshirish
netlify deploy --build       # preview
netlify deploy --build --prod
```

Yangi lockfile kerak bo'lsa: ildizda `npm install` (hech qachon `cd web`
ichida emas).

### Faqat-status rejimi

`?status=1` yoki `VITE_STATUS_ONLY=1` bilan ishga tushirilganda faqat Status
sahifasi ochiladi, qolgan barcha route'lar `/` ga qaytariladi
(`web/src/utils/host.ts`).

## Backend (`server/`)

```bash
cd server
cp .env.example .env   # o'z qiymatlaringizni kiriting
go mod tidy
go run .               # bazasiz => demo rejim, konsolda API Key chiqadi
go test ./...          # media probe, vault, delete, upload xatolari
```

Sozlamalar `.env` yoki muhit o'zgaruvchilari orqali beriladi (`.env`
git'ga tushmaydi, muhitdagi qiymat ustun yozadi). `server/.env.example` —
haqiqiy ishlatiladigan barcha kalitlar, izohsiz; defaultlar `config.go` da.

Muhim ENV'lar:

| ENV | Nega kerak |
|---|---|
| `PORT`, `PUBLIC_URL` | HTTP + to'liq havolalar uchun |
| `WEB_URL`, `CORS_ORIGINS` | frontend alohida domenda bo'lsa |
| `HOTLINK_PROTECT`, `HOTLINK_ALLOWED_ORIGINS` | rasm hotlink himoyasi |
| `POSTGRES_DSN`, `MONGO_URI`, `MONGO_DB` | bazalar; ikkalasi bo'sh = in-memory demo |
| `STRICT_DB` | `true` — baza ulanmasa server ishga tushmaydi |
| `JWT_SECRET` | >=32 belgi; production'da bo'lmasa server turmaydi |
| `GITHUB_CLIENT_ID/SECRET` | GitHub OAuth (bo'sh bo'lsa faqat email) |
| `VAULT_TOKENS`, `VAULT_CHAT_ID` | ichki saqlash pooli (foydalanuvchiga ko'rinmaydi) |
| `FLOOD_COOLDOWN`, `VAULT_TIMEOUT`, `FILE_LINK_TTL` | bot pooli va soxta-sidlik holatlari |
| `MAX_AUDIO_MB`, `ALLOW_AUDIO` | audio chegarasi va yoqilganligi |
| `HD_MAX_WIDTH`, `HD_QUALITY` | HD optimallashtirish |
| `PALETTE_COLORS` | nechta dominant rang chiqarilsin (1..8) |
| `DEFAULT_VISIBILITY` | yuklashdan keyingi standart: `public` yoki `private` |
| `SHARE_TTL` | imzolangan havolaning maksimal umri |
| `PUBLIC_RATE_PER_MIN`, `PUBLIC_BURST`, `PUBLIC_MAX_AGE` | kalitsiz CDN holatlari |
| `REDIS_*` | qo'shimcha kesh (yuqoridagi bo'lim) |
| `MEM_LIMIT_MB`, `MEM_HIGH_WATER`, `MEM_CRITICAL`, `MEM_TICK`, `MAX_HEAVY_JOBS`, `LRU_SIZE` | memory governor |
| `KEEPALIVE`, `KEEPALIVE_INTERVAL` | bepul hosting'lar uxlab qolmasligi |
| `ADMIN_EMAILS` | bu emaillar avtomatik admin bo'ladi |

Postgres — metadata (foydalanuvchilar, kalitlar, media ro'yxati),
Mongo — analitika va katta tarixlar. Ikkalasini bo'sh qoldirsangiz —
baza yo'q demo rejim (hammasi xotirada, restart'da yo'qoladi).
