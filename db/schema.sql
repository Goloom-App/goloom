create extension if not exists pgcrypto;

create table if not exists users (
    id uuid primary key default gen_random_uuid(),
    subject text not null unique,
    email text not null,
    name text not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists teams (
    id uuid primary key default gen_random_uuid(),
    name text not null unique,
    created_at timestamptz not null default now()
);

create table if not exists team_memberships (
    user_id uuid not null references users(id) on delete cascade,
    team_id uuid not null references teams(id) on delete cascade,
    role text not null check (role in ('owner', 'editor', 'viewer')),
    created_at timestamptz not null default now(),
    primary key (user_id, team_id)
);

create table if not exists api_tokens (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    name text not null,
    token_hash text not null unique,
    last_used_at timestamptz,
    expires_at timestamptz,
    created_at timestamptz not null default now()
);

create table if not exists social_accounts (
    id uuid primary key default gen_random_uuid(),
    team_id uuid not null references teams(id) on delete cascade,
    provider text not null check (provider in ('bluesky', 'friendica', 'mastodon')),
    instance_url text not null,
    username text not null,
    remote_account_id text not null default '',
    access_token_ciphertext text not null,
    refresh_token_ciphertext text not null default '',
    max_chars_override integer,
    created_at timestamptz not null default now()
);

create table if not exists scheduled_posts (
    id uuid primary key default gen_random_uuid(),
    team_id uuid not null references teams(id) on delete cascade,
    author_user_id uuid not null references users(id) on delete restrict,
    content text not null,
    scheduled_at timestamptz not null,
    status text not null check (status in ('pending', 'processing', 'posted', 'failed', 'cancelled')),
    attempt_count integer not null default 0,
    last_error text,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists scheduled_post_targets (
    post_id uuid not null references scheduled_posts(id) on delete cascade,
    account_id uuid not null references social_accounts(id) on delete cascade,
    status text not null check (status in ('pending', 'processing', 'posted', 'failed', 'cancelled')),
    published_url text,
    last_error text,
    primary key (post_id, account_id)
);

create index if not exists idx_social_accounts_team on social_accounts(team_id);
create index if not exists idx_scheduled_posts_due on scheduled_posts(status, scheduled_at);
create index if not exists idx_post_targets_post on scheduled_post_targets(post_id);
