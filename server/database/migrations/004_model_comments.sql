-- 004_model_comments.sql
-- 为已有数据库补齐表和字段注释。
-- 旧环境可能存在列尚未补齐的情况，因此本迁移会按“表/列存在才写注释”的方式安全执行。

DO $$
DECLARE
  target_table text;
  target_column text;
  target_comment text;
BEGIN
  FOR target_table, target_comment IN
    SELECT * FROM (
      VALUES
        ('users', '系统用户表'),
        ('line_friends', 'LINE好友表'),
        ('line_events', 'LINE事件表'),
        ('customers', '客户主档表'),
        ('appointments', '预约工单表'),
        ('service_zones', '服务区域表'),
        ('service_items', '服务项目表'),
        ('extra_items', '额外收费项表'),
        ('cash_ledger_entries', '现金流水表'),
        ('reviews', '评价表'),
        ('notification_logs', '通知日志表'),
        ('app_settings', '系统设置表'),
        ('auth_tokens', '认证令牌表')
    ) AS table_comments(table_name, table_comment)
  LOOP
    IF EXISTS (
      SELECT 1
      FROM information_schema.tables
      WHERE table_schema = 'public' AND table_name = target_table
    ) THEN
      EXECUTE format('COMMENT ON TABLE %I IS %L', target_table, target_comment);
    END IF;
  END LOOP;

  FOR target_table, target_column, target_comment IN
    SELECT * FROM (
      VALUES
        ('users', 'id', '用户主键'),
        ('users', 'name', '用户名称'),
        ('users', 'role', '用户角色'),
        ('users', 'phone', '用户手机号'),
        ('users', 'password_hash', '密码哈希'),
        ('users', 'color', '技师展示色'),
        ('users', 'skills', '技师技能列表'),
        ('users', 'zone_id', '默认服务区域ID'),
        ('users', 'availability', '技师可用时段'),
        ('users', 'created_at', '创建时间'),
        ('users', 'updated_at', '更新时间'),
        ('line_friends', 'line_uid', 'LINE用户UID'),
        ('line_friends', 'line_name', 'LINE昵称'),
        ('line_friends', 'line_picture', 'LINE头像地址'),
        ('line_friends', 'joined_at', '首次关注时间'),
        ('line_friends', 'phone', '手机号'),
        ('line_friends', 'linked_customer_id', '绑定客户ID'),
        ('line_friends', 'status', '好友状态'),
        ('line_friends', 'last_payload', '最近一次事件负载'),
        ('line_friends', 'created_at', '创建时间'),
        ('line_friends', 'updated_at', '更新时间'),
        ('line_events', 'id', '事件主键'),
        ('line_events', 'event_type', '事件类型'),
        ('line_events', 'line_uid', '关联LINE用户UID'),
        ('line_events', 'received_at', '接收时间'),
        ('line_events', 'payload', '原始事件内容'),
        ('line_events', 'created_at', '创建时间'),
        ('line_events', 'updated_at', '更新时间'),
        ('customers', 'id', '客户主键'),
        ('customers', 'name', '客户名称'),
        ('customers', 'phone', '客户手机号'),
        ('customers', 'address', '客户地址'),
        ('customers', 'line_id', 'LINE标识'),
        ('customers', 'line_name', 'LINE昵称'),
        ('customers', 'line_picture', 'LINE头像地址'),
        ('customers', 'line_uid', 'LINE用户UID'),
        ('customers', 'line_joined_at', 'LINE关注时间'),
        ('customers', 'line_data', 'LINE扩展资料'),
        ('customers', 'created_at', '创建时间'),
        ('customers', 'updated_at', '更新时间'),
        ('appointments', 'id', '预约主键'),
        ('appointments', 'customer_name', '客户名称快照'),
        ('appointments', 'address', '服务地址'),
        ('appointments', 'phone', '联系电话'),
        ('appointments', 'items', '服务项目列表'),
        ('appointments', 'extra_items', '额外收费项目列表'),
        ('appointments', 'payment_method', '付款方式'),
        ('appointments', 'total_amount', '应收总金额'),
        ('appointments', 'discount_amount', '折扣金额'),
        ('appointments', 'paid_amount', '已收金额'),
        ('appointments', 'scheduled_at', '预约开始时间'),
        ('appointments', 'scheduled_end', '预约结束时间'),
        ('appointments', 'status', '预约状态'),
        ('appointments', 'cancel_reason', '取消原因'),
        ('appointments', 'technician_id', '指派技师ID'),
        ('appointments', 'technician_name', '技师名称快照'),
        ('appointments', 'lat', '纬度'),
        ('appointments', 'lng', '经度'),
        ('appointments', 'checkin_time', '签到时间'),
        ('appointments', 'checkout_time', '签退时间'),
        ('appointments', 'departed_time', '出发时间'),
        ('appointments', 'completed_time', '完成时间'),
        ('appointments', 'payment_time', '确认收款时间'),
        ('appointments', 'photos', '现场照片列表'),
        ('appointments', 'payment_received', '是否已确认收款'),
        ('appointments', 'signature_data', '客户签名数据'),
        ('appointments', 'line_uid', '关联LINE用户UID'),
        ('appointments', 'zone_id', '匹配服务区域ID'),
        ('appointments', 'review_public_token', '公开评价令牌'),
        ('appointments', 'created_at', '创建时间'),
        ('appointments', 'updated_at', '更新时间'),
        ('service_zones', 'id', '服务区域主键'),
        ('service_zones', 'name', '服务区域名称'),
        ('service_zones', 'districts', '行政区列表'),
        ('service_zones', 'assigned_technician_ids', '分配技师ID列表'),
        ('service_zones', 'created_at', '创建时间'),
        ('service_zones', 'updated_at', '更新时间'),
        ('service_items', 'id', '服务项目主键'),
        ('service_items', 'name', '服务项目名称'),
        ('service_items', 'default_price', '默认报价'),
        ('service_items', 'description', '服务项目描述'),
        ('service_items', 'created_at', '创建时间'),
        ('service_items', 'updated_at', '更新时间'),
        ('extra_items', 'id', '额外收费项主键'),
        ('extra_items', 'name', '额外收费项名称'),
        ('extra_items', 'price', '额外收费金额'),
        ('extra_items', 'created_at', '创建时间'),
        ('extra_items', 'updated_at', '更新时间'),
        ('cash_ledger_entries', 'id', '现金流水主键'),
        ('cash_ledger_entries', 'technician_id', '技师ID'),
        ('cash_ledger_entries', 'appointment_id', '关联预约ID'),
        ('cash_ledger_entries', 'type', '流水类型'),
        ('cash_ledger_entries', 'amount', '流水金额'),
        ('cash_ledger_entries', 'note', '流水备注'),
        ('cash_ledger_entries', 'created_at', '创建时间'),
        ('cash_ledger_entries', 'updated_at', '更新时间'),
        ('reviews', 'id', '评价主键'),
        ('reviews', 'appointment_id', '预约ID'),
        ('reviews', 'customer_name', '客户名称快照'),
        ('reviews', 'technician_id', '技师ID'),
        ('reviews', 'technician_name', '技师名称快照'),
        ('reviews', 'rating', '评分'),
        ('reviews', 'misconducts', '异常行为标签列表'),
        ('reviews', 'comment', '评价内容'),
        ('reviews', 'shared_line', '是否已分享到LINE'),
        ('reviews', 'created_at', '创建时间'),
        ('reviews', 'updated_at', '更新时间'),
        ('notification_logs', 'id', '通知日志主键'),
        ('notification_logs', 'appointment_id', '预约ID'),
        ('notification_logs', 'type', '通知类型'),
        ('notification_logs', 'message', '通知内容'),
        ('notification_logs', 'sent_at', '发送时间'),
        ('notification_logs', 'created_at', '创建时间'),
        ('notification_logs', 'updated_at', '更新时间'),
        ('app_settings', 'key', '配置键'),
        ('app_settings', 'value', '配置值'),
        ('app_settings', 'description', '配置说明'),
        ('app_settings', 'created_at', '创建时间'),
        ('app_settings', 'updated_at', '更新时间'),
        ('auth_tokens', 'id', '认证令牌主键'),
        ('auth_tokens', 'user_id', '所属用户ID'),
        ('auth_tokens', 'token', '认证令牌'),
        ('auth_tokens', 'expires_at', '过期时间'),
        ('auth_tokens', 'created_at', '创建时间'),
        ('auth_tokens', 'updated_at', '更新时间')
    ) AS column_comments(table_name, column_name, column_comment)
  LOOP
    IF EXISTS (
      SELECT 1
      FROM information_schema.columns
      WHERE table_schema = 'public'
        AND table_name = target_table
        AND column_name = target_column
    ) THEN
      EXECUTE format('COMMENT ON COLUMN %I.%I IS %L', target_table, target_column, target_comment);
    END IF;
  END LOOP;
END $$;
