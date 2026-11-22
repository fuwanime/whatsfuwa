# WhatsFuwa

[![Go 1.26+](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)](#instalasi)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**Chat WhatsApp yang ringan, langsung dari terminal.**

WhatsFuwa adalah klien WhatsApp satu akun berbasis Go dan [`whatsmeow`](https://github.com/tulir/whatsmeow). Antarmukanya berupa TUI full-screen dengan daftar chat, panel percakapan, dan kolom pesan—tanpa browser, Chromium, atau Node.js.

> **Status: MVP untuk pemakaian pribadi.** Fokus saat ini adalah chat teks dan balasan. Media ditampilkan sebagai label, bukan diunduh.

## Fitur utama

- **Tetap terhubung:** pairing QR, sesi login tersimpan, dan reconnect otomatis.
- **Chat yang familiar:** kirim/baca pesan teks, balas dengan kutipan, dan lihat nama grup.
- **Keyboard dan mouse:** pilih chat, cari percakapan, buat chat baru, dan scroll riwayat.
- **Ringan secara default:** tidak mengunduh arsip chat penuh atau file media secara otomatis.
- **Cache terbatas:** menyimpan maksimal 80 chat dan 30 pesan per chat di perangkat.

## Instalasi

### Prasyarat

- Go **1.26 atau lebih baru**. Dengan `GOTOOLCHAIN=auto`, Go dapat mengunduh toolchain yang diminta oleh `go.mod`.
- `git` untuk installer dan perintah `whatsfuwa update`.
- Koneksi internet dan WhatsApp aktif di ponsel.
- Terminal interaktif; fitur mouse memerlukan terminal yang mendukung mouse reporting.

### Linux dan macOS — installer

```bash
curl -fsSL https://raw.githubusercontent.com/fuwanime/whatsfuwa/main/install.sh | bash
```

Installer akan:

1. Mengunduh source ke `~/.whatsfuwa/src`, atau memperbaruinya jika sudah ada.
2. Membangun binary ke `~/.local/bin/whatsfuwa`.
3. Menambahkan folder binary ke `PATH` pada konfigurasi Bash atau Zsh bila diperlukan.

Buka terminal baru, lalu jalankan:

```bash
whatsfuwa
```

Pengguna Fish dapat menambahkan folder binary dengan:

```bash
fish_add_path ~/.local/bin
```

Opsi installer melalui environment variable:

| Variable | Fungsi | Default |
| --- | --- | --- |
| `WHATSFUWA_HOME` | Folder instalasi source | `~/.whatsfuwa` |
| `WHATSFUWA_BIN_DIR` | Folder tujuan binary | `~/.local/bin` |
| `WHATSFUWA_REPO` | URL Git sumber | Repository resmi |

Contoh instalasi ke folder binary lain:

```bash
WHATSFUWA_BIN_DIR=/usr/local/bin bash install.sh
```

### Windows atau instalasi manual

Cara ini berlaku di semua sistem operasi yang didukung Go:

```bash
go install github.com/fuwanime/whatsfuwa/cmd/whatsfuwa@latest
```

Binary tersimpan di `GOBIN`, atau `$(go env GOPATH)/bin` jika `GOBIN` tidak diatur. Pastikan folder tersebut ada di `PATH`.

> `whatsfuwa update` memerlukan source checkout. Instalasi dengan `go install` tetap dapat menjalankan aplikasi, tetapi pembaruan otomatis memerlukan checkout di `~/.whatsfuwa/src` atau variable `WHATSFUWA_SRC`.

## Login pertama kali

1. Jalankan `whatsfuwa`.
2. Di ponsel, buka **WhatsApp → Perangkat tertaut → Tautkan perangkat**.
3. Pindai QR yang muncul di terminal.
4. Setelah pairing berhasil, TUI WhatsFuwa akan terbuka.

Peluncuran berikutnya memakai sesi tersimpan selama perangkat masih tertaut. Jika QR kedaluwarsa sebelum dipindai, jalankan kembali aplikasi untuk memulai pairing baru.

## Tampilan

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ ✦ WhatsFuwa   ● TERHUBUNG                    [ + New ] [ / Search ] [ × Exit ]│
│ CHATS  12                          CONVERSATION  FuwaStream Community         │
├──────────────────────────────┬───────────────────────────────────────────────┤
│ ▸ FuwaStream Community        │  Admin  ·  18:31                              │
│   Ada update baru?            │  Jangan lupa cek update terbarunya.           │
│   Teman                       │                                               │
│   Halo bro...                 │  Kamu  ·  18:32                               │
│   Keluarga                    │  Siap, makasih!                               │
│   Jangan lupa makan           │                                               │
├──────────────────────────────┴───────────────────────────────────────────────┤
│  > Tulis pesan...                                           [ Send ]          │
│  Enter: kirim · Ctrl+F: cari · Ctrl+N: chat baru · PgUp/PgDn: riwayat         │
└──────────────────────────────────────────────────────────────────────────────┘
```

*Ilustrasi layout. Isi chat berasal dari pesan yang diterima atau dikirim melalui akun tertaut.*

## Kontrol

### Keyboard

| Tombol | Fungsi |
| --- | --- |
| `↑` / `↓` | Pindah pilihan chat saat kolom pesan kosong atau daftar chat aktif |
| `Enter` | Kirim pesan atau konfirmasi form aktif |
| `Ctrl+F` | Cari nama atau JID chat |
| `Ctrl+N` | Buat chat baru |
| `Ctrl+R` | Balas pesan masuk terbaru di chat yang dipilih |
| `Esc` | Batalkan balasan atau tutup form aktif |
| `PgUp` / `PgDn` | Scroll riwayat pesan |
| `Tab` | Pindah fokus antara daftar chat dan kolom pesan |
| `Ctrl+C` | Keluar |

### Mouse

| Target | Aksi |
| --- | --- |
| Item chat | Buka percakapan |
| `+ New` | Buat percakapan baru |
| `Search` | Cari percakapan |
| Kolom pesan | Fokus mengetik |
| `Send` | Kirim pesan |
| `× Exit` | Tutup aplikasi |
| Scroll wheel | Scroll daftar chat atau riwayat |

Untuk chat baru, masukkan nomor dalam format internasional, misalnya `628123456789`. Windows Terminal, GNOME Terminal, Konsole, dan terminal modern lain umumnya mendukung mouse reporting; keyboard tetap tersedia sebagai fallback.

## Perintah

| Perintah | Fungsi |
| --- | --- |
| `whatsfuwa` | Buka aplikasi |
| `whatsfuwa update` | Tarik source dengan `git pull --ff-only`, lalu build ulang |
| `whatsfuwa version` | Tampilkan commit Git tempat binary dibangun |
| `whatsfuwa help` | Tampilkan bantuan |

Source untuk update dicari dari `WHATSFUWA_SRC`, `~/.whatsfuwa/src`, folder saat ini, lalu folder binary. Di Windows, binary baru disimpan sebagai `whatsfuwa.exe.new`; tutup aplikasi lalu ganti binary lama secara manual.

## Data lokal dan privasi

Data disimpan di folder `whatsfuwa` dalam direktori konfigurasi pengguna, bukan di folder tempat perintah dijalankan:

| Sistem operasi | Lokasi default |
| --- | --- |
| Linux | `~/.config/whatsfuwa/`, atau `$XDG_CONFIG_HOME/whatsfuwa/` |
| macOS | `~/Library/Application Support/whatsfuwa/` |
| Windows | `%AppData%\whatsfuwa\` |

Gunakan `WHATSFUWA_DATA_DIR` untuk instalasi portabel atau lokasi khusus:

```bash
WHATSFUWA_DATA_DIR="$HOME/.local/share/whatsfuwa" whatsfuwa
```

Isi folder data:

- `session.db` — sesi perangkat tertaut dan material autentikasi.
- `chats.json` — cache percakapan dan cuplikan pesan dalam JSON yang tidak dienkripsi oleh WhatsFuwa.

> **Seluruh isi folder data bersifat sensitif.** Jangan bagikan, upload, atau commit folder ini, termasuk file tambahan SQLite seperti WAL. Folder `./data` pada source tetap diabaikan Git untuk kompatibilitas lama; folder override lain tidak otomatis diabaikan.

### Migrasi dari versi sebelumnya

Versi lama menyimpan data di `./data`, relatif terhadap folder tempat aplikasi dijalankan. Tutup WhatsFuwa, lalu pindahkan seluruh isi folder tersebut ke lokasi data baru agar sesi tidak hilang.

Contoh untuk Linux dengan konfigurasi default:

```bash
mkdir -p "${XDG_CONFIG_HOME:-$HOME/.config}/whatsfuwa"
mv ./data/. "${XDG_CONFIG_HOME:-$HOME/.config}/whatsfuwa/"
```

Alternatifnya, arahkan `WHATSFUWA_DATA_DIR` ke folder data lama.

## Batasan saat ini

- Dibuat untuk **satu akun dan satu instance** pada folder sesi yang sama. Jangan menjalankan dua instance pada database sesi yang sama.
- Pengiriman berfokus pada teks dan kutipan. Foto, video, dokumen, audio, dan stiker hanya muncul sebagai label beserta caption yang tersedia; file tidak diunduh.
- Riwayat lama tidak diunduh otomatis. Daftar chat terisi dari cache lokal serta pesan baru yang diterima atau dikirim.
- Cache menyimpan maksimal 80 chat dan 30 pesan per chat. Teks yang melebihi 1.200 karakter Unicode dipotong dan diberi tanda elipsis.
- WhatsFuwa memakai library komunitas nonresmi. Perubahan protokol WhatsApp dapat memutus sesi atau mengubah perilaku aplikasi.

## Troubleshooting

| Masalah | Langkah yang disarankan |
| --- | --- |
| QR kedaluwarsa | Jalankan kembali `whatsfuwa`, lalu pindai QR baru |
| Chat lama tidak muncul | Ini perilaku default; WhatsFuwa tidak mengunduh arsip chat penuh |
| Error `Bad MAC` berulang | Pastikan hanya satu instance memakai sesi, simpan log, lalu relink perangkat sebagai langkah terakhir |
| Sesi terputus setelah update WhatsApp | Coba restart aplikasi dan `whatsfuwa update`; relink jika sesi memang sudah tidak valid |
| Binary Windows tidak berubah setelah update | Tutup aplikasi, lalu ganti `whatsfuwa.exe` dengan `whatsfuwa.exe.new` |

`Bad MAC` berkaitan dengan kegagalan autentikasi/dekripsi atau state sesi Signal yang tidak sinkron. Penggunaan Go dan `whatsmeow` tidak menjamin error tersebut tidak akan pernah terjadi. WhatsFuwa mengaktifkan permintaan ulang pesan dari ponsel sebagai salah satu mekanisme pemulihan.

## Menjalankan dari source

```bash
git clone https://github.com/fuwanime/whatsfuwa.git
cd whatsfuwa
go test ./...
go run ./cmd/whatsfuwa
```

Build binary di Linux atau macOS:

```bash
go build -trimpath -ldflags="-s -w" -o whatsfuwa ./cmd/whatsfuwa
./whatsfuwa
```

Di Windows PowerShell:

```powershell
go build -trimpath -ldflags="-s -w" -o whatsfuwa.exe ./cmd/whatsfuwa
.\whatsfuwa.exe
```

Pemeriksaan tambahan untuk pengembangan:

```bash
go test -race ./...
go vet ./...
```

## Catatan arsitektur

- Go menghasilkan executable native dan tidak memerlukan Node.js.
- SQLite pure-Go menyimpan sesi tanpa memerlukan C compiler.
- Bubble Tea menangani input keyboard dan mouse di terminal.
- `whatsmeow` dipilih untuk scope satu akun yang lebih sederhana; HyperMeow tidak digunakan pada MVP ini.
- Tidak ada Chromium, Selenium, atau browser penuh.

## Catatan penggunaan

WhatsFuwa bukan produk atau afiliasi WhatsApp. Gunakan untuk komunikasi pribadi secara wajar, bukan spam atau pesan massal, dan patuhi ketentuan WhatsApp. Penggunaan klien pihak ketiga memiliki risiko sesi terputus atau pembatasan akun.

## Lisensi

WhatsFuwa tersedia di bawah [MIT License](LICENSE). `whatsmeow` menggunakan lisensi MPL-2.0; dependency lain mengikuti lisensinya masing-masing.
