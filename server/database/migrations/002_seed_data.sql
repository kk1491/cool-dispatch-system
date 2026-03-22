-- ============================================================================
-- 002_seed_data.sql
-- 默认演示数据种子脚本
-- 包含：用户、系统设置、服务项目、额外费用、服务区域、客户、预约单、
--       现金流水、评价、通知记录、LINE 好友
-- 所有 INSERT 使用 ON CONFLICT DO NOTHING 保证幂等可重复执行
-- ============================================================================

-- ---------- 用户（管理员 + 技师） ----------
-- SQL 种子脚本无法安全生成 bcrypt 哈希，因此此处仅占位写入空密码哈希。
-- 正常开发启动应通过 Go Seed + SEED_ADMIN_PASSWORD / SEED_TECHNICIAN_PASSWORD 初始化可登录账号。
INSERT INTO users (id, name, role, phone, password_hash, color, skills, zone_id, availability, created_at, updated_at)
VALUES
  (1, '管理員', 'admin', '0912345678', '', NULL, '[]'::jsonb, NULL, '[]'::jsonb, NOW(), NOW()),
  (2, '王師傅', 'technician', '0987654321', '', '#4f46e5', '["分離式","吊隱式"]'::jsonb, 'zone-1',
     '[{"day":1,"slots":["09:00","10:00","11:00","13:00","14:00","15:00","16:00"]},{"day":2,"slots":["09:00","10:00","11:00","13:00","14:00","15:00","16:00"]},{"day":3,"slots":["09:00","10:00","11:00","13:00","14:00","15:00","16:00"]},{"day":4,"slots":["09:00","10:00","11:00","13:00","14:00","15:00","16:00"]},{"day":5,"slots":["09:00","10:00","11:00","13:00","14:00","15:00","16:00"]}]'::jsonb,
     NOW(), NOW()),
  (3, '李師傅', 'technician', '0911111111', '', '#059669', '["分離式","窗型"]'::jsonb, 'zone-2',
     '[{"day":1,"slots":["08:00","09:00","10:00","11:00","12:00"]},{"day":3,"slots":["08:00","09:00","10:00","11:00","12:00"]},{"day":5,"slots":["08:00","09:00","10:00","11:00","12:00"]}]'::jsonb,
     NOW(), NOW()),
  (4, '陳師傅', 'technician', '0922222222', '', '#d97706', '["分離式","吊隱式","窗型"]'::jsonb, 'zone-1',
     '[{"day":2,"slots":["13:00","14:00","15:00","16:00","17:00","18:00"]},{"day":4,"slots":["13:00","14:00","15:00","16:00","17:00","18:00"]},{"day":6,"slots":["09:00","10:00","11:00","12:00"]}]'::jsonb,
     NOW(), NOW())
ON CONFLICT (id) DO NOTHING;

-- ---------- 系统设置 ----------
INSERT INTO app_settings (key, value, description, created_at, updated_at)
VALUES
  ('reminder_days', '180', '客戶完工後幾天提醒回訪', NOW(), NOW())
ON CONFLICT (key) DO NOTHING;

-- ---------- 服务项目 ----------
INSERT INTO service_items (id, name, default_price, description, created_at, updated_at)
VALUES
  ('si-1', '分離式', 2500, '壁掛式室內機 + 室外機', NOW(), NOW()),
  ('si-2', '吊隱式', 3500, '隱藏於天花板內的機型', NOW(), NOW()),
  ('si-3', '窗型',   2000, '窗戶安裝一體機',       NOW(), NOW())
ON CONFLICT (id) DO NOTHING;

-- ---------- 额外费用项目 ----------
INSERT INTO extra_items (id, name, price, created_at, updated_at)
VALUES
  ('1', '加購清潔劑', 200, NOW(), NOW()),
  ('2', '高樓層費',   500, NOW(), NOW())
ON CONFLICT (id) DO NOTHING;

-- ---------- 服务区域 ----------
INSERT INTO service_zones (id, name, districts, assigned_technician_ids, created_at, updated_at)
VALUES
  ('zone-1', '台北市信義/大安/中山',
   '["信義區","大安區","中山區","松山區","中正區"]'::jsonb,
   '[2,4]'::jsonb, NOW(), NOW()),
  ('zone-2', '新北市板橋/中和/永和',
   '["板橋區","中和區","永和區","新店區","土城區"]'::jsonb,
   '[3]'::jsonb, NOW(), NOW())
