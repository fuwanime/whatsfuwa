# WhatsFuwa

[![Go 1.26+](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)](#instalasi)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**Chat WhatsApp yang ringan, langsung dari terminal.**

WhatsFuwa adalah klien WhatsApp satu akun berbasis Go dan [`whatsmeow`](https://github.com/tulir/whatsmeow). Antarmukanya berupa TUI full-screen dengan daftar chat, panel percakapan, dan kolom pesan—tanpa browser, Chromium, atau Node.js.

> **Status: MVP untuk pemakaian pribadi.** Mendukung chat teks, lampiran, riwayat on-demand, reaksi, read receipt, dan outbox offline. Preview gambar tersedia melalui Kitty graphics protocol dan Sixel pada terminal yang kompatibel.

## Fitur utama

- **Tetap terhubung:** pairing QR, sesi login tersimpan, dan reconnect otomatis.
- **Percakapan lengkap:** kirim/baca teks, reply, edit/hapus/reaction, unread counter, serta delivery/read receipt.
- **Media:** unduh media masuk, kirim lampiran hingga 64 MiB, dan preview gambar dengan Kitty atau Sixel.
- **Riwayat on-demand:** minta 50 pesan lama dari ponsel tanpa mengunduh arsip penuh saat login.
- **Produktif di terminal:** seleksi pesan, reply/copy, keyboard dan mouse, serta pencarian judul/JID.
- **Outbox offline:** antrean teks tersimpan dengan ID stabil dan retry manual yang konservatif.

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
| `Ctrl+G` | Masuk mode seleksi pesan |
| `Ctrl+H` | Minta 50 pesan lama dari ponsel |
| `Ctrl+A` | Lampirkan file |
| `Ctrl+O` | Retry outbox secara manual |
| `Esc` | Batalkan balasan atau tutup form/mode aktif |
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

### Aksi pada pesan terpilih

Tekan `Ctrl+G`, lalu gunakan `↑` / `↓` untuk memilih pesan. Tampilan akan menggulir otomatis agar pesan tetap terlihat.

| Tombol | Aksi |
| --- | --- |
| `r` atau `Enter` | Reply pesan terpilih |
| `c` | Copy teks melalui clipboard terminal (OSC 52) |
| `d` | Unduh media dan tampilkan path lokal |
| `v` | Preview gambar melalui Kitty/Sixel, atau buka media lain secara eksternal |
| `o` | Buka media dengan aplikasi eksternal |
| `Esc` | Keluar dari mode seleksi |

## Riwayat dan media

### Riwayat on-demand

`Ctrl+H` meminta 50 pesan yang berada tepat sebelum pesan tertua di cache. Ponsel harus online dan chat harus sudah memiliki satu pesan acuan. Response diproses sebagai `HISTORY_SYNC_ON_DEMAND`, diurutkan berdasarkan timestamp, dan dideduplikasi berdasarkan message ID. Arsip awal tetap tidak diunduh otomatis saat login.

### Mengunduh dan mengirim media

- `d` pada pesan media mengunduh file secara on-demand ke folder `media/` dalam direktori data.
- `Ctrl+A` membuka form lampiran. Masukkan path file; teks yang sudah ada di composer dipakai sebagai caption bila tipe media mendukungnya.
- Image, video, audio, dan dokumen dibuat dari tipe MIME. Tipe lain dikirim sebagai dokumen.
- Batas unduhan dan lampiran adalah **64 MiB per file**. Audio tidak mendukung caption pada jalur ini.

### Preview Kitty dan Sixel

Preview mendukung PNG, JPEG, dan GIF (frame pertama). Untuk format lain atau terminal yang tidak dikenali, WhatsFuwa membuka file melalui aplikasi eksternal.

| Nilai `WHATSFUWA_IMAGE_PROTOCOL` | Perilaku |
| --- | --- |
| `auto` (default) | Kitty untuk Kitty/Ghostty/Konsole/WezTerm; Sixel untuk TERM yang dikenali seperti `xterm-sixel`, `mlterm`, `foot`, atau `contour` |
| `kitty` | Paksa Kitty graphics protocol |
| `sixel` | Paksa Sixel |
| `none` | Jangan preview di terminal; gunakan aplikasi eksternal |

Deteksi otomatis berbasis environment terminal, bukan negosiasi kemampuan terminal. Pada xterm atau terminal lain yang dikompilasi dengan dukungan khusus, pilih protokol secara eksplisit. Preview dari TUI berjalan melalui perintah `media-preview`; TUI kembali setelah pengguna menekan Enter.

Perintah manual:

```bash
WHATSFUWA_IMAGE_PROTOCOL=kitty whatsfuwa media ./foto.png
WHATSFUWA_IMAGE_PROTOCOL=sixel whatsfuwa media ./foto.png
```

## Outbox offline

Jika pengiriman teks gagal pada preflight karena koneksi belum tersedia, pesan disimpan dalam outbox di `chats.json` bersama reply dan client-generated message ID. Tekan `Ctrl+O` untuk retry setelah online.

Outbox sengaja **tidak melakukan auto-retry untuk error ambigu** seperti timeout. Error tersebut tetap berada dalam antrean untuk review manual agar pesan tidak terduplikasi. Retry memakai `SendRequestExtra.ID` yang sama; pesan yang sudah diterima server dikeluarkan dari antrean. Lampiran belum masuk outbox.

## Perintah

| Perintah | Fungsi |
| --- | --- |
| `whatsfuwa` | Buka aplikasi |
| `whatsfuwa update` | Tarik source dengan `git pull --ff-only`, lalu build ulang |
| `whatsfuwa version` | Tampilkan commit Git tempat binary dibangun |
| `whatsfuwa media FILE` | Preview gambar melalui Kitty atau Sixel |
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
- `chats.json` — cache percakapan, reaksi, delivery state, dan outbox dalam JSON yang tidak dienkripsi oleh WhatsFuwa.
- `media/` — file yang diunduh on-demand dengan nama hash dan permission privat.

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
- Arsip awal tidak diunduh otomatis. Riwayat tambahan hanya diminta melalui `Ctrl+H` dan memerlukan ponsel online.
- Cache menyimpan maksimal 80 chat dan 500 pesan per chat. Teks yang melebihi 1.200 karakter Unicode dipotong dan diberi tanda elipsis.
- Preview gambar bergantung pada dukungan protokol terminal dan decoder PNG/JPEG/GIF. Terminal lain tetap memakai aplikasi eksternal.
- Outbox hanya untuk pesan teks yang gagal sebelum network call. Timeout atau kegagalan dengan status penerimaan tidak jelas tidak dicoba ulang otomatis.
- `ReactionMessage` biasa sudah didukung; `EncReactionMessage` yang memerlukan dekripsi tambahan belum diproses.
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

## Roadmap

Rencana pengembangan berikutnya dan status verifikasinya tercatat di [`ROADMAP.md`](ROADMAP.md).

## Catatan penggunaan

WhatsFuwa bukan produk atau afiliasi WhatsApp. Gunakan untuk komunikasi pribadi secara wajar, bukan spam atau pesan massal, dan patuhi ketentuan WhatsApp. Penggunaan klien pihak ketiga memiliki risiko sesi terputus atau pembatasan akun.

## Lisensi

WhatsFuwa tersedia di bawah [MIT License](LICENSE). `whatsmeow` menggunakan lisensi MPL-2.0; dependency lain mengikuti lisensinya masing-masing.
