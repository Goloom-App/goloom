create table if not exists users (
    id text primary key,
    subject text not null unique,
    email text not null,
    name text not null,
    is_admin integer not null default 0,
    created_at text not null,
    updated_at text not null
);

create table if not exists teams (
    id text primary key,
    name text not null unique,
    description text not null default '',
    is_personal integer not null default 0,
    personal_for_user_id text references users(id) on delete cascade,
    created_at text not null
);

create unique index if not exists idx_teams_personal_user on teams(personal_for_user_id)
    where personal_for_user_id is not null;

create table if not exists team_invitations (
    id text primary key,
    team_id text not null references teams(id) on delete cascade,
    email text not null,
    role text not null check (role in ('editor', 'viewer')),
    token_hash text not null unique,
    expires_at text not null,
    created_by_user_id text not null references users(id) on delete cascade,
    created_at text not null
);

create index if not exists idx_team_invitations_team on team_invitations(team_id);

create table if not exists team_memberships (
    user_id text not null references users(id) on delete cascade,
    team_id text not null references teams(id) on delete cascade,
    role text not null check (role in ('owner', 'editor', 'viewer')),
    created_at text not null,
    primary key (user_id, team_id)
);

create table if not exists api_tokens (
    id text primary key,
    user_id text not null references users(id) on delete cascade,
    name text not null,
    token_hash text not null unique,
    last_used_at text,
    expires_at text,
    created_at text not null
);

create table if not exists provider_instances (
    id text primary key,
    provider text not null check (provider in ('bluesky', 'friendica', 'mastodon')),
    name text not null,
    instance_url text not null,
    client_id text not null default '',
    client_secret_ciphertext text not null default '',
    scopes_json text not null default '[]',
    authorization_endpoint text not null default '',
    token_endpoint text not null default '',
    created_by_user_id text not null references users(id) on delete restrict,
    created_at text not null,
    updated_at text not null,
    unique (provider, instance_url)
);

create table if not exists social_accounts (
    id text primary key,
    team_id text not null references teams(id) on delete cascade,
    provider text not null check (provider in ('bluesky', 'friendica', 'mastodon')),
    auth_type text not null default 'oauth_token' check (auth_type in ('oauth_token', 'app_password')),
    provider_instance_id text references provider_instances(id) on delete set null,
    instance_url text not null,
    username text not null,
    remote_account_id text not null default '',
    avatar_url text not null default '',
    access_token_ciphertext text not null,
    refresh_token_ciphertext text not null default '',
    max_chars_override integer,
    access_token_expires_at text,
    created_at text not null
);

create table if not exists scheduled_posts (
    id text primary key,
    team_id text not null references teams(id) on delete cascade,
    author_user_id text not null references users(id) on delete restrict,
    title text not null default '',
    content text not null,
    scheduled_at text not null,
    status text not null check (status in ('pending', 'processing', 'posted', 'failed', 'cancelled', 'draft')),
    attempt_count integer not null default 0,
    last_error text,
    visibility text not null default 'public',
    media_ids text not null default '[]',
    created_at text not null,
    updated_at text not null
);

create table if not exists scheduled_post_targets (
    post_id text not null references scheduled_posts(id) on delete cascade,
    account_id text not null references social_accounts(id) on delete cascade,
    status text not null check (status in ('pending', 'processing', 'posted', 'failed', 'cancelled')),
    published_url text,
    last_error text,
    publish_metadata text not null default '{}',
    metrics_last_sync_date text,
    primary key (post_id, account_id)
);

create index if not exists idx_social_accounts_team on social_accounts(team_id);
create index if not exists idx_provider_instances_provider on provider_instances(provider, instance_url);
create index if not exists idx_scheduled_posts_due on scheduled_posts(status, scheduled_at);
create index if not exists idx_post_targets_post on scheduled_post_targets(post_id);

create table if not exists post_metrics (
    post_id text not null references scheduled_posts(id) on delete cascade,
    account_id text not null references social_accounts(id) on delete cascade,
    metric text not null,
    value integer not null default 0,
    updated_at text not null,
    primary key (post_id, account_id, metric)
);

create index if not exists idx_post_metrics_post on post_metrics(post_id);

create table if not exists post_metrics_history (
    post_id text not null references scheduled_posts(id) on delete cascade,
    account_id text not null references social_accounts(id) on delete cascade,
    metric text not null,
    value integer not null default 0,
    recorded_at text not null,
    primary key (post_id, account_id, metric, recorded_at)
);

create index if not exists idx_post_metrics_history_post on post_metrics_history(post_id);
create index if not exists idx_post_metrics_history_recorded on post_metrics_history(recorded_at);

create table if not exists post_versions (
    post_id text not null references scheduled_posts(id) on delete cascade,
    account_id text not null references social_accounts(id) on delete cascade,
    content text not null default '',
    primary key (post_id, account_id)
);