ON CONFLICT (id) DO NOTHING;

-- ---------- 客户 ----------
INSERT INTO customers (id, name, phone, address, line_id, line_name, line_picture, line_uid, line_joined_at, line_data, created_at, updated_at)
VALUES
  ('0922333444', '張先生', '0922333444', '台北市信義區信義路五段7號',
   NULL, 'Kevin Chang', 'https://api.dicebear.com/7.x/avataaars/svg?seed=Kevin', 'U123456789',
   NOW() - INTERVAL '4 months', '{}'::jsonb, NOW(), NOW()),
  ('0933444555', '林小姐', '0933444555', '新北市板橋區文化路一段',
   NULL, '林小美', 'https://api.dicebear.com/7.x/avataaars/svg?seed=Lin', 'U987654321',
   NOW() - INTERVAL '3 months 5 days', '{}'::jsonb, NOW(), NOW()),
  ('0955666777', '陳先生', '0955666777', '台北市大安區忠孝東路',
   NULL, NULL, NULL, NULL, NULL, '{}'::jsonb, NOW(), NOW()),
  ('0966777888', '黃太太', '0966777888', '台北市中山區南京東路三段',
   NULL, NULL, NULL, NULL, NULL, '{}'::jsonb, NOW(), NOW()),
  ('0977888999', '曾先生', '0977888999', '新北市中和區景安路',
   NULL, NULL, NULL, NULL, NULL, '{}'::jsonb, NOW(), NOW()),
  ('0912888777', '林先生', '0912888777', '台北市信義區忠孝東路五段',
   NULL, NULL, NULL, NULL, NULL, '{}'::jsonb, NOW(), NOW()),
  ('0933999111', '柯小姐', '0933999111', '新北市永和區中山路一段',
   NULL, NULL, NULL, NULL, NULL, '{}'::jsonb, NOW(), NOW()),
  ('0922111333', '吳太太', '0922111333', '台北市松山區南京東路五段',
   NULL, NULL, NULL, NULL, NULL, '{}'::jsonb, NOW(), NOW()),
  ('0955222444', '許先生', '0955222444', '新北市板橋區民生路二段',
   NULL, NULL, NULL, NULL, NULL, '{}'::jsonb, NOW(), NOW()),
  ('0966333555', '趙小姐', '0966333555', '台北市大安區復興南路一段',
   NULL, NULL, NULL, NULL, NULL, '{}'::jsonb, NOW(), NOW()),
  ('0977444666', '鄭太太', '0977444666', '新北市中和區景平路',
   NULL, NULL, NULL, NULL, NULL, '{}'::jsonb, NOW(), NOW()),
  ('0988555777', '蔡先生', '0988555777', '台北市中正區忠孝西路一段',
   NULL, NULL, NULL, NULL, NULL, '{}'::jsonb, NOW(), NOW())
ON CONFLICT (id) DO NOTHING;

-- ---------- 预约单（12 笔，覆盖多种状态） ----------
-- 需要先重置序列以确保后续插入不冲突
SELECT setval(pg_get_serial_sequence('appointments', 'id'), 100, true);

