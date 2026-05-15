-- ============================================================
-- Миграция 001: Полная схема БД
-- Таблицы: agents, agent_registrations, tickets, a2a_calls,
--          knowledge_base (заготовка), dispatcher_configs
-- ============================================================

-- 0. Диспетчерские
CREATE TABLE IF NOT EXISTS dispatchers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    api_key VARCHAR(255) NOT NULL,
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE UNIQUE INDEX idx_dispatchers_api_key ON dispatchers(api_key);
CREATE INDEX idx_dispatchers_status ON dispatchers(status);

-- 1. Агенты
CREATE TABLE IF NOT EXISTS agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dispatcher_id UUID REFERENCES dispatchers(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    endpoint VARCHAR(512) NOT NULL,
    agent_type VARCHAR(100) NOT NULL,
    capabilities JSONB DEFAULT '[]'::jsonb,
    skills JSONB DEFAULT '[]'::jsonb,
    status VARCHAR(50) DEFAULT 'pending',
    auth_token VARCHAR(512),
    last_heartbeat TIMESTAMP WITH TIME ZONE,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_agents_dispatcher ON agents(dispatcher_id);
CREATE INDEX idx_agents_status ON agents(status);
CREATE INDEX idx_agents_type ON agents(agent_type);

-- 2. Журнал регистраций агентов
CREATE TABLE IF NOT EXISTS agent_registrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
    event_type VARCHAR(50) NOT NULL,
    source_ip VARCHAR(45),
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_agent_reg_agent ON agent_registrations(agent_id);
CREATE INDEX idx_agent_reg_time ON agent_registrations(created_at);

-- 3. Тикеты
CREATE TABLE IF NOT EXISTS tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dispatcher_id UUID REFERENCES dispatchers(id) ON DELETE SET NULL,
    external_id VARCHAR(255),
    text TEXT NOT NULL,
    channel VARCHAR(50) DEFAULT 'api',
    classification JSONB,
    embedding JSONB,
    final_response TEXT,
    plan TEXT,
    execution_log JSONB DEFAULT '[]'::jsonb,
    agents_used JSONB DEFAULT '[]'::jsonb,
    status VARCHAR(50) DEFAULT 'received',
    confidence DOUBLE PRECISION,
    suggested_team VARCHAR(100),
    metadata JSONB DEFAULT '{}'::jsonb,
    processing_time_ms INTEGER,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_tickets_dispatcher ON tickets(dispatcher_id);
CREATE INDEX idx_tickets_status ON tickets(status);
CREATE INDEX idx_tickets_created ON tickets(created_at DESC);
CREATE INDEX idx_tickets_external ON tickets(dispatcher_id, external_id);

-- 4. Журнал A2A вызовов
CREATE TABLE IF NOT EXISTS a2a_calls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id UUID REFERENCES tickets(id) ON DELETE SET NULL,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    skill_id VARCHAR(100),
    request JSONB,
    response JSONB,
    task_id VARCHAR(255),
    status VARCHAR(50),
    error TEXT,
    duration_ms INTEGER,
    retry_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_a2a_ticket ON a2a_calls(ticket_id);
CREATE INDEX idx_a2a_agent ON a2a_calls(agent_id);
CREATE INDEX idx_a2a_time ON a2a_calls(created_at DESC);

-- 5. Заготовка базы знаний (для RAG)
CREATE TABLE IF NOT EXISTS knowledge_base (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dispatcher_id UUID REFERENCES dispatchers(id) ON DELETE CASCADE,
    title VARCHAR(500),
    content TEXT NOT NULL,
    category VARCHAR(100),
    tags JSONB DEFAULT '[]'::jsonb,
    source VARCHAR(50),
    qdrant_point_id UUID,
    embedded BOOLEAN DEFAULT false,
    usage_count INTEGER DEFAULT 0,
    success_rate DECIMAL(3,2),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_kb_dispatcher ON knowledge_base(dispatcher_id);
CREATE INDEX idx_kb_category ON knowledge_base(category);

-- 6. История конфигураций диспетчерских
CREATE TABLE IF NOT EXISTS dispatcher_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dispatcher_id UUID REFERENCES dispatchers(id) ON DELETE CASCADE,
    config JSONB NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    comment TEXT,
    changed_by VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_dc_dispatcher ON dispatcher_configs(dispatcher_id);
CREATE INDEX idx_dc_version ON dispatcher_configs(dispatcher_id, version DESC);

-- ============================================================
-- Seed: системные агенты
-- ============================================================
INSERT INTO agents (name, endpoint, agent_type, capabilities, skills, status, metadata) VALUES
(
    'classifier',
    'http://100.87.189.74:9001',
    'classifier',
    '["classification"]'::jsonb,
    '[
        {"id": "classify", "description": "Classify user request into problem category with confidence score"},
        {"id": "extract_entities", "description": "Extract named entities from text"}
    ]'::jsonb,
    'online',
    '{"model": "rubert-tiny2"}'::jsonb
),
(
    'llm-generator',
    'http://100.87.189.74:9003',
    'generator',
    '["generation"]'::jsonb,
    '[
        {"id": "generate_response", "description": "Generate final answer based on classification and solutions"},
        {"id": "format_response", "description": "Format and adjust tone of existing answer"}
    ]'::jsonb,
    'online',
    '{"model": "saiga-llama"}'::jsonb
),
(
    'researcher',
    'http://100.87.189.74:9002',
    'researcher',
    '["search"]'::jsonb,
    '[
        {"id": "search", "description": "Search knowledge base and internet for solutions"},
        {"id": "search_knowledge_base", "description": "Search only local knowledge base"}
    ]'::jsonb,
    'online',
    '{"type": "mock"}'::jsonb
);