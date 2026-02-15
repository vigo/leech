# Leech v2 Design

Concurrent command-line download manager. Multiple URL desteği (pipe + args).

## 1. Modernizasyon

- `go.mod` → Go 1.26
- `ioutil.ReadAll` → `io.ReadAll`, `ioutil.Discard` → `io.Discard`
- `flag` global state temizliği

## 2. Structured Logging (log/slog)

- Tüm `fmt.Println` debug çıktıları → `slog.Debug` / `slog.Info`
- `-verbose` → `slog.LevelDebug`, default → `slog.LevelInfo`
- User-facing çıktılar (progress, tamamlanma) → `slog.Info`
- İç detaylar (chunk, HTTP status) → `slog.Debug`
- Multi-URL: her log satırında URL bilgisi (`"url", url`)

## 3. Progress Bar (sıfır dependency)

- `\r` ile satır üzerinde güncellenen format: `[████████░░░░] 65% 3.2MB/5MB 1.2MB/s`
- Her dosya kendi satırında (multi-download)
- `io.Reader` wrapper: `progressReader` — okuma sırasında byte sayacını günceller

## 4. Bandwidth Limiter (toplam)

- Token bucket yaklaşımı: paylaşılan `rateLimiter` struct
- `-limit` flag: `--limit 5M` (MB/s)
- `io.Reader` wrapper: `rateLimitedReader` — okumayı throttle eder
- Tüm goroutine'ler aynı limiter'ı paylaşır
- 0 = limitsiz (default)

## 5. Resume (.part dosyaları)

- İndirme sırasında `<filename>.part` dosyasına yaz
- Tamamlanınca `.part` → asıl dosya adına rename
- Tekrar çalıştırılırsa `.part` varsa → boyutunu al, `Range: bytes=<size>-` ile devam et
- Sunucu `Accept-Ranges` desteklemiyorsa → baştan indir

## 6. Single-chunk Fallback

- `Accept-Ranges` header yoksa veya `Content-Length` bilinmiyorsa → tek parça indir
- Chunk'sız indirme: `io.Copy` ile direkt dosyaya yaz

## 7. Fetch Context/Timeout

- `fetch()` fonksiyonuna configurable timeout (default 30s per chunk)
- Parent context ile cancel propagation

## 8. Proje Yapısı

```
app/
  app.go          → CLIApplication, Run(), flag parsing
  download.go     → download(), fetch(), resource struct
  progress.go     → progressReader, progress bar rendering
  ratelimit.go    → rateLimitedReader, token bucket
  resume.go       → .part dosya yönetimi
  helpers.go      → URL parse, extension finder, chunk calculator
  version.go      → (aynı kalır)
  usage.go        → (güncellenir: yeni flag'ler)
  *_test.go       → her dosya için testler
main.go           → (aynı kalır)
```

## 9. CLI Flag'leri

```
-version          versiyon bilgisi
-verbose          debug log seviyesi
-chunks N         chunk sayısı (default: 5)
-limit RATE       bandwidth limiti (örn: 5M, 500K) — 0 = limitsiz
-output DIR       indirme dizini (default: .)
```

## 10. golangci-lint v2

- `.golangci.yml` → v2 formatına migration
- Kaldırılan linter'lar: `exportloopref` (Go 1.22+), `rowserrcheck`, `makezero`
- Yeni: `copyloopvar`, `intrange`, `perfsprint`
- Makefile ile `lint` komutu