INSERT INTO appointments (
  id, customer_name, address, phone, items, extra_items,
  payment_method, total_amount, discount_amount, paid_amount,
  scheduled_at, scheduled_end, status, cancel_reason,
  technician_id, technician_name, lat, lng,
  checkin_time, checkout_time, departed_time, completed_time, payment_time,
  photos, payment_received, signature_data, line_uid, zone_id,
  created_at, updated_at
)
VALUES
  -- #1 张先生 - assigned（今天 10:00）
  (1, '張先生', '台北市信義區信義路五段7號', '0922333444',
   '[{"id":"1","type":"分離式","note":"客廳","price":2500},{"id":"2","type":"分離式","note":"主臥","price":2500}]'::jsonb,
   '[]'::jsonb, '現金', 5000, 0, 0,
   (CURRENT_DATE + TIME '10:00'), (CURRENT_DATE + TIME '12:00'), 'assigned', NULL,
   2, '王師傅', NULL, NULL,
   NULL, NULL, NULL, NULL, NULL,
   '[]'::jsonb, FALSE, NULL, NULL, 'zone-1',
   NOW(), NOW()),

  -- #2 林小姐 - pending（今天 14:00）
  (2, '林小姐', '新北市板橋區文化路一段', '0933444555',
   '[{"id":"3","type":"吊隱式","note":"全室","price":3500}]'::jsonb,
   '[{"id":"e1","name":"室外機清洗","price":500}]'::jsonb, '轉帳', 4000, 0, 0,
   (CURRENT_DATE + TIME '14:00'), (CURRENT_DATE + TIME '15:30'), 'pending', NULL,
   NULL, NULL, NULL, NULL,
   NULL, NULL, NULL, NULL, NULL,
   '[]'::jsonb, FALSE, NULL, 'U987654321', 'zone-2',
   NOW(), NOW()),

  -- #3 陈先生 - completed（昨天 09:00）
  (3, '陳先生', '台北市大安區忠孝東路', '0955666777',
   '[{"id":"4","type":"窗型","note":"書房","price":1800}]'::jsonb,
   '[]'::jsonb, '現金', 1800, 0, 1800,
   (CURRENT_DATE - INTERVAL '1 day' + TIME '09:00'), (CURRENT_DATE - INTERVAL '1 day' + TIME '10:00'), 'completed', NULL,
   3, '李師傅', NULL, NULL,
   (CURRENT_DATE - INTERVAL '1 day' + TIME '09:05'), (CURRENT_DATE - INTERVAL '1 day' + TIME '09:55'), NULL, (CURRENT_DATE - INTERVAL '1 day' + TIME '09:55'), NULL,
   '["https://images.unsplash.com/photo-1581094288338-2314dddb79a1?w=400"]'::jsonb, TRUE,
   'data:image/svg+xml;base64,PHN2Zy8+', NULL, 'zone-1',
   (CURRENT_DATE - INTERVAL '2 days'), NOW()),

  -- #4 黄太太 - assigned（今天 16:00）
  (4, '黃太太', '台北市中山區南京東路三段', '0966777888',
   '[{"id":"5","type":"分離式","note":"客廳","price":2500}]'::jsonb,
   '[]'::jsonb, '轉帳', 2500, 0, 0,
   (CURRENT_DATE + TIME '16:00'), (CURRENT_DATE + TIME '17:00'), 'assigned', NULL,
   4, '陳師傅', NULL, NULL,
   NULL, NULL, NULL, NULL, NULL,
   '[]'::jsonb, FALSE, NULL, NULL, 'zone-1',
   NOW(), NOW()),

  -- #5 曾先生 - pending（明天 10:00）
  (5, '曾先生', '新北市中和區景安路', '0977888999',
   '[{"id":"6","type":"分離式","note":"主臥","price":2500}]'::jsonb,
   '[{"id":"e2","name":"高樓層費","price":500}]'::jsonb, '現金', 3000, 0, 0,
   (CURRENT_DATE + INTERVAL '1 day' + TIME '10:00'), (CURRENT_DATE + INTERVAL '1 day' + TIME '11:00'), 'pending', NULL,
   NULL, NULL, NULL, NULL,
   NULL, NULL, NULL, NULL, NULL,
   '[]'::jsonb, FALSE, NULL, NULL, 'zone-2',
   NOW(), NOW()),

  -- #6 林先生 - completed（昨天 15:00）
  (6, '林先生', '台北市信義區忠孝東路五段', '0912888777',
   '[{"id":"7","type":"分離式","note":"客廳","price":2500}]'::jsonb,
   '[]'::jsonb, '現金', 2500, 0, 2500,
   (CURRENT_DATE - INTERVAL '1 day' + TIME '15:00'), (CURRENT_DATE - INTERVAL '1 day' + TIME '16:00'), 'completed', NULL,
   2, '王師傅', NULL, NULL,
   (CURRENT_DATE - INTERVAL '1 day' + TIME '15:10'), (CURRENT_DATE - INTERVAL '1 day' + TIME '16:10'), NULL, (CURRENT_DATE - INTERVAL '1 day' + TIME '16:10'), NULL,
   '["https://images.unsplash.com/photo-1585771724684-38269d6639fd?w=400"]'::jsonb, TRUE,
   'data:image/svg+xml;base64,PHN2Zy8+', NULL, 'zone-1',
   (CURRENT_DATE - INTERVAL '6 days'), NOW()),

  -- #7 柯小姐 - completed（今天 - 2小时前）
  (7, '柯小姐', '新北市永和區中山路一段', '0933999111',
   '[{"id":"8","type":"窗型","note":"臥室","price":1800}]'::jsonb,
   '[{"id":"e3","name":"室外機清洗","price":500}]'::jsonb, '轉帳', 2300, 0, 2300,
   (NOW() - INTERVAL '2 hours'), (NOW() - INTERVAL '1 hour'), 'completed', NULL,
   3, '李師傅', NULL, NULL,
   (NOW() - INTERVAL '115 minutes'), (NOW() - INTERVAL '65 minutes'), NULL, (NOW() - INTERVAL '65 minutes'), NULL,
   '["https://images.unsplash.com/photo-1527515637462-cff94eecc1ac?w=400"]'::jsonb, TRUE,
   'data:image/svg+xml;base64,PHN2Zy8+', NULL, 'zone-2',
   NOW(), NOW()),

  -- #8 吴太太 - completed（昨天）
  (8, '吳太太', '台北市松山區南京東路五段', '0922111333',
   '[{"id":"9","type":"分離式","note":"客廳","price":2500},{"id":"10","type":"分離式","note":"主臥","price":2500},{"id":"11","type":"窗型","note":"小孩房","price":1800}]'::jsonb,
   '[]'::jsonb, '現金', 6800, 0, 6800,
   (CURRENT_DATE - INTERVAL '1 day' + TIME '10:00'), (CURRENT_DATE - INTERVAL '1 day' + TIME '13:00'), 'completed', NULL,
   2, '王師傅', NULL, NULL,
   (CURRENT_DATE - INTERVAL '1 day' + TIME '10:05'), (CURRENT_DATE - INTERVAL '1 day' + TIME '12:55'), NULL, (CURRENT_DATE - INTERVAL '1 day' + TIME '12:55'), NULL,
   '["https://images.unsplash.com/photo-1558618666-fcd25c85f82e?w=400"]'::jsonb, TRUE,
   'data:image/svg+xml;base64,PHN2Zy8+', NULL, 'zone-1',
   (CURRENT_DATE - INTERVAL '1 day'), NOW()),

  -- #9 许先生 - completed（2天前）
  (9, '許先生', '新北市板橋區民生路二段', '0955222444',
   '[{"id":"12","type":"吊隱式","note":"客廳","price":3500},{"id":"13","type":"吊隱式","note":"主臥","price":3500}]'::jsonb,
   '[{"id":"e4","name":"室外機清洗","price":500}]'::jsonb, '轉帳', 7500, 0, 7500,
   (CURRENT_DATE - INTERVAL '2 days' + TIME '10:00'), (CURRENT_DATE - INTERVAL '2 days' + TIME '13:00'), 'completed', NULL,
   4, '陳師傅', NULL, NULL,
   (CURRENT_DATE - INTERVAL '2 days' + TIME '10:10'), (CURRENT_DATE - INTERVAL '2 days' + TIME '12:50'), NULL, (CURRENT_DATE - INTERVAL '2 days' + TIME '12:50'), NULL,
   '["https://images.unsplash.com/photo-1504280390367-361c6d9f38f4?w=400"]'::jsonb, TRUE,
   'data:image/svg+xml;base64,PHN2Zy8+', NULL, 'zone-2',
   (CURRENT_DATE - INTERVAL '2 days'), NOW()),

  -- #10 赵小姐 - completed（3天前）
  (10, '趙小姐', '台北市大安區復興南路一段', '0966333555',
   '[{"id":"14","type":"分離式","note":"客廳","price":2500}]'::jsonb,
   '[]'::jsonb, '現金', 2500, 0, 2500,
   (CURRENT_DATE - INTERVAL '3 days' + TIME '10:00'), (CURRENT_DATE - INTERVAL '3 days' + TIME '11:00'), 'completed', NULL,
   3, '李師傅', NULL, NULL,
   (CURRENT_DATE - INTERVAL '3 days' + TIME '10:05'), (CURRENT_DATE - INTERVAL '3 days' + TIME '10:55'), NULL, (CURRENT_DATE - INTERVAL '3 days' + TIME '10:55'), NULL,
   '["https://images.unsplash.com/photo-1551361415-69c87624334f?w=400"]'::jsonb, TRUE,
   'data:image/svg+xml;base64,PHN2Zy8+', NULL, 'zone-1',
   (CURRENT_DATE - INTERVAL '3 days'), NOW()),

  -- #11 郑太太 - completed（4天前）
  (11, '鄭太太', '新北市中和區景平路', '0977444666',
   '[{"id":"15","type":"窗型","note":"廚房","price":1800},{"id":"16","type":"分離式","note":"客廳","price":2500}]'::jsonb,
   '[{"id":"e5","name":"加購清潔劑","price":200}]'::jsonb, '現金', 4500, 0, 4500,
   (CURRENT_DATE - INTERVAL '4 days' + TIME '10:00'), (CURRENT_DATE - INTERVAL '4 days' + TIME '12:00'), 'completed', NULL,
   2, '王師傅', NULL, NULL,
   (CURRENT_DATE - INTERVAL '4 days' + TIME '10:10'), (CURRENT_DATE - INTERVAL '4 days' + TIME '11:50'), NULL, (CURRENT_DATE - INTERVAL '4 days' + TIME '11:50'), NULL,
   '["https://images.unsplash.com/photo-1585771724684-38269d6639fd?w=400"]'::jsonb, TRUE,
   'data:image/svg+xml;base64,PHN2Zy8+', NULL, 'zone-2',
   (CURRENT_DATE - INTERVAL '4 days'), NOW()),

  -- #12 蔡先生 - completed（5天前）
  (12, '蔡先生', '台北市中正區忠孝西路一段', '0988555777',
   '[{"id":"17","type":"分離式","note":"辦公室A","price":2500},{"id":"18","type":"分離式","note":"辦公室B","price":2500},{"id":"19","type":"吊隱式","note":"會議室","price":3500},{"id":"20","type":"分離式","note":"老闆室","price":2500}]'::jsonb,
   '[{"id":"e6","name":"高樓層費","price":500}]'::jsonb, '轉帳', 11500, 0, 11000,
   (CURRENT_DATE - INTERVAL '5 days' + TIME '09:00'), (CURRENT_DATE - INTERVAL '5 days' + TIME '14:00'), 'completed', NULL,
   4, '陳師傅', NULL, NULL,
   (CURRENT_DATE - INTERVAL '5 days' + TIME '09:10'), (CURRENT_DATE - INTERVAL '5 days' + TIME '13:50'), NULL, (CURRENT_DATE - INTERVAL '5 days' + TIME '13:50'), NULL,
   '["https://images.unsplash.com/photo-1504280390367-361c6d9f38f4?w=400"]'::jsonb, TRUE,
   'data:image/svg+xml;base64,PHN2Zy8+', NULL, 'zone-1',
   (CURRENT_DATE - INTERVAL '5 days'), NOW())
