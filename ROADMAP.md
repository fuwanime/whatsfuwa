# WhatsFuwa Roadmap

Dokumen ini mencatat gap produk hasil audit commit `609d53f`. Status berlaku untuk item yang benar-benar memiliki implementasi dan regression test; rencana tidak dihitung sebagai fitur shipped.

## Prinsip

- Tetap ringan untuk satu pengguna dan satu akun; fitur berat harus on-demand atau opt-in.
- Jangan mengunduh riwayat/media secara otomatis tanpa tindakan pengguna.
- Jangan pernah auto-retry operasi yang status penerimaan servernya tidak jelas.
- Sesi, isi pesan, metadata media, dan file unduhan adalah data sensitif.
- Setiap perubahan perilaku memiliki test. Pairing dengan akun nyata diverifikasi terpisah.
- Preview gambar mendukung **Kitty graphics protocol** dan **Sixel**, dengan override manual dan fallback path/eksternal opener. Jangan mengklaim semua terminal mendukung keduanya.

## Milestone 1 — High impact

**Status:** seluruh implementasi minimum dan regression test selesai. Verifikasi dengan akun WhatsApp dan terminal grafis nyata tetap terpisah di bagian bawah dokumen ini.

- [x] **1. Riwayat lama dan pagination**
  - Skenario: membaca pesan sebelum instalasi atau sebelum batas cache saat ini.
  - Implementasi minimum: request `HISTORY_SYNC_ON_DEMAND` untuk 50 pesan sebelum pesan tertua, ingest `events.HistorySync`, deduplikasi, dan tombol/shortcut muat lebih lama.
  - Acceptance: pesan lama masuk berurutan, tidak menduplikasi pesan, tidak menghapus pesan baru, dan error request terlihat.
- [x] **2. Melihat dan mengunduh media masuk**
  - Skenario: membuka foto, dokumen, video, audio, atau stiker yang diterima.
  - Implementasi minimum: simpan metadata protobuf tanpa file biner, unduh on-demand dengan batas ukuran, simpan atomik dengan permission privat, lalu buka file.
  - Preview: dukung Kitty (`kitty`, Ghostty, Konsole/terminal kompatibel) dan Sixel pada terminal yang dikenali/dipilih pengguna.
  - Environment: `WHATSFUWA_IMAGE_PROTOCOL=auto|kitty|sixel|none`; `auto` harus gagal secara aman ke teks jika terminal tidak dikenali.
  - Acceptance: metadata dapat dipakai setelah restart, unduhan tidak memakai nama file dari remote sebagai path, dan kedua encoder memiliki golden test.
- [x] **3. Mengirim lampiran**
  - Implementasi minimum: pilih path melalui TUI, baca file dengan batas ukuran, deteksi MIME, `Client.Upload`, kirim image/video/audio/document, dan simpan hasil dengan status accepted terpisah dari cache error.
  - Acceptance: tidak ada path shell, tipe dokumen fallback benar, file besar ditolak sebelum upload, dan draft path tidak hilang pada kegagalan upload/send.
- [x] **4. Sinkronisasi edit, hapus, dan reaksi**
  - Implementasi minimum: terapkan edit berdasarkan ID target, tombstone revoke/DeleteForMe, serta tambah/hapus reaksi per pengirim.
  - Acceptance: edit tidak membuat duplikat, revoke tidak menghapus audit lokal secara diam-diam, dan reaksi dari dua pengirim tidak saling menimpa.
- [x] **5. Unread, delivered, dan read receipts**
  - Implementasi minimum: field unread/delivery state, handler `events.Receipt`, mark-as-read saat chat aktif, dan tampilan status yang tidak hanya mengandalkan warna.
  - Acceptance: delivered tidak menurunkan status read, read-self menandai incoming terbaca, dan unread dapat dihapus melalui perangkat lain.
