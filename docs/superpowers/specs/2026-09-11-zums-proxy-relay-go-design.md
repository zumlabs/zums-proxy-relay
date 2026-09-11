# Design: zums-proxy-relay (Go rewrite)

Tanggal: 2026-09-11
Status: menunggu review

## Konteks & Tujuan

Port `buff-relay` (Node.js, 1 file) menjadi **zums-proxy-relay** dalam Go, untuk repo
portofolio GitHub (`github.com/zumlabs/zums-proxy-relay`) dengan target: **dapat kerja**.

Prinsip panduan: repo hanya berisi hal yang bisa dipertanggungjawabkan pemiliknya di
interview. Junior-level, jujur, clean.

- Relay ini **tetap dipakai sungguhan** (satu instance per region → satu egress IP),
  jadi disguise nginx **dipertahankan**.
- Zero dependency: hanya Go stdlib (`net/http`, `net`, `crypto/subtle`, `log/slog`),
  sama seperti versi Node-nya tidak pakai library.

## Keputusan yang sudah disepakati

| Topik | Keputusan |
|---|---|
| Bahasa | Go (module `github.com/zumlabs/zums-proxy-relay`, Go stable terbaru, >= 1.22) |
| Arsitektur | Opsi 2: layout service idiomatik `cmd/` + `internal/`, manual wiring, tanpa DI library |
| Fitur | Port 1:1 dari perilaku `server.js` + perbaikan (di bawah) |
| Disguise nginx | Tetap (dipakai nyata) |
| Health check | `/health` (ganti nama dari `/healthz`) |
| Credential default | **Dihapus** — tanpa `AUTH_USER`/`AUTH_PASS` server menolak start (fail-fast) |
| Auth | Constant-time compare (`crypto/subtle.ConstantTimeCompare`) |
| Timeout | Dial upstream 5s; idle tunnel 30s; forward pakai `context` |
| Shutdown | Graceful: `SIGINT`/`SIGTERM` → `server.Shutdown` dengan timeout |
| Logging | `log/slog` JSON ke stdout; `DEBUG=true` → debug, selainnya info |
| Test | Table-driven, stdlib `testing` + `net/http/httptest` |
| CI | GitHub Actions: `gofmt` check, `go vet`, `go test -race` saat push/PR |
| CD | **Tidak ada** (Railway sudah deploy-otomatis dari GitHub) |
| Deploy | Railway (Nixpacks deteksi Go; CONNECT lewat TCP Proxy, bukan domain HTTP) |
| File Node | `server.js`, `package.json` dihapus |
| Lisensi | MIT |

## Struktur repo

```
zums-proxy-relay/
├── cmd/server/main.go         # env → config, validasi, wiring, graceful shutdown
├── internal/proxy/
│   ├── server.go              # http.Handler, landing/404, absolute-form forward, CONNECT
│   ├── auth.go                # Proxy-Authorization Basic, constant-time
│   ├── upstream.go            # parsing host:port (IPv6-aware) + dial dengan timeout
│   ├── server_test.go
│   ├── auth_test.go
│   └── upstream_test.go
├── go.mod
├── Dockerfile                  # multi-stage build → scratch, CMD binary statis
├── Makefile                    # build / run / test / fmt / vet / lint
├── .golangci.yml
├── .env.example
├── .gitignore                  # binari, vendor/, .env, IDE
├── .github/workflows/ci.yml
├── LICENSE                     # MIT
└── README.md                   # badge CI, diagram alur, quickstart, deploy Railway
```

## Perilaku runtime

### Permukaan publik (tanpa auth — supaya probe melihat host biasa)

| Request | Respons |
|---|---|
| `GET /` | `200` halaman "Welcome to nginx!" |
| `GET /health` | `200 text/plain` → `ok` |
| lainnya (bukan bentuk proxy) | `404` HTML ala nginx |

Semua respons membawa `Server: nginx/1.24.0`.

### Permukaan proxy (wajib auth Basic via `Proxy-Authorization`)

| Request | Perilaku |
|---|---|
| `CONNECT host:port` | auth OK → `200 Connection Established`, pipe dua arah (`io.Copy` goroutine dua sisi); auth gagal → `407` + close |
| absolute-form `http://host:port/path` | auth OK → forward ke upstream, stream balik; auth gagal → `407 Proxy-Authenticate: Basic realm="auth"` |

Header yang di-strip saat forward: `proxy-authorization`, `proxy-connection`, `connection`.
`Host` upstream diisi authority dari URL (sama seperti sekarang).

### Fail-fast saat start