ON CONFLICT (id) DO NOTHING;

-- 重置预约序列到当前最大 ID 之后，防止新建预约时 ID 冲突
SELECT setval(pg_get_serial_sequence('appointments', 'id'), GREATEST((SELECT MAX(id) FROM appointments), 12) + 1, false);

-- ---------- 现金流水 ----------
INSERT INTO cash_ledger_entries (id, technician_id, appointment_id, type, amount, note, created_at, updated_at)
VALUES
  ('cl-1', 3, 3,  'collect', 1800, '陳先生冷氣清洗 - 現金收款', NOW() - INTERVAL '48 hours', NOW()),
  ('cl-2', 2, 6,  'collect', 2500, '林先生冷氣清洗 - 現金收款', NOW() - INTERVAL '24 hours', NOW()),
  ('cl-3', 3, 10, 'collect', 2500, '趙小姐冷氣清洗 - 現金收款', NOW() - INTERVAL '2 hours', NOW()),
  ('cl-4', 2, 8,  'collect', 6800, '吳太太冷氣清洗 - 現金收款', NOW() - INTERVAL '24 hours', NOW()),
  ('cl-5', 2, 11, 'collect', 4500, '鄭太太冷氣清洗 - 現金收款', NOW() - INTERVAL '4 days', NOW()),
  ('cl-6', 2, 6,  'return',  2500, '林先生現金繳回',            NOW() - INTERVAL '20 hours', NOW()),
  ('cl-7', 2, 8,  'return',  6800, '吳太太現金繳回',            NOW() - INTERVAL '20 hours', NOW())
