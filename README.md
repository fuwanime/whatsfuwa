# WhatsFuwa

**WhatsFuwa** adalah klien WhatsApp berbasis terminal yang ditulis dengan Go dan [`whatsmeow`](https://github.com/tulir/whatsmeow). Antarmukanya berupa **TUI full-screen bergaya chat**, bukan menu CLI bernomor: ada daftar percakapan di sisi kiri, isi chat di sisi kanan, dan kolom pesan di bawah. Pada terminal yang mendukung mouse reporting, elemen seperti chat, `New`, `Search`, dan `Send` dapat diklik.

> **Status:** MVP interaktif. Source sudah menggunakan Go + `whatsmeow`; proses build dan pairing WhatsApp langsung tetap harus diverifikasi di perangkat pengguna.

## Instalasi

### Linux dan macOS (disarankan)

Butuh `git` dan Go 1.26 atau lebih baru. Lalu jalankan:

```bash
curl -fsSL https://raw.githubusercontent.com/fuwanime/whatsfuwa/main/install.sh | bash
```

Installer akan:

1. Mengunduh source ke `~/.whatsfuwa/src` (atau memperbaruinya jika sudah ada).
2. Membangun program ke `~/.local/bin/whatsfuwa`.
3. Menambahkan `~/.local/bin` ke PATH di `~/.bashrc`, `~/.bash_profile` (macOS), atau `~/.zshrc`, jika belum ada.

Setelah itu, buka terminal baru dan jalankan:

```bash
whatsfuwa
```

Opsi yang bisa diatur lewat environment variable:

| Variable | Fungsi | Default |
| --- | --- | --- |
| `WHATSFUWA_HOME` | Folder instalasi | `~/.whatsfuwa` |
| `WHATSFUWA_BIN_DIR` | Folder untuk binary | `~/.local/bin` |
| `WHATSFUWA_REPO` | URL git sumber | repositori resmi |

Contoh: `WHATSFUWA_BIN_DIR=/usr/local/bin bash install.sh` (perlu hak akses ke folder itu).

### Windows atau instalasi manual

Dengan Go 1.26 atau lebih baru, cara ini berlaku di semua sistem operasi:

```bash
go install github.com/fuwanime/whatsfuwa/cmd/whatsfuwa@latest
```

Binary akan berada di folder `GOBIN` (atau `$(go env GOPATH)/bin`). Pastikan folder itu ada di PATH. Perintah `whatsfuwa update` memerlukan source checkout di `~/.whatsfuwa/src`; untuk itu, pakai installer di atas atau jalankan `git clone` manual ke lokasi tersebut.

## Tampilan dan interaksi

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ ✦ WhatsFuwa   ● TERHUBUNG                       [ + New ] [ / Search ] [ Exit ]│
│ CHATS  12                          CONVERSATION  FuwaStream Community         │
├──────────────────────────────┬───────────────────────────────────────────────┤
│ ▸ FuwaStream Community        │  FuwaStream Community                         │
│   Ada update baru?            │                                               │
│   Teman                       │  Admin  ·  18:31                              │
│   Halo bro...                 │  Jangan lupa cek update terbarunya.           │
│   Keluarga                    │                                               │
│   Jangan lupa makan           │  Kamu  ·  18:32                               │
│                               │  Siap, makasih!                               │
│                               │                                               │
├──────────────────────────────┴───────────────────────────────────────────────┤
│  > Tulis pesan...                                              [ Send ]       │
│  Enter: kirim · Ctrl+F: cari · Ctrl+N: chat baru · ↑↓: pilih chat · Ctrl+C: keluar     │
└──────────────────────────────────────────────────────────────────────────────┘
```

*Gambaran layout; isi chat di perangkat akan berasal dari pesan yang diterima atau dikirim melalui akun WhatsApp tertaut.*

### Mouse

- Klik item percakapan di panel kiri untuk membuka chat.
- Klik **`+ New`** untuk membuat percakapan baru.
- Klik **`Search`** untuk mencari nama atau JID chat.
- Klik kolom pesan untuk fokus mengetik.
- Klik **`Send`** untuk mengirim pesan.
- Scroll wheel di daftar chat atau panel percakapan untuk menggulir.
- Klik **`Exit`** untuk menutup WhatsFuwa.

Mouse reporting perlu didukung oleh terminal yang dipakai. Coba Windows Terminal, GNOME Terminal, Konsole, atau terminal modern lain. Pada terminal yang tidak meneruskan event mouse, navigasi keyboard tetap tersedia.

### Keyboard

| Tombol | Fungsi |
| --- | --- |
| `↑` / `↓` | Pindah pilihan chat saat kolom pesan kosong |
| `Enter` | Kirim pesan |
| `Ctrl+F` | Buka pencarian chat |
| `Ctrl+N` | Buat chat baru |
| `Ctrl+R` | Balas pesan masuk terakhir di chat yang dipilih |
| `Esc` | Batalkan balasan, tutup mode pencarian, atau tutup form chat baru |
| `PgUp` / `PgDn` | Gulir riwayat pesan |
| `Tab` | Pindah fokus antara daftar chat dan kolom pesan |
| `Ctrl+C` | Keluar |

## Kenapa Go + whatsmeow?

- Go menghasilkan executable native dan tidak membutuhkan Node.js runtime.
- `whatsmeow` adalah library Go komunitas untuk protokol WhatsApp multi-device.
- SQLite pure-Go menyimpan sesi tanpa perlu memasang C compiler.
- Bubble Tea menangani input keyboard dan mouse di terminal.
- Tidak ada Chromium, Selenium, atau browser penuh.

### Kenapa bukan HyperMeow dulu?

HyperMeow menarik untuk kebutuhan yang lebih luas, termasuk pengelolaan identitas LID dan banyak sesi. WhatsFuwa saat ini ditujukan untuk pemakaian pribadi pada satu akun, jadi versi awal memakai upstream `whatsmeow` agar cakupan lebih sederhana. Keduanya tetap bergantung pada protokol WhatsApp yang dapat berubah.

Memakai Go atau library berbeda **tidak menjamin** semua error dekripsi seperti `Bad MAC` otomatis hilang.

## Fitur saat ini

- Pairing perangkat tertaut menggunakan QR di terminal.
- Memakai ulang sesi login yang tersimpan.
- Reconnect otomatis.
- TUI full-screen dengan daftar chat dan panel percakapan.
- Tombol mouse untuk memilih chat, membuat chat, mencari, dan mengirim pesan.
- Navigasi keyboard sebagai alternatif mouse.
- Mengirim dan membaca pesan teks.
- Nama grup ditampilkan otomatis (diambil sekali per grup).
- Pesan media ditampilkan sebagai label (mis. `📷 Foto`, `📄 Dokumen`) beserta caption bila ada. Media tidak diunduh.
- Membalas pesan dengan kutipan (`Ctrl+R`), dan melihat kutipan pada balasan yang diterima.
- Cache lokal dibatasi hingga 80 chat dan 30 pesan per chat.
- Tidak mengunduh arsip riwayat chat penuh secara otomatis.

Daftar chat mulai terisi setelah ada pesan baru yang diterima atau pesan yang dikirim. Riwayat lama tidak disedot otomatis.

## Persyaratan

- Go **1.26 atau lebih baru**. `go.mod` saat ini menyarankan toolchain Go 1.27.4; dengan `GOTOOLCHAIN=auto`, Go dapat mencoba mengunduh toolchain yang diminta.
- Koneksi internet.
- WhatsApp aktif di ponsel untuk proses pairing.
- Terminal yang mendukung warna dan input interaktif. Mouse memerlukan dukungan mouse reporting dari terminal.

## Menjalankan dari source

```bash
git clone https://github.com/fuwanime/whatsfuwa.git
cd WhatsFuwa
go mod tidy
go run ./cmd/whatsfuwa
```

Atau build executable:

```bash
go build -trimpath -ldflags="-s -w" -o whatsfuwa ./cmd/whatsfuwa
./whatsfuwa
```

Di Windows PowerShell:

```powershell
go build -trimpath -ldflags="-s -w" -o whatsfuwa.exe ./cmd/whatsfuwa
.\whatsfuwa.exe
```

`go mod tidy` akan mengunduh dependency dan membuat/memperbarui `go.sum` pada checkout lokal.

## Login pertama kali

1. Jalankan WhatsFuwa.
2. Pindai QR yang muncul di terminal menggunakan WhatsApp di ponsel.
3. Buka **Perangkat tertaut → Tautkan perangkat**.
4. Setelah pairing sukses, TUI WhatsFuwa akan terbuka.

Pada peluncuran berikutnya, WhatsFuwa akan memakai sesi yang tersimpan dan tidak meminta QR lagi selama sesi masih tertaut.

## Perintah

| Perintah | Fungsi |
| --- | --- |
| `whatsfuwa` | Buka aplikasi |
| `whatsfuwa update` | Jalankan `git pull --ff-only` di folder source, lalu bangun ulang program. Folder source dicari dari `WHATSFUWA_SRC`, `~/.whatsfuwa/src`, folder saat ini, atau folder tempat program berada. Di Windows, build baru disimpan sebagai `whatsfuwa.exe.new` dan harus diganti manual setelah aplikasi ditutup. |
| `whatsfuwa version` | Tampilkan commit git tempat program dibangun |
| `whatsfuwa help` | Tampilkan daftar perintah |

## Data lokal dan privasi

WhatsFuwa menyimpan data di folder `data/`:

- `session.db` — database sesi perangkat tertaut, termasuk material autentikasi. **Jangan bagikan file ini.**
- `chats.json` — cache percakapan dan cuplikan pesan dalam bentuk JSON; teks cache tidak dienkripsi oleh WhatsFuwa.

Folder `data/` diabaikan oleh Git. Jangan commit, upload, atau kirim isi folder ini ke orang lain. Untuk logout, gunakan fitur logout melalui WhatsApp di ponsel atau hapus sesi lokal setelah memastikan perangkat sudah tidak tertaut.

## Catatan Bad MAC

Error `Bad MAC` berkaitan dengan kegagalan autentikasi/dekripsi atau state sesi Signal yang tidak sinkron. Berpindah dari Baileys ke Go/`whatsmeow` **bukan jaminan** error sejenis mustahil terjadi. WhatsFuwa mengaktifkan opsi permintaan ulang pesan dari perangkat utama sebagai salah satu mekanisme pemulihan. Jika masalah berulang, simpan log error, pastikan hanya satu instance WhatsFuwa memakai database sesi, dan coba relink perangkat sebagai langkah pemulihan terakhir.

## Catatan penggunaan

WhatsFuwa memakai library komunitas nonresmi dan bukan produk atau afiliasi WhatsApp. Protokol dapat berubah sewaktu-waktu; penggunaan klien pihak ketiga memiliki risiko sesi terputus atau pembatasan akun. Gunakan secara wajar untuk komunikasi pribadi, jangan untuk spam atau pesan massal, dan patuhi ketentuan WhatsApp.

## Lisensi

Kode WhatsFuwa disediakan sebagaimana adanya. `whatsmeow` menggunakan lisensi MPL-2.0; dependency lain mengikuti lisensinya masing-masing.
