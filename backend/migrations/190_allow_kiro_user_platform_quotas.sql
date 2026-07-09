-- 允许用户平台维度配额记录 Kiro。
--
-- 部分旧实例没有 user_platform_quotas 表；迁移应在表存在时更新约束，
-- 避免空库或精简实例执行到这里失败。

DO $$
BEGIN
  IF to_regclass('public.user_platform_quotas') IS NOT NULL THEN
    ALTER TABLE user_platform_quotas
      DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

    ALTER TABLE user_platform_quotas
      ADD CONSTRAINT user_platform_quotas_platform_check
      CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'kiro'));
  END IF;
END $$;
