-- SSO Providers configuration
CREATE TABLE IF NOT EXISTS sso_providers (
    id SERIAL PRIMARY KEY,
    provider_type TEXT NOT NULL,
    name TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT false,
    
    -- OAuth2/OIDC settings
    client_id TEXT NOT NULL DEFAULT '',
    client_secret TEXT NOT NULL DEFAULT '',
    auth_url TEXT NOT NULL DEFAULT '',
    token_url TEXT NOT NULL DEFAULT '',
    issuer_url TEXT NOT NULL DEFAULT '',
    scopes TEXT NOT NULL DEFAULT 'email,profile',
    
    -- SAML settings
    idp_metadata_url TEXT NOT NULL DEFAULT '',
    idp_metadata TEXT NOT NULL DEFAULT '',
    sp_entity_id TEXT NOT NULL DEFAULT '',
    
    -- Access control
    allowed_domains TEXT NOT NULL DEFAULT '',
    default_role TEXT NOT NULL DEFAULT 'viewer',
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    UNIQUE(provider_type)
);

-- SSO Users
CREATE TABLE IF NOT EXISTS sso_users (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL DEFAULT '',
    avatar_url TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'viewer',
    groups TEXT[] DEFAULT '{}',
    last_login_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sso_users_email ON sso_users(email);
CREATE INDEX IF NOT EXISTS idx_sso_users_provider ON sso_users(provider, provider_id);

-- SSO Sessions
CREATE TABLE IF NOT EXISTS sso_sessions (
    token TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES sso_users(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    provider TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sso_sessions_user ON sso_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sso_sessions_expires ON sso_sessions(expires_at)