ON CONFLICT (id) DO NOTHING;

-- ---------- 评价（5 笔，覆盖多种评分和违纪） ----------
INSERT INTO reviews (id, appointment_id, customer_name, technician_id, technician_name, rating, misconducts, comment, shared_line, created_at, updated_at)
VALUES
  ('rev-1', 3,  '陳先生', 3, '李師傅', 5, '[]'::jsonb,
   '服務很專業，冷氣清洗得很乾淨！', FALSE, NOW() - INTERVAL '24 hours', NOW()),
  ('rev-2', 6,  '林先生', 2, '王師傅', 3, '["late_arrival","not_clean"]'::jsonb,
   '師傅遲到了半小時，清洗後還是有點髒', FALSE, NOW() - INTERVAL '48 hours', NOW()),
  ('rev-3', 7,  '柯小姐', 3, '李師傅', 4, '[]'::jsonb,
   '整體不錯，下次會再預約', FALSE, NOW() - INTERVAL '2 days', NOW()),
  ('rev-4', 8,  '吳太太', 2, '王師傅', 2, '["bad_attitude","overcharge","not_clean"]'::jsonb,
   '態度很差，還多收了費用', FALSE, NOW() - INTERVAL '3 days', NOW()),
  ('rev-5', 9,  '許先生', 4, '陳師傅', 5, '[]'::jsonb,
   '', FALSE, NOW() - INTERVAL '4 days', NOW())
