# todoapp

Aplikasi terminal (TUI) all-in-one: **Jurnal | To-do | Chat**, dengan server Go
tersinkron (MariaDB), login multi-user, dan notifikasi.

```
[TUI client] <-- HTTPS REST --> [Server Go] <--> [MariaDB]
[TUI client] <-- WebSocket ---> [Server Go]
```

> Status: **Selesai (Fase 1–6)** — Aplikasi terminal (TUI) all-in-one lengkap: Jurnal, To-do, Chat WebSocket real-time, desktop notifications, theming, offline resilience, dan release builds.

## Struktur

```
cmd/server            entrypoint server (+ subcommand migrate)
cmd/client            entrypoint TUI client (Bubble Tea)
internal/models       tipe wire bersama client/server
internal/server/      config, db, auth, store, middleware, handler, reminder (Scheduler), ws (Hub), httpx
internal/client/      config, api client, ws (client), notify (beeep/bell), tui (views, styles, themes)
migrations/           file SQL goose (di-embed ke binary)
deploy/mariadb-init/  init script container (membuat todoapp_test)
bin/                  hasil kompilasi binary (server, client)
```

## Menjalankan (dev)

### Dengan Docker
```sh
docker compose up --build
curl http://127.0.0.1:8080/healthz
```

### Tanpa Docker (MariaDB lokal)
Buat database dan user:
```sql
CREATE DATABASE todoapp CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE DATABASE todoapp_test CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'todoapp'@'localhost' IDENTIFIED BY 'todoapp';
GRANT ALL PRIVILEGES ON todoapp.* TO 'todoapp'@'localhost';
GRANT ALL PRIVILEGES ON todoapp_test.* TO 'todoapp'@'localhost';
```
Lalu:
```sh
cp .env.example .env      # sesuaikan DB_DSN
go run ./cmd/server       # migrasi otomatis saat start
```

### Migrasi manual
```sh
go run ./cmd/server migrate status
go run ./cmd/server migrate up
go run ./cmd/server migrate down    # rollback satu versi
```
Tambah migrasi baru: buat file `migrations/0000N_nama.sql` dengan blok
`-- +goose Up` dan `-- +goose Down`.

## Konfigurasi server
Lihat [.env.example](.env.example). Variabel env asli selalu mengalahkan isi `.env`.

| Var | Default | Keterangan |
|---|---|---|
| `APP_ADDR` | `:8080` | alamat listen |
| `DB_DSN` | — (wajib) | DSN go-sql-driver/mysql |
| `AUTO_MIGRATE` | `true` | migrasi saat start |
| `SESSION_TTL` | `720h` | masa berlaku sesi (sliding) |
| `SESSION_TOUCH_INTERVAL` | `1h` | interval minimum perpanjangan sesi |
| `TRUST_PROXY` | `false` | percayai `X-Forwarded-For` (hanya di belakang proxy) |
| `LOGIN_RATE_PER_MINUTE` / `LOGIN_BURST` | `5` / `5` | rate limit login/register |
| `MAX_BODY_BYTES` | `1048576` | batas ukuran body |
| `LOG_LEVEL` | `info` | debug/info/warn/error |

## API (Fase 1, 2, 3, 4)

