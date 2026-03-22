-- 003_reviews_unique_appointment.sql
-- 修复 reviews 的 upsert 依赖：同一预约只允许存在一条评价记录。
-- 迁移前先按更新时间保留每个 appointment_id 最新的一条，避免旧环境重复数据导致唯一索引创建失败。

WITH ranked_reviews AS (
  SELECT
    id,
    ROW_NUMBER() OVER (
      PARTITION BY appointment_id
      ORDER BY updated_at DESC, created_at DESC, id DESC
    ) AS row_num
  FROM reviews
)
DELETE FROM reviews
WHERE id IN (
  SELECT id
  FROM ranked_reviews
  WHERE row_num > 1
);

DROP INDEX IF EXISTS idx_reviews_appointment_id;

CREATE UNIQUE INDEX IF NOT EXISTS uq_reviews_appointment_id
  ON reviews(appointment_id);
