create extension if not exists pgcrypto;

create table if not exists users (
    id uuid primary key default gen_random_uuid(),
    subject text not null unique,
    email text not null,
    name text not null,
    is_admin boolean not null default false,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

alter table if exists users
    add column if not exists is_admin boolean default false;

update users
set is_admin = false
where is_admin is null;

alter table if exists users
    alter column is_admin set default false;

alter table if exists users
    alter column is_admin set not null;

create table if not exists teams (
    id uuid primary key default gen_random_uuid(),
    name text not null unique,
    description text not null default '',
    created_at timestamptz not null default now()
);

alter table if exists teams
    add column if not exists description text default '';

update teams
set description = ''
where description is null;

alter table if exists teams
    alter column description set default '';

alter table if exists teams
    alter column description set not null;

alter table if exists teams
    add column if not exists is_personal boolean not null default false;

alter table if exists teams
    add column if not exists personal_for_user_id uuid references users(id) on delete cascade;

create unique index if not exists idx_teams_personal_user
    on teams (personal_for_user_id)
    where personal_for_user_id is not null;

create table if not exists team_invitations (
    id uuid primary key default gen_random_uuid(),
    team_id uuid not null references teams(id) on delete cascade,
    email text not null,
    role text not null check (role in ('editor', 'viewer')),
    token_hash text not null unique,
    expires_at timestamptz not null,
    created_by_user_id uuid not null references users(id) on delete cascade,
    created_at timestamptz not null default now()
);

create index if not exists idx_team_invitations_team on team_invitations(team_id);

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

create table if not exists provider_instances (
    id uuid primary key default gen_random_uuid(),
    provider text not null check (provider in ('bluesky', 'friendica', 'mastodon')),
    name text not null,
    instance_url text not null,
    client_id text not null default '',
    client_secret_ciphertext text not null default '',
    scopes text[] not null default '{}',
    authorization_endpoint text not null default '',
    token_endpoint text not null default '',
    created_by_user_id uuid not null references users(id) on delete restrict,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (provider, instance_url)
);

create table if not exists social_accounts (
    id uuid primary key default gen_random_uuid(),
    team_id uuid not null references teams(id) on delete cascade,
    provider text not null check (provider in ('bluesky', 'friendica', 'mastodon')),
    auth_type text not null default 'oauth_token' check (auth_type in ('oauth_token', 'app_password')),
    provider_instance_id uuid references provider_instances(id) on delete set null,
    instance_url text not null,
    username text not null,
    remote_account_id text not null default '',
    avatar_url text not null default '',
    access_token_ciphertext text not null,
    refresh_token_ciphertext text not null default '',
    max_chars_override integer,
    created_at timestamptz not null default now()
);

alter table if exists social_accounts
    add column if not exists auth_type text default 'oauth_token';

update social_accounts
set auth_type = 'oauth_token'
where auth_type is null;

alter table if exists social_accounts
    alter column auth_type set default 'oauth_token';

alter table if exists social_accounts
    alter column auth_type set not null;

alter table if exists social_accounts
    add column if not exists provider_instance_id uuid references provider_instances(id) on delete set null;

alter table if exists social_accounts
    add column if not exists avatar_url text not null default '';

create table if not exists scheduled_posts (
    id uuid primary key default gen_random_uuid(),
    team_id uuid not null references teams(id) on delete cascade,
    author_user_id uuid not null references users(id) on delete restrict,
    title text not null default '',
    content text not null,
    scheduled_at timestamptz not null,
    status text not null check (status in ('pending', 'processing', 'posted', 'failed', 'cancelled', 'draft')),
    attempt_count integer not null default 0,
    last_error text,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

alter table if exists scheduled_posts
    add column if not exists title text default '';

update scheduled_posts
set title = ''
where title is null;

alter table if exists scheduled_posts
    alter column title set default '';

alter table if exists scheduled_posts
    alter column title set not null;

create table if not exists scheduled_post_targets (
    post_id uuid not null references scheduled_posts(id) on delete cascade,
    account_id uuid not null references social_accounts(id) on delete cascade,
    status text not null check (status in ('pending', 'processing', 'posted', 'failed', 'cancelled')),
    published_url text,
    last_error text,
    primary key (post_id, account_id)
);

create index if not exists idx_social_accounts_team on social_accounts(team_id);
create index if not exists idx_provider_instances_provider on provider_instances(provider, instance_url);
create index if not exists idx_scheduled_posts_due on scheduled_posts(status, scheduled_at);
create index if not exists idx_post_targets_post on scheduled_post_targets(post_id);

create table if not exists post_metrics (
    post_id uuid not null references scheduled_posts(id) on delete cascade,
    account_id uuid not null references social_accounts(id) on delete cascade,
    metric text not null,
    value bigint not null default 0,
    updated_at timestamptz not null default now(),
    primary key (post_id, account_id, metric)
);

create index if not exists idx_post_metrics_post on post_metrics(post_id);

create table if not exists post_metrics_history (
    post_id uuid not null references scheduled_posts(id) on delete cascade,
    account_id uuid not null references social_accounts(id) on delete cascade,
    metric text not null,
    value bigint not null default 0,
    recorded_at date not null default ((now() at time zone 'utc')::date),
    primary key (post_id, account_id, metric, recorded_at)
);

create index if not exists idx_post_metrics_history_post on post_metrics_history(post_id);
create index if not exists idx_post_metrics_history_recorded on post_metrics_history(recorded_at);

create table if not exists post_versions (
    post_id uuid not null references scheduled_posts(id) on delete cascade,
    account_id uuid not null references social_accounts(id) on delete cascade,
    content text not null default '',
    primary key (post_id, account_id)
);

alter table social_accounts add column if not exists access_token_expires_at timestamptz;

alter table scheduled_posts add column if not exists visibility text not null default 'public';
alter table scheduled_posts add column if not exists media_ids text not null default '[]';

alter table scheduled_posts add column if not exists media_exclude_by_account text not null default '{}';

alter table scheduled_post_targets add column if not exists publish_metadata text not null default '{}';
alter table scheduled_post_targets add column if not exists metrics_last_sync_date text;

alter table scheduled_posts drop constraint if exists scheduled_posts_status_check;
alter table scheduled_posts add constraint scheduled_posts_status_check
    check (status in ('pending', 'processing', 'posted', 'failed', 'cancelled', 'draft'));

-- Media library (must match db/schema.sql for Docker init parity)
create table if not exists media_items (
    id uuid primary key default gen_random_uuid(),
    team_id uuid not null references teams(id) on delete cascade,
    sha256 text not null,
    filename text not null,
    mime_type text not null,
    size_bytes bigint not null,
    width integer,
    height integer,
    created_at timestamptz not null default now()
);

create index if not exists idx_media_items_team on media_items(team_id);
create index if not exists idx_media_items_sha256 on media_items(sha256);
create unique index if not exists ux_media_items_team_sha256 on media_items(team_id, sha256);

create table if not exists media_provider_mappings (
    media_id uuid not null references media_items(id) on delete cascade,
    account_id uuid not null references social_accounts(id) on delete cascade,
    remote_id text not null,
    expires_at timestamptz,
    created_at timestamptz not null default now(),
    primary key (media_id, account_id)
);

create index if not exists idx_media_mappings_account on media_provider_mappings(account_id);