| Method | Path | Auth | Body → Respons |
|---|---|---|---|
| GET | `/healthz` | – | `{"status":"ok"}` |
| POST | `/api/v1/auth/register` | – | `{username,password,timezone?}` → `201 {user}` |
| POST | `/api/v1/auth/login` | – | `{username,password,timezone?}` → `200 {token,expires_at,user}` |
| POST | `/api/v1/auth/logout` | Bearer | → `204` |
| GET | `/api/v1/me` | Bearer | → `200 {user}` |
| GET | `/api/v1/todos` | Bearer | `?date=YYYY-MM-DD` atau `?since=...` → `200 {todos,server_time}` |
| POST | `/api/v1/todos` | Bearer | `{date?,title,description?,priority?,remind_at?,repeat_rule?}` → `201 {todo}` |
| POST | `/api/v1/todos/batch` | Bearer | `{title,description?,start_date?,end_date?,weekdays_only?,priority?,remind_time?} / {items:[...]}` → `201 {created_count,todos}` |
| GET | `/api/v1/todos/{id}` | Bearer | → `200 {todo}` |
| PATCH | `/api/v1/todos/{id}` | Bearer | `{title?,description?,done?,priority?,date?,remind_at?,repeat_rule?}` → `200 {todo}` |
| DELETE | `/api/v1/todos/{id}` | Bearer | → `204` (soft delete) |
| POST | `/api/v1/todos/rollover` | Bearer | `{from_date,to_date}` → `200 {moved_count}` |
| GET | `/api/v1/journal` | Bearer | `?date=YYYY-MM-DD` atau `?since=...` → `200 {entry}` / `{entries,server_time}` |
| PUT | `/api/v1/journal` | Bearer | `{date,title?,body,tags?}` → `200 {entry}` |
| DELETE | `/api/v1/journal` | Bearer | `?date=YYYY-MM-DD` → `204` (soft delete) |
| GET | `/api/v1/journal/search` | Bearer | `?q=...` → `200 {query,matches:[{id,date,title,snippet,tags}]}` |
| GET | `/api/v1/rooms` | Bearer | → `200 {rooms:[{id,name,is_dm,unread_count}]}` |
| POST | `/api/v1/rooms` | Bearer | `{name}` → `201 {room}` |
| POST | `/api/v1/rooms/{id}/join` | Bearer | → `204` |
| POST | `/api/v1/rooms/dm` | Bearer | `{username}` → `200 {room}` (idempotent key) |
| GET | `/api/v1/rooms/{id}/messages` | Bearer | `?before_id=...&limit=50` → `200 {messages,has_more}` |
| POST | `/api/v1/rooms/{id}/messages` | Bearer | `{body}` → `201 {message}` (broadcast via WS) |
| POST | `/api/v1/rooms/{id}/read` | Bearer | `{last_read_message_id}` → `204` |
| GET | `/ws?token=...` | Query Token | Handshake WebSocket (event broadcast, presence, typing) |

Error selalu berbentuk `{"error":{"code":"...","message":"...","field":"..."}}`.

## Menjalankan Klien TUI

Jalankan client Bubble Tea:
```sh
go run ./cmd/client
# atau hubungkan ke server remote:
go run ./cmd/client -server http://127.0.0.1:8080
```

### Navigasi & Shortcut TUI
- **Navigasi Tab**: `1` (Jurnal), `2` (To-do), `3` (Chat), atau `Tab`.
- **Navigasi Tanggal**: `h` / `[` / `←` (hari sebelumnya), `l` / `]` / `→` (hari berikutnya), `t` (kembali ke hari ini).
- **Operasi Jurnal (Multi-Catatan Per Tanggal & Dual-Panel)**:
  - **Dukungan Multi-Catatan**: Setiap tanggal dapat memiliki lebih dari satu catatan jurnal tersendiri.
  - **Tampilan Dual-Panel**:
    - Kolom kiri: Daftar semua catatan untuk tanggal yang dipilih (`CATATAN (N)`), judul atau preview cuplikan teks.
    - Kolom kanan: Viewport preview membaca isi lengkap catatan yang sedang dipilih (Judul `📌`, Tags `🏷️`, Waktu Tersimpan `🕒`, dan isi Markdown).
  - `j` / `k` atau `↑` / `↓`: Pindah dan pilih catatan dalam tanggal aktif (viewport kanan otomatis berganti sesuai pilihan).
  - `a` / `n`: **Tambah catatan baru** pada tanggal aktif.
  - `Enter` / `i`: Edit catatan yang sedang dipilih di terminal (judul, textarea markdown, dan tags). Pindah field: `Tab`, Simpan: `Ctrl+S`, Batal: `Esc`.
  - `e`: Buka catatan yang sedang dipilih di editor eksternal `$EDITOR` (vim/nano/notepad). Judul di baris pertama `# Judul Catatan` otomatis tersinkron saat disimpan.
  - `d`: Hapus catatan yang sedang dipilih (konfirmasi `y`).
  - `/`: Buka modal pencarian full-text jurnal MariaDB. Tekan `Enter` pada hasil pencarian untuk langsung lompat ke tanggal catatan tersebut.
