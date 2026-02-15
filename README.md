![Version](https://img.shields.io/badge/version-0.1.0-orange.svg)
![Go](https://img.shields.io/badge/go-1.26-blue.svg)
[![Run golangci-lint](https://github.com/vigo/leech/actions/workflows/golint.yml/badge.svg)](https://github.com/vigo/leech/actions/workflows/golint.yml)
[![Run go tests](https://github.com/vigo/leech/actions/workflows/gotests.yml/badge.svg)](https://github.com/vigo/leech/actions/workflows/gotests.yml)
[![codecov](https://codecov.io/gh/vigo/leech/graph/badge.svg?token=TDMD5E9N1Y)](https://codecov.io/gh/vigo/leech)

# Leech

Concurrent command-line download manager. Inspired from [Leech](https://manytricks.com/leech/)
macOS application!

## Features

- Concurrent chunked downloads (parallel byte-range fetches)
- Multiple URL support (pipe and/or arguments)
- Progress bar with real-time terminal output
- Bandwidth limiting (shared token bucket across all downloads)
- Resume support (`.part` files, continues from where it left off)
- Single-chunk fallback for servers without `Accept-Ranges`
- Structured logging with `log/slog` (debug mode via `-verbose`)

---

## Requirements

- Go 1.26+

---

## Installation

### Homebrew (macOS)

```bash
brew tap vigo/leech
brew install leech
```

> **Cask Conflict:** Homebrew's official cask registry has an app called
> [Leech](https://manytricks.com/leech/) (a GUI download manager by Many Tricks).
> If that cask is installed, `brew install leech` will install the formula but
> **skip linking the binary**. To fix this:
>
> ```bash
> brew install vigo/leech/leech
> brew link --overwrite leech
> ```

### Go

```bash
go install github.com/vigo/leech@latest
```

### Build from source

```bash
git clone https://github.com/vigo/leech.git
cd leech
make build
```

---

## Usage

```bash
# single URL
leech https://example.com/file.zip

# multiple URLs
leech https://example.com/file1.zip https://example.com/file2.zip

# pipe URLs
cat urls.txt | leech

# with options
leech -verbose -chunks 10 -limit 5M -output ~/Downloads https://example.com/file.zip
```

### Flags

```bash
-version        display version information
-verbose        verbose output / debug logging (default: false)
-chunks N       chunk size for parallel download (default: 5)
-limit RATE     bandwidth limit, e.g. 5M, 500K (default: 0, unlimited)
-output DIR     output directory (default: current directory)
```

### Bandwidth Limit Examples

```bash
leech -limit 1M  ...   # 1 MB/s total
leech -limit 500K ...  # 500 KB/s total
leech -limit 2G  ...   # 2 GB/s total (why not)
```

---

## Development

```bash
make build    # build binary
make test     # run tests with race detector
make lint     # run golangci-lint v2
make clean    # remove binary and .part files
make install  # go install
```

### Pre-commit

```bash
pre-commit install
pre-commit run --all-files
```

---

## Contributor(s)

* [Uğur "vigo" Özyılmazel](https://github.com/vigo) - Creator, maintainer

---

## Contribute

All PR's are welcome!

1. `fork` (https://github.com/vigo/leech/fork)
1. Create your `branch` (`git checkout -b my-feature`)
1. `commit` yours (`git commit -am 'add some functionality'`)
1. `push` your `branch` (`git push origin my-feature`)
1. Than create a new **Pull Request**!

This project is intended to be a safe, welcoming space for collaboration, and
contributors are expected to adhere to the [code of conduct][coc].

---

## License

This project is licensed under MIT

[coc]: https://github.com/vigo/leech/blob/main/CODE_OF_CONDUCT.md