ON CONFLICT (id) DO NOTHING;

-- ---------- 通知记录 ----------
INSERT INTO notification_logs (id, appointment_id, type, message, sent_at, created_at, updated_at)
VALUES
  ('notif-1', 1, 'line',
   '您好 張先生，您的冷氣清洗已預約在今日 10:00，師傅 王師傅 將為您服務。',
   NOW() - INTERVAL '2 hours', NOW() - INTERVAL '2 hours', NOW()),
  ('notif-2', 2, 'line',
   '您好 林小姐，您的冷氣清洗已預約在今日 14:00，我們將盡快為您安排師傅。',
   NOW() - INTERVAL '1 hour', NOW() - INTERVAL '1 hour', NOW()),
  ('notif-3', 4, 'sms',
   '黃太太您好，您的冷氣清洗預約在今日 16:00，師傅 陳師傅 將為您服務。如需更改請來電。',
   NOW() - INTERVAL '3 hours', NOW() - INTERVAL '3 hours', NOW())
ON CONFLICT (id) DO NOTHING;

-- ---------- LINE 好友 ----------
INSERT INTO line_friends (line_uid, line_name, line_picture, joined_at, phone, linked_customer_id, status, last_payload, created_at, updated_at)
VALUES
  ('U123456789', 'Kevin Chang',
   'https://api.dicebear.com/7.x/avataaars/svg?seed=Kevin',
   '2025-11-20T08:30:00Z', '0922333444', '0922333444', 'followed', '{}'::jsonb, NOW(), NOW()),
  ('U987654321', '林小美',
   'https://api.dicebear.com/7.x/avataaars/svg?seed=Lin',
   '2025-12-05T14:20:00Z', '0933444555', '0933444555', 'followed', '{}'::jsonb, NOW(), NOW()),
  ('Uabc111222', '小花花',
   'https://api.dicebear.com/7.x/avataaars/svg?seed=Flower',
   '2026-01-15T09:00:00Z', NULL, NULL, 'followed', '{}'::jsonb, NOW(), NOW()),
  ('Udef333444', 'David Wu',
   'https://api.dicebear.com/7.x/avataaars/svg?seed=David',
   '2026-02-01T16:45:00Z', NULL, NULL, 'followed', '{}'::jsonb, NOW(), NOW()),
  ('Ughi555666', '阿明的冷氣',
   'https://api.dicebear.com/7.x/avataaars/svg?seed=Ming',
   '2026-02-28T11:10:00Z', NULL, NULL, 'followed', '{}'::jsonb, NOW(), NOW()),
  ('Ujkl777888', '陳太太',
   'https://api.dicebear.com/7.x/avataaars/svg?seed=ChenTai',
   '2026-03-01T07:55:00Z', NULL, NULL, 'followed', '{}'::jsonb, NOW(), NOW())
ON CONFLICT (line_uid) DO NOTHING;