- **Operasi To-do (Detail View & Batch)**:
  - `j` / `k` atau `↑` / `↓`: Pindah kursor task.
  - `Enter` / `v` / `o`: **Detail View ("View Masuk")** — Buka tampilan detail lengkap task:
    - Badge status jelas `[✓ SELESAI]` / `[⏳ BELUM SELESAI]` & badge prioritas `[P1/P2/P3]`.
    - Judul lengkap, tanggal jadwal, pengulangan (`↻ daily/weekly`), dan jam pengingat (`⏰ HH:MM`).
    - Catatan / deskripsi multiline yang rapi dan mudah dibaca.
    - Timestamp pembuatan dan pembaruan terakhir.
    - Di dalam Detail View: `Space` toggle status selesai, `e` edit, `d` hapus, `j`/`k` jelajahi to-do lain, `Esc`/`Enter` kembali.
  - `Space`: Toggle selesai / buka task langsung dari daftar utama.
  - `a`: Tambah to-do baru (judul, catatan/deskripsi tambahan, prioritas 1-3, pengulangan).
  - `b` / `B`: **Batch Creation Modal** — generate to-do berulang otomatis (mis. **Senin sampai Jumat** untuk posting konten, atau rentang tanggal kustom) dilengkapi catatan/deskripsi dan jam pengingat (`HH:MM`).
  - `e`: Edit to-do yang dipilih (judul, catatan/deskripsi, prioritas, repeat rule).
  - `d`: Hapus to-do (konfirmasi `y`).
  - `m`: Rollover task yang belum selesai dari hari ini ke besok (`y`).
  - `r`: Refresh data dari server.
  - Catatan/deskripsi to-do otomatis ditampilkan dengan penanda `↳ 📝 <catatan>` pada task yang dipilih di daftar utama.
- **Operasi Chat (Fase 4, Redesign & Scroll)**:
  - **Tampilan Balon Chat & Penanda Tanggal**:
    - Pesan pengguna (`Anda`) di sebelah kanan, pesan rekan di sebelah kiri.
    - Pembatas tanggal otomatis di tengah percakapan (mis. `─── Hari ini • 20 Oktober 2026 ───` atau `─── Kemarin • 19 Oktober 2026 ───`).
    - Penanda waktu/tanggal jelas pada header balon (`15:04` jika hari ini, atau `19 Okt, 15:04` jika hari sebelumnya).
  - **Navigasi 3 Area Fokus**:
    - **Sidebar Room**: `j`/`k` atau `↑`/`↓` pindah room, `l` atau `→` masuk ke Mode Baca Chat, `Enter`/`i` langsung ketik pesan, `n` buat channel, `u` kirim DM.
    - **Mode Baca Chat (Scroll Viewport)**: Tekan `l`/`→` dari sidebar atau `Esc` dari input teks:
      - `k` / `↑`: Scroll ke atas (pesan lama/sebelumnya). Jika mencapai paling atas, otomatis memuat riwayat pesan lebih lama (*infinite scroll*).
      - `j` / `↓`: Scroll ke bawah (pesan terbaru).
      - `PgUp` / `b`: Scroll naik 1 halaman penuh.
      - `PgDn` / `f` / `Space`: Scroll turun 1 halaman penuh.
      - `u` / `d` (atau `Ctrl+U`/`Ctrl+D`): Scroll setengah halaman naik/turun.
      - `g` / `Home`: Lompat langsung ke pesan paling atas/lama.
      - `G` / `End`: Lompat langsung ke pesan paling bawah/terbaru.
      - `h` / `←` / `Esc`: Kembali ke sidebar room.
      - `i` / `Enter`: Masuk ke mode ketik pesan.
    - **Mode Ketik Pesan (Input)**:
      - `Enter`: Kirim pesan (langsung broadcast via WebSocket).
      - `PgUp` / `PgDn` (atau `Alt+↑`/`Alt+↓`): Tetap bisa scroll riwayat pesan di atas tanpa kehilangan ketikan.
      - `↑` (saat input masih kosong): Otomatis masuk ke Mode Baca dan scroll ke atas.
      - `Esc`: Pindah ke Mode Baca Chat.
      - `Ctrl+K`: Toggle kembali ke sidebar room.
  - `r`: Refresh daftar channel dan pesan.
