-- RAG store: embeddings live next to app data (pgvector), no extra infra
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS documents (
    id BIGSERIAL PRIMARY KEY,
    collection TEXT NOT NULL DEFAULT 'support', -- 'support' | 'chat'
    title TEXT,
    content TEXT NOT NULL,
    embedding vector(768) NOT NULL,             -- bge-base-en-v1.5 = 768 dims
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS documents_embedding_idx
    ON documents USING hnsw (embedding vector_cosine_ops);
