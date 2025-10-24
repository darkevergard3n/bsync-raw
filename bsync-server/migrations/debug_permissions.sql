-- Debug script untuk mengecek permission issues
-- Jalankan sebagai postgres superuser atau user bsync

-- 1. Cek ownership dari view
SELECT
    schemaname,
    viewname,
    viewowner,
    definition
FROM pg_views
WHERE viewname IN ('v_top_jobs_by_file_count', 'v_top_jobs_by_data_size');

-- 2. Cek ownership dari tabel sync_jobs
SELECT
    schemaname,
    tablename,
    tableowner
FROM pg_tables
WHERE tablename = 'sync_jobs';

-- 3. Cek permissions yang dimiliki user bsync pada sync_jobs
SELECT
    grantee,
    table_schema,
    table_name,
    privilege_type
FROM information_schema.table_privileges
WHERE table_name = 'sync_jobs'
  AND grantee = 'bsync';

-- 4. Cek apakah user bsync bisa akses views
SELECT
    grantee,
    table_schema,
    table_name,
    privilege_type
FROM information_schema.table_privileges
WHERE table_name IN ('v_top_jobs_by_file_count', 'v_top_jobs_by_data_size')
  AND grantee IN ('bsync', 'PUBLIC');

-- 5. Test query langsung sebagai user bsync
-- (Jalankan ini setelah connect sebagai user bsync)
-- SELECT * FROM v_top_jobs_by_file_count LIMIT 1;

-- 6. Cek apakah ada RLS (Row Level Security) yang aktif
SELECT
    schemaname,
    tablename,
    rowsecurity
FROM pg_tables
WHERE tablename = 'sync_jobs';

-- 7. Cek current user dan search_path
SELECT current_user, current_schema();
SHOW search_path;