- **Command Palette**: Tekan `:` atau `Ctrl+P`:
  - `:batch`: Buka modal batch to-do creation (Senin-Jumat / range posting).
  - `:theme <nama>`: Ganti tema warna (mendukung 13 tema + alias seperti `:theme pink`, `:theme cerah`, dll.).
  - `:notify [on|off]`: Toggle notifikasi desktop.
  - `:today`: Lompat ke hari ini pada tab aktif.
  - `:search <kata>`: Buka modal pencarian full-text jurnal.
  - `:add <judul>`: Tambah task to-do cepat.
  - `:refresh` / `:r`: Muat ulang data server.
  - `:logout`: Keluar dari sesi akun saat ini.
  - `:quit` / `:q`: Tutup aplikasi.
- **Pilihan Tema Warna**:
  - **Tema Pink / Pastel**:
    - `sakura` (alias `:theme pink`): Nuansa bunga sakura Jepang yang anggun dengan aksen soft pink & plum.
    - `rose-pine` (alias `:theme rose`): Palet estetik dreamy rosé pink, gold, dan foam teal.
    - `bubblegum`: Palet ceria berenergi tinggi dengan warna hot pink dan neon cyan.
  - **Tema Cerah / Terang (*Light Mode*)**:
    - `latte` (alias `:theme light`, `:theme cerah`, `:theme putih`): Catppuccin Latte modern, bersih, dan nyaman di mata untuk ruangan terang.
    - `solarized-light` (alias `:theme solarized`): Kertas hangat krem legendaris Ethan Schoonover dengan kontras tipografi buku.
    - `nord-light`: Tema terang kutub salju arctic frost yang elegan dan jernih.
  - **Tema Gelap / Cyber (*Dark Mode*)**:
    - `catppuccin`: Tema gelap modern bernuansa ungu pastel dan cyan (default).
    - `nord`: Palet kutub sejuk dengan aksen biru es dan aurora green.
    - `gruvbox`: Palet retro hangat berbasis warna amber, oranye, dan earthy green.
    - `dracula`: Tema kontras tinggi bernuansa gothic dark vampire.
    - `tokyonight`: Nuansa lampu neon malam kota Tokyo dengan biru elektrik dan lavender.
    - `monokai`: Palet developer berenergi tinggi dengan magenta neon dan lime green.
    - `synthwave`: Nuansa retro 80s synthwave cyberpunk dengan laser pink dan grid purple.
  - Tema tersimpan otomatis di `~/.config/todoapp/config.json` dan langsung memperbarui tampilan chat, kursor, dan tab seketika.
- **Indikator Koneksi & Status Online**:
  - `● Online`: Terhubung langsung ke server via WebSocket dan REST.
  - `○ Offline`: Koneksi terputus; sistem melakukan auto-reconnect dengan exponential backoff.
- **Notifikasi & Pengingat (Fase 5)**:
  - Pengingat task otomatis dikirim dari server via WebSocket saat `remind_at` tiba.
  - Memunculkan notifikasi toast desktop sistem (Windows/macOS/Linux via `beeep`) beserta audio alert dan bell terminal (`\a`).
  - Pesan chat baru memicu notifikasi desktop jika sedang membuka tab lain atau room berbeda.
