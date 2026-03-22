-- 006_appointments_review_public_token.sql
-- 为预约表补齐公开评价令牌字段，后续评价外链统一按随机 token 访问，不再暴露自增预约 ID。

ALTER TABLE appointments
ADD COLUMN IF NOT EXISTS review_public_token TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS uq_appointments_review_public_token
  ON appointments(review_public_token)
  WHERE review_public_token IS NOT NULL;

COMMENT ON COLUMN appointments.review_public_token IS '公开评价令牌';
