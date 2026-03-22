-- 005_users_password_hash.sql
-- 为历史数据库补齐 users.password_hash 字段，兼容旧库仍停留在明文密码时代的结构。

ALTER TABLE users
ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN users.password_hash IS '密码哈希';
