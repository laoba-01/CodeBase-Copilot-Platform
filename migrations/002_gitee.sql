-- Allow Gitee OAuth users alongside GitHub
ALTER TABLE users ALTER COLUMN github_id DROP NOT NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS gitee_id BIGINT UNIQUE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS provider TEXT DEFAULT 'github' CHECK (provider IN ('github', 'gitee'));
