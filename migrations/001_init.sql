CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    github_id BIGINT UNIQUE NOT NULL,
    username TEXT NOT NULL,
    email TEXT,
    avatar_url TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE repos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    full_name TEXT NOT NULL,
    clone_url TEXT NOT NULL,
    default_branch TEXT DEFAULT 'main',
    status TEXT DEFAULT 'pending' CHECK (status IN ('pending','cloning','indexing','ready','error')),
    indexed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_repos_user ON repos(user_id);

CREATE TABLE index_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_id UUID NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('file','function','class','method')),
    name TEXT NOT NULL,
    signature TEXT DEFAULT '',
    code TEXT DEFAULT '',
    file_path TEXT NOT NULL,
    start_line INT DEFAULT 0,
    end_line INT DEFAULT 0,
    summary TEXT DEFAULT '',
    embedding vector(1024),
    language TEXT DEFAULT '',
    package TEXT DEFAULT '',
    metadata JSONB DEFAULT '{}'
);
CREATE INDEX idx_nodes_repo_type ON index_nodes(repo_id, type);
CREATE INDEX idx_nodes_file_path ON index_nodes(repo_id, file_path);

CREATE TABLE call_edges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_id UUID NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    caller_id UUID NOT NULL REFERENCES index_nodes(id) ON DELETE CASCADE,
    callee_id UUID NOT NULL REFERENCES index_nodes(id) ON DELETE CASCADE,
    file_path TEXT DEFAULT '',
    line INT DEFAULT 0
);
CREATE INDEX idx_call_edges_repo ON call_edges(repo_id);

CREATE TABLE dep_edges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_id UUID NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    source_id UUID NOT NULL REFERENCES index_nodes(id) ON DELETE CASCADE,
    target_id UUID NOT NULL REFERENCES index_nodes(id) ON DELETE CASCADE,
    dep_type TEXT DEFAULT 'import' CHECK (dep_type IN ('import','extend','implement'))
);
CREATE INDEX idx_dep_edges_repo ON dep_edges(repo_id);

CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_id UUID NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('index','reindex')),
    status TEXT DEFAULT 'pending' CHECK (status IN ('pending','processing','completed','failed')),
    progress INT DEFAULT 0,
    error TEXT DEFAULT '',
    result TEXT DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_tasks_repo ON tasks(repo_id);
CREATE INDEX idx_tasks_status ON tasks(status);

CREATE TABLE conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    repo_id UUID NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    title TEXT DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_conv_user ON conversations(user_id);

CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conv_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('user','assistant')),
    content TEXT DEFAULT '',
    citations JSONB DEFAULT '[]',
    confidence FLOAT DEFAULT 0,
    tokens INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_messages_conv ON messages(conv_id);
