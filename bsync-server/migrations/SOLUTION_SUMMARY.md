## Solusi: Permission Denied untuk API top-jobs-performance

### 🔍 Root Cause Analysis

Setelah investigasi mendalam, masalah "permission denied for table sync_jobs" **BUKAN** disebabkan oleh kurangnya GRANT permissions (permissions sudah benar), melainkan ada **2 masalah yang saling berkaitan**:

#### Masalah 1: View Ownership
Views `v_top_jobs_by_file_count` dan `v_top_jobs_by_data_size` mungkin dibuat oleh user yang berbeda (bukan 'bsync'). Ketika user 'bsync' (yang digunakan aplikasi) mencoba query view tersebut, view dieksekusi dengan permission dari creator view, bukan user yang query.

**Bukti:**
- Aplikasi menggunakan user `bsync` (lihat [server.go:119](../internal/server/server.go#L119))
- Permission pada tabel sudah benar (PUBLIC dan bsync memiliki SELECT)
- Error terjadi saat view mencoba akses tabel underlying

#### Masalah 2: View Definition Tidak Kompatibel dengan Multi-Destination Model
Migration `005_add_multi_destination_support.sql` mengubah kolom `target_agent_id` menjadi NULLABLE dan memperkenalkan tabel `sync_job_destinations` untuk mendukung multiple destinations per job.

**View lama** (migration 004) masih menggunakan:
```sql
SELECT sj.target_agent_id FROM sync_jobs sj ...
```

Untuk multi-destination jobs, `target_agent_id` akan NULL, menyebabkan:
- Data tidak lengkap dalam view
- Potensial error saat GROUP BY dengan NULL values

### ✅ Solusi Lengkap

File migration: **`007_fix_dashboard_views_ownership.sql`**

Solusi ini mengatasi kedua masalah sekaligus:

1. **Drop dan Recreate Views** - Memastikan views dibuat dengan ownership yang benar
2. **Update View Logic** - Menggunakan COALESCE untuk handle multi-destination:
   ```sql
   COALESCE(sj.target_agent_id, jd.target_agent_id) as target_agent_id
   ```
3. **ALTER VIEW OWNER** - Explicitly set owner ke user `bsync`
4. **Recreate Semua Dashboard Views** - Untuk konsistensi

### 📋 Langkah Implementasi

#### Step 1: Jalankan Migration 007
```bash
# Opsi A: Sebagai postgres superuser (recommended)
psql -h localhost -U postgres -d bsync << 'EOF'
\i /home/primasys/bsync-universe/bsync-server/migrations/007_fix_dashboard_views_ownership.sql
EOF

# Opsi B: Sebagai user bsync (jika memiliki permission)
psql -h localhost -U bsync -d bsync << 'EOF'
\i /home/primasys/bsync-universe/bsync-server/migrations/007_fix_dashboard_views_ownership.sql
EOF
```

#### Step 2: Verifikasi
```bash
# Test ownership
psql -h localhost -U postgres -d bsync -f /home/primasys/bsync-universe/bsync-server/migrations/test_fix.sql
```

Expected output:
```
          viewname          | viewowner
----------------------------+-----------
 v_top_jobs_by_data_size   | bsync
 v_top_jobs_by_file_count  | bsync
```

#### Step 3: Restart Aplikasi
```bash
# Stop bsync-server
pkill bsync-server  # atau gunakan metode yang sesuai

# Start bsync-server
cd /home/primasys/bsync-universe/bsync-server
./bsync-server
```

#### Step 4: Test API Endpoint
```bash
curl -X GET http://localhost:8090/api/v1/dashboard/top-jobs-performance \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json"
```

Expected response:
```json
{
    "success": true,
    "data": {
        "top_jobs_by_file_count": [
            {
                "job_id": 1,
                "job_name": "sync-job-1",
                "source_agent_id": "agent-123",
                "target_agent_id": "agent-456",
                "job_status": "active",
                "total_files_transferred": 1500,
                "total_bytes_transferred": 52428800,
                "last_transfer_at": "2025-10-22T10:30:00Z"
            }
        ],
        "top_jobs_by_data_size": [...]
    }
}
```

### 🔧 Troubleshooting

#### Error: "must be owner of view" saat ALTER VIEW OWNER
**Solusi:** Jalankan migration sebagai postgres superuser
```bash
sudo -u postgres psql bsync -f migrations/007_fix_dashboard_views_ownership.sql
```

#### Error: "relation sync_jobs does not exist"
**Solusi:** Pastikan semua migration sebelumnya (001-006) sudah dijalankan
```bash
# Check existing tables
psql -h localhost -U postgres -d bsync -c "\dt"
```

#### API masih return error setelah migration
**Checklist:**
1. ✅ Migration 007 sudah dijalankan?
2. ✅ View ownership sudah benar? (jalankan test_fix.sql)
3. ✅ Aplikasi sudah di-restart?
4. ✅ Connection pool sudah di-clear? (restart memastikan ini)

#### View returns empty data
**Kemungkinan:** Tidak ada data di tabel `sync_jobs` atau `file_transfer_logs`
```sql
-- Check if there's data
SELECT COUNT(*) FROM sync_jobs;
SELECT COUNT(*) FROM file_transfer_logs WHERE status = 'completed';
```

### 📁 Files yang Dimodifikasi/Dibuat

1. **[007_fix_dashboard_views_ownership.sql](./007_fix_dashboard_views_ownership.sql)** - Main fix migration
2. **[test_fix.sql](./test_fix.sql)** - Verification script
3. **[debug_permissions.sql](./debug_permissions.sql)** - Debug helper
4. **[006_fix_table_permissions.sql](./006_fix_table_permissions.sql)** - Permission grants (masih relevan)

### 📊 Perubahan View

#### Before (Migration 004):
```sql
SELECT sj.target_agent_id FROM sync_jobs sj ...
```
❌ Tidak handle multi-destination jobs
❌ target_agent_id NULL untuk multi-dest jobs

#### After (Migration 007):
```sql
COALESCE(sj.target_agent_id, jd.target_agent_id) as target_agent_id
FROM sync_jobs sj
LEFT JOIN job_destinations jd ON sj.id = jd.job_id
```
✅ Supports legacy single-destination jobs
✅ Supports new multi-destination jobs
✅ Correct ownership (bsync user)

### 🎯 Kesimpulan

Error "permission denied for table sync_jobs" diselesaikan dengan:
1. Recreate views dengan ownership yang benar
2. Update view logic untuk support multi-destination model
3. Explicitly set view owner ke user aplikasi (`bsync`)

Setelah migration 007 dijalankan dan aplikasi di-restart, API endpoint `/api/v1/dashboard/top-jobs-performance` seharusnya berfungsi normal.