- [x] **6. Seleksi pesan untuk reply dan copy**
  - Implementasi minimum: masuk mode seleksi, navigasi pesan, highlight, reply pesan mana pun, copy melalui clipboard terminal, serta download/preview pesan yang sama.
  - Acceptance: seleksi tidak berpindah chat sendiri, reply memakai ID pesan terpilih, dan text biasa tetap masuk composer.
- [x] **7. Outbox offline**
  - Implementasi minimum: antrekan hanya kegagalan preflight yang jelas belum dikirim, persist antrean, tampilkan jumlah, dan retry eksplisit dengan client-generated message ID yang stabil.
  - Acceptance: restart mempertahankan outbox, sukses mengeluarkan item, error cache tidak dianggap send failure, dan tidak ada auto-retry pada error ambigu.

### Batasan implementasi Milestone 1

- `EncReactionMessage` yang memerlukan dekripsi tambahan belum diproses; `ReactionMessage` biasa sudah didukung.
- Lampiran belum masuk outbox; antrean offline saat ini untuk teks.
- Preview decoder tersedia untuk PNG, JPEG, dan GIF frame pertama; format lain memakai aplikasi eksternal.
- Deteksi protokol berbasis environment dan dapat dioverride. Dukungan terminal tetap perlu diverifikasi pada perangkat pengguna.
- Riwayat memakai pesan acuan lokal dan dibatasi 500 pesan per chat agar cache tidak tumbuh tanpa batas.

## Milestone 2 — Medium impact

- [ ] **8. Notifikasi pesan baru** — unread badge, terminal bell, lalu desktop notification opsional.
- [ ] **9. Daftar kontak dan validasi penerima** — `OnWhatsApp` sebelum create dan pencarian kontak.
- [ ] **10. Pencarian isi pesan** — cari `Messages.Text`, tampilkan konteks, lompat ke hasil.
- [ ] **11. Draft per chat yang persisten** — simpan berdasarkan JID dan hapus setelah accepted.
- [ ] **12. Logout dan kontrol privasi lokal** — `logout`, `clear-cache`, mode tanpa cache; pertimbangkan enkripsi at-rest lewat OS keyring tanpa menyamarkan plaintext session DB.
- [ ] **13. Export dan backup** — JSON/Markdown per chat tanpa sesi; backup terenkripsi opsional.
- [ ] **14. CI dan akun uji** — matrix test/vet/build Linux/Windows/macOS serta smoke test WhatsApp opt-in.
- [ ] **15. Release dan rollback** — tag, binary per OS, checksum, dan binary cadangan sebelum update.
- [ ] **16. Composer penuh** — cursor left/right, Home/End, sekitar cursor terlihat, dan mode multiline.
- [ ] **17. Diagnostik dan proxy** — last error, reconnect detail, log tersanitasi, proxy opt-in.

## Milestone 3 — Low impact / fase berikutnya

- [ ] **18. Organisasi chat** — pin, mute, archive, block; mulai lokal lalu sinkronkan.
- [ ] **19. Aksesibilitas dan personalisasi** — layout sempit/plain mode, label nonwarna, tema, keybinding, bahasa, dan format tanggal.
- [ ] **20. Permukaan WhatsApp lanjutan** — group admin, poll, lokasi, kartu kontak, presence/typing, multi-akun, avatar, status, channels, call log, pairing kode nomor, link preview, dan rich formatting. Pilih satu alur per release; jangan membangun semuanya sekaligus.

## Urutan eksekusi

1. Model data dan test migrasi cache.
2. Seleksi pesan, karena reply/download/preview memakai target yang sama.
3. Media masuk dan preview Kitty/Sixel.
4. Lampiran keluar.
5. History on-demand.
6. Edit/delete/reaction dan receipts.
7. Outbox dengan ID stabil dan aturan retry konservatif.
8. Dokumentasi, quality gate, cross-build, dan verifikasi akun uji.

## Verifikasi yang selalu diperlukan sebelum shipped

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `go build ./...` serta cross-build Windows/macOS
- Test encoder/protokol dan migrasi data tanpa akun nyata
- Pairing, history request, upload/download, receipts, dan preview pada terminal nyata sebelum mencentang verifikasi live
