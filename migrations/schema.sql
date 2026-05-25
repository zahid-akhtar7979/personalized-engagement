-- Real-Time Personalized Engagement Platform Schema

CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(255) UNIQUE NOT NULL,
    email VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS categories (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    parent_id BIGINT REFERENCES categories(id)
);

CREATE TABLE IF NOT EXISTS content_catalog (
    id BIGSERIAL PRIMARY KEY,
    item_id BIGINT UNIQUE NOT NULL,
    title VARCHAR(500) NOT NULL,
    category_id BIGINT,
    category VARCHAR(255),
    price DECIMAL(10,2) DEFAULT 0,
    image_url VARCHAR(500)
);

CREATE TABLE IF NOT EXISTS user_events (
    id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(64) UNIQUE NOT NULL,
    user_id BIGINT NOT NULL,
    item_id BIGINT,
    category_id BIGINT,
    event_type VARCHAR(32) NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_user_events_user_id ON user_events(user_id);
CREATE INDEX IF NOT EXISTS idx_user_events_event_type ON user_events(event_type);
CREATE INDEX IF NOT EXISTS idx_user_events_timestamp ON user_events(timestamp);

CREATE TABLE IF NOT EXISTS recommendations (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    item_id BIGINT NOT NULL,
    score DECIMAL(8,4),
    reason VARCHAR(255),
    section VARCHAR(64),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_recommendations_user_id ON recommendations(user_id);

CREATE TABLE IF NOT EXISTS analytics_metrics (
    id BIGSERIAL PRIMARY KEY,
    retention_rate DECIMAL(8,4),
    ctr DECIMAL(8,4),
    conversion_rate DECIMAL(8,4),
    engagement_score DECIMAL(12,2),
    roi_percentage DECIMAL(8,4),
    active_users BIGINT,
    recorded_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_analytics_metrics_recorded_at ON analytics_metrics(recorded_at);
