# Fix: Permission Denied untuk Table sync_jobs

## Masalah
API endpoint `/api/v1/dashboard/top-jobs-performance` mengembalikan error:
```json
{
    "details": "failed to get top jobs by file count: permission denied for table sync_jobs",
    "error": "Failed to retrieve top jobs by file count",
    "success": false
}
```

## Penyebab
Database views `v_top_jobs_by_file_count` dan `v_top_jobs_by_data_size` mencoba mengakses tabel `sync_jobs`, tetapi database user yang digunakan oleh aplikasi tidak memiliki permission yang cukup.

## Solusi

### Langkah 1: Jalankan Migration Baru
Migration `006_fix_table_permissions.sql` telah dibuat untuk memberikan permission yang diperlukan.

**Cara menjalankan migration:**

#### Opsi A: Menggunakan psql (Manual)
```bash
# Login sebagai postgres superuser
sudo -u postgres psql

# Connect ke database bsync
\c bsync_db

# Jalankan migration
\i /home/primasys/bsync-universe/bsync-server/migrations/006_fix_table_permissions.sql

# Verifikasi permissions
SELECT grantee, privilege_type
FROM information_schema.table_privileges
WHERE table_name = 'sync_jobs';
```

#### Opsi B: Menggunakan psql dengan connection string
```bash
# Jika Anda tahu username dan password database
psql -h localhost -U <db_username> -d <db_name> -f /home/primasys/bsync-universe/bsync-server/migrations/006_fix_table_permissions.sql
```

#### Opsi C: Jika aplikasi memiliki migration runner
Jika aplikasi memiliki migration runner otomatis, restart aplikasi dan migration seharusnya berjalan otomatis.

### Langkah 2: Verifikasi Permission

Setelah menjalankan migration, verifikasi bahwa permission sudah diberikan:

```sql
-- Cek permission pada tabel sync_jobs
SELECT grantee, privilege_type
FROM information_schema.table_privileges
WHERE table_name = 'sync_jobs';

-- Cek permission pada views
SELECT table_name, grantee, privilege_type
FROM information_schema.table_privileges
WHERE table_name IN ('v_top_jobs_by_file_count', 'v_top_jobs_by_data_size');
```

### Langkah 3: Test API Endpoint

Setelah migration berhasil, test API endpoint:

```bash
curl -X GET http://localhost:8090/api/v1/dashboard/top-jobs-performance \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

Response yang diharapkan:
```json
{
    "success": true,
    "data": {
        "top_jobs_by_file_count": [...],
        "top_jobs_by_data_size": [...]
    }
}
```

## Catatan Keamanan

### Production Environment
Migration ini memberikan permission ke `PUBLIC` (semua users) untuk kemudahan development.

**Untuk production**, sebaiknya:
1. Buat dedicated database user untuk aplikasi
2. Grant permission hanya ke user tersebut

```sql
-- Buat user khusus untuk aplikasi
CREATE USER bsync_app_user WITH PASSWORD 'strong_password_here';

-- Revoke dari PUBLIC
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM PUBLIC;

-- Grant hanya ke app user
GRANT SELECT ON sync_jobs TO bsync_app_user;
GRANT SELECT ON integrated_agents TO bsync_app_user;
GRANT SELECT ON file_transfer_logs TO bsync_app_user;
-- dst untuk semua tabel yang diperlukan

-- Grant pada views
GRANT SELECT ON v_top_jobs_by_file_count TO bsync_app_user;
GRANT SELECT ON v_top_jobs_by_data_size TO bsync_app_user;
-- dst

-- Grant execute pada functions
GRANT EXECUTE ON FUNCTION get_dashboard_stats() TO bsync_app_user;
GRANT EXECUTE ON FUNCTION get_user_stats() TO bsync_app_user;
-- dst
```

## Troubleshooting

### Error: "relation sync_jobs does not exist"
Artinya tabel `sync_jobs` belum dibuat. Pastikan Anda sudah menjalankan semua migration sebelumnya (001-005).

### Error: "must be owner of table sync_jobs"
Anda harus menjalankan migration sebagai postgres superuser atau user yang memiliki permission untuk GRANT.

```bash
# Login sebagai postgres
sudo -u postgres psql bsync_db -f migrations/006_fix_table_permissions.sql
```

### Permission masih error setelah migration
1. Restart aplikasi bsync-server
2. Clear connection pool jika ada
3. Verifikasi permission dengan query di atas

## Files yang Terlibat

- [006_fix_table_permissions.sql](./006_fix_table_permissions.sql) - Migration untuk fix permissions
- [004_add_dashboard_stats.sql](./004_add_dashboard_stats.sql) - Migration yang membuat views (baris 128-160)
- [dashboard_repository.go](../internal/repository/dashboard_repository.go) - Repository yang menggunakan views (baris 269-333)
- [dashboard_handlers.go](../internal/server/dashboard_handlers.go) - Handler untuk API endpoint (baris 192-242)