`AUTH_USER` atau `AUTH_PASS` kosong → log error jelas, `os.Exit(1)`. Tidak ada nilai
default untuk credential (perbaikan langsung dari bug security versi Node).

### Timeout & resource lifecycle

- `http.Server{ReadHeaderTimeout: 10s}`; `IdleTimeout` secukupnya.
- Dial upstream: `net.Dialer{Timeout: 5s}` / `http.Transport` dengan timeout.
- Tunnel: `SetDeadline`/idle 30s; error di salah satu sisi → tutup dua-duanya, tidak
  ada goroutine bocor (`sync.WaitGroup` per tunnel untuk menunggu kedua arah selesai).
- Graceful shutdown: context signal → `Shutdown(ctx)` 10s → tunnel idle ikut tutup.

### Logging (slog)

- Info: start/stop, tunnel `open/close` (host:port, durasi, byte), error upstream.
- Debug (`DEBUG=true`): request forward, auth failure (tanpa nilai header mentah), raw CONNECT.
- Tidak pernah me-log credential.

## Error handling

- Upstream error sebelum header terkirim → `502 bad gateway` (plain, header Server palsu).
- Request rusak total → `400` (via `ConnError` handler).
- Gagal parse authority/tunnel target → `400` lalu socket ditutup.
- Panic tidak diharapkan dibiarkan crash (supervisor/Railway restart) — bukan middleware
  `recover` serba-menelan; cukup dicatat oleh slog sebelum exit.

## Testing

Stdlib only, tanpa framework, co-located di `internal/proxy`:

1. **upstream_test.go** — tabel parsing authority: `example.com`, `example.com:8080`,
   `[::1]:8080`, tanpa port (default 80 / CONNECT default 443), string rusak → error.
2. **auth_test.go** — kredensial benar, user/pass salah, header absen, base64 rusak,
   skema bukan Basic. (Bukti tertulis kenapa `ConstantTimeCompare`, bukan `==`.)
3. **server_test.go** — `httptest`:
   - `GET /` dan `GET /health` → 200 + header `Server: nginx/1.24.0`
   - path aneh → 404 nginx-style
   - forward via `httptest.Server` upstream: diteruskan + header auth di-strip
   - upstream mati → 502
   - CONNECT: client nyata ↔ server, upstream `net.Listener` palsu, payload dua arah utuh
4. `go test -race` wajib hijau (jadi gate CI).

## CI (GitHub Actions)

`on: push + pull_request`, matrix nggak perlu. Job tunggal:
checkout → setup-go (stable) → `test -z $(gofmt -l .)` → `go vet ./...` →
`go build ./...` → `go test -race ./...`.
README dipasang status badge dari workflow ini.

## Deploy (Railway)

1. Project baru → deploy dari GitHub (Nixpacks build Go, `make build`/`go build` via
   start script atau Procfile-ish default Nixpacks).
2. Variables: `PORT` (di-inject Railway), `HOST=0.0.0.0`, `AUTH_USER`, `AUTH_PASS`
   (credential **baru**, jangan pakai yang lama), `DEBUG=false`.
3. Service → Networking → TCP Proxy → Application Port `8080` → alamat `<domain>:<port>`
   untuk klien; domain `.up.railway.app` hanya untuk permukaan web + `/health`.
4. Verifikasi: `curl /health`, CONNECT dengan auth benar (200), auth salah (407).

Catatan keamanan di README: tunnel tanpa auth = open proxy; karena repo publik,
credential hanya lewat env vars, dan diganti kalau pernah bocor.

## Konfigurasi (env)

| Var | Default | Catatan |
|---|---|---|
| `PORT` | `8080` | Railway inject |
| `HOST` | `0.0.0.0` | bind address |
| `AUTH_USER` | — | **wajib**, kosong = menolak start |
| `AUTH_PASS` | — | **wajib**, kosong = menolak start |
| `DEBUG` | `false` | level log |

## Di luar scope (sengaja tidak dibuat)

- SOCKS5 / rotasi / multi-user / rate limit (yagni; bisa jadi project sendiri nanti)
- CD pipeline (Railway mengurus deploy)
- Endpoint `/healthz` (diganti `/health`)
- Credential default apa pun
- Package `pkg/` publik — tidak ada konsumen eksternal

## Risika yang disadari

- Disguise nginx di repo publik = "resep" terbaca; diterima karena dipakai nyata,
  dan bukan rahasia yang dimaksud (keamanan tetap dari auth).
- CONNECT ke host mana pun = SSRF-capable by design untuk klien ber-auth; itu memang
  fungsi relay. Tanpa auth = open proxy → dicegah fail-fast.