- **Fitur Salin & Tempel (Copy & Paste / Clipboard)**:
  - **Salin (Copy / Yank)**:
    - Tekan `c` atau `y` di Tab Jurnal untuk menyalin isi catatan terpilih ke clipboard sistem.
    - Tekan `c` atau `y` di Tab To-Do (tampilan daftar atau detail) untuk menyalin task ke clipboard.
    - Tekan `c` atau `y` di Tab Chat (mode baca) untuk menyalin pesan terakhir ke clipboard.
    - Perintah `:copy` (alias `:c` atau `:yank`) di Command Palette (`:`) untuk menyalin item aktif.
  - **Tempel (Paste)**:
    - Tekan `Ctrl+V` di semua input teks (Edit Jurnal, Tambah/Edit To-Do, Batch To-Do, Chat, Pencarian, Login/Server URL, dan Command Palette) untuk menempel teks langsung dari clipboard.
    - Perintah `:paste` di Command Palette (`:`) untuk membuat task to-do cepat dari clipboard.
- **Keluar**: `q` atau `Ctrl+C`.

Aturan input: username `^[a-z][a-z0-9_]{2,31}$` (dinormalisasi ke huruf kecil),
password 8–128 byte, timezone nama IANA (mis. `Asia/Jakarta`).

## Test
```sh
go test ./...
```
Integration test MariaDB hanya jalan bila `TEST_MARIADB_DSN` di-set. Test ini
**menghapus semua tabel** (migrate down → up), karena itu nama database wajib
mengandung `test`:
```powershell
$env:TEST_MARIADB_DSN="todoapp:todoapp@tcp(127.0.0.1:3306)/todoapp_test"; go test ./internal/server/store/ -v
```

## Kompilasi & Release Build (Fase 6)

### Kompilasi Lokal (Host OS)
```sh
# Menggunakan Makefile (Linux/macOS)
make build

# Menggunakan PowerShell (Windows)
.\build.ps1

# Menggunakan Bash
./build.sh
```
Binary hasil kompilasi akan diletakkan di direktori `bin/` dengan simbol stripped (`-ldflags="-s -w"`).

### Cross-Compilation (Linux, macOS, Windows)
```sh
make build-all
```
Menghasilkan binary siap distribusi untuk:
- Linux: `bin/todoapp-server-linux-amd64`, `bin/todoapp-client-linux-amd64`, `arm64`
- macOS: `bin/todoapp-server-darwin-amd64`, `bin/todoapp-client-darwin-amd64`, `arm64` (Apple Silicon)
- Windows: `bin/todoapp-server-windows-amd64.exe`, `bin/todoapp-client-windows-amd64.exe`, `arm64`

## Keamanan (ringkas)
- Password: argon2id (RFC 9106: m=64MiB, t=3, p=4), format PHC.
- Token sesi: 32 byte acak; DB hanya menyimpan SHA-256-nya. Sliding 30 hari.
- Login: rate limit per-IP **dan** per-username; waktu respons disamakan untuk
  username yang tidak ada (dummy hash).
- Semua query memakai placeholder `?` sebagai prepared statement server-side
  (`interpolateParams=false` dipaksa).
- Log request hanya mencatat method, path, status, durasi — tanpa header,
  query string, atau body.
- Body dibatasi (`MAX_BODY_BYTES`), JSON ditolak bila ada field tak dikenal.

## TLS (produksi)
Server sendiri berbicara HTTP biasa; jalankan di belakang reverse proxy yang
menangani TLS, lalu set `TRUST_PROXY=true` dan bind server ke `127.0.0.1`.

**Caddy** (sertifikat otomatis, WebSocket otomatis didukung):
```caddyfile
todo.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

**nginx**:
```nginx
server {
    listen 443 ssl http2;
    server_name todo.example.com;
    ssl_certificate     /etc/letsencrypt/live/todo.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/todo.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        # WebSocket
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 1h;
    }
}
```

## Lisensi
Proyek ini dilisensikan di bawah lisensi [MIT](LICENSE).

