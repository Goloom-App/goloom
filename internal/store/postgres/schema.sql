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
    provider text not null check (provider <> ''),
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
    name text not null default '',
    provider text not null check (provider <> ''),
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

-- Relax the provider CHECK constraints so new Mastodon-compatible providers (e.g. pixelfed)
-- are accepted; the application's provider registry is the source of truth for valid names.
alter table if exists provider_instances
    drop constraint if exists provider_instances_provider_check;
alter table if exists provider_instances
    add constraint provider_instances_provider_check check (provider <> '');

alter table if exists social_accounts
    drop constraint if exists social_accounts_provider_check;
alter table if exists social_accounts
    add constraint social_accounts_provider_check check (provider <> '');

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
    add column if not exists name text not null default '';

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
    publish_metadata text not null default '{}',
    remote_post_id text,
    metrics_last_sync_date text,
    metrics_last_sync_at timestamptz,
    primary key (post_id, account_id)
);

create index if not exists idx_social_accounts_team on social_accounts(team_id);
create index if not exists idx_provider_instances_provider on provider_instances(provider, instance_url);
create index if not exists idx_scheduled_posts_due on scheduled_posts(status, scheduled_at);
create index if not exists idx_post_targets_post on scheduled_post_targets(post_id);
create index if not exists idx_post_targets_metrics_sync on scheduled_post_targets(metrics_last_sync_at);

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

create table if not exists post_hashtags (
    post_id uuid not null references scheduled_posts(id) on delete cascade,
    account_id uuid not null references social_accounts(id) on delete cascade,
    tag_norm text not null,
    tag_display text not null default '',
    primary key (post_id, account_id, tag_norm)
);

create index if not exists idx_post_hashtags_tag on post_hashtags(tag_norm);

create table if not exists account_metrics (
    account_id uuid not null references social_accounts(id) on delete cascade,
    metric text not null,
    value bigint not null default 0,
    updated_at timestamptz not null default now(),
    primary key (account_id, metric)
);

create table if not exists account_metrics_history (
    account_id uuid not null references social_accounts(id) on delete cascade,
    metric text not null,
    value bigint not null default 0,
    recorded_at date not null,
    primary key (account_id, metric, recorded_at)
);
create index if not exists idx_account_metrics_history_recorded on account_metrics_history(recorded_at);

create table if not exists post_versions (
    post_id uuid not null references scheduled_posts(id) on delete cascade,
    account_id uuid not null references social_accounts(id) on delete cascade,
    content text not null default '',
    primary key (post_id, account_id)
);

create table if not exists team_profiles (
    id uuid primary key default gen_random_uuid(),
    team_id uuid not null references teams(id) on delete cascade,
    style_metadata jsonb not null default '{}',
    auto_publish_enabled boolean not null default false,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists campaign_formats (
    id uuid primary key default gen_random_uuid(),
    team_id uuid not null references teams(id) on delete cascade,
    name text not null,
    weekday smallint,
    structure jsonb not null default '{}',
    required_hashtags text[] not null default '{}',
    is_active boolean not null default true,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists style_examples (
    id uuid primary key default gen_random_uuid(),
    team_id uuid not null references teams(id) on delete cascade,
    platform text not null,
    content text not null,
    notes text not null default '',
    created_at timestamptz not null default now()
);

create table if not exists knowledge_sources (
    id uuid primary key default gen_random_uuid(),
    team_id uuid not null references teams(id) on delete cascade,
    source_type text not null check (source_type in ('text', 'url', 'file')),
    name text not null,
    content text not null default '',
    source_url text not null default '',
    media_id text,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists ai_jobs (
    id uuid primary key default gen_random_uuid(),
    team_id uuid not null references teams(id) on delete cascade,
    author_user_id uuid not null references users(id),
    job_type text not null,
    status text not null default 'pending',
    payload jsonb not null default '{}',
    result jsonb,
    error_message text,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    completed_at timestamptz
);

create table if not exists ai_service_configs (
    id uuid primary key default gen_random_uuid(),
    team_id uuid references teams(id) on delete cascade,
    provider text not null default 'openai',
    model text not null default '',
    base_url text not null default '',
    api_key_ciphertext text not null default '',
    description text not null default '',
    created_at timestamptz not null default now()
);

alter table ai_service_configs add column if not exists provider text not null default 'openai';
alter table ai_service_configs add column if not exists model text not null default '';
alter table ai_service_configs add column if not exists base_url text not null default '';
alter table ai_service_configs add column if not exists api_key_ciphertext text not null default '';
alter table ai_service_configs drop column if exists service_url;

create table if not exists rss_feed_configs (
    id uuid primary key default gen_random_uuid(),
    team_id uuid not null references teams(id) on delete cascade,
    feed_url text not null,
    name text not null,
    is_active boolean not null default true,
    prompt_hint text not null default '',
    target_account_ids text not null default '[]',
    tonality text not null default '',
    initial_sync_mode text not null default 'baseline',
    last_fetched_at timestamptz,
    created_at timestamptz not null default now()
);

create table if not exists proactive_trigger_settings (
    id uuid primary key default gen_random_uuid(),
    team_id uuid not null references teams(id) on delete cascade unique,
    content_gap_threshold_days integer not null default 3,
    auto_fill_enabled boolean not null default false,
    max_triggers_per_day integer not null default 5,
    cron_schedule text not null default '0 9 * * *',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index if not exists idx_team_profiles_team on team_profiles(team_id);
create unique index if not exists idx_team_profiles_team_unique on team_profiles(team_id);
create index if not exists idx_campaign_formats_team on campaign_formats(team_id);
create index if not exists idx_style_examples_team on style_examples(team_id);
create index if not exists idx_knowledge_sources_team on knowledge_sources(team_id);
create index if not exists idx_ai_jobs_team_status on ai_jobs(team_id, status);
create index if not exists idx_ai_jobs_status on ai_jobs(status);
create index if not exists idx_rss_feed_configs_team on rss_feed_configs(team_id);

alter table social_accounts add column if not exists access_token_expires_at timestamptz;

alter table scheduled_posts add column if not exists visibility text not null default 'public';
alter table scheduled_posts add column if not exists media_ids text not null default '[]';

alter table scheduled_posts add column if not exists media_exclude_by_account text not null default '{}';

alter table scheduled_post_targets add column if not exists publish_metadata text not null default '{}';
alter table scheduled_post_targets add column if not exists metrics_last_sync_date text;
alter table scheduled_post_targets add column if not exists metrics_last_sync_at timestamptz;

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

alter table teams add column if not exists scheduling_prefs text not null default '{}';

create table if not exists post_templates (
    id uuid primary key default gen_random_uuid(),
    team_id uuid not null references teams(id) on delete cascade,
    author_user_id uuid not null references users(id) on delete restrict,
    title text not null default '',
    content text not null,
    recurrence_json text not null,
    visibility text not null default 'public',
    media_ids text not null default '[]',
    media_exclude_by_account text not null default '{}',
    target_account_ids text not null default '[]',
    enabled boolean not null default true,
    next_materialize_at timestamptz,
    counter_next integer not null default 1,
    announces_template_id uuid references post_templates(id) on delete set null,
    announcement_days_before integer,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index if not exists idx_post_templates_due on post_templates (enabled, next_materialize_at);

alter table post_templates add column if not exists announces_template_id uuid references post_templates(id) on delete set null;
alter table post_templates add column if not exists announcement_days_before integer;

alter table post_templates add column if not exists ai_enhance_enabled boolean not null default false;
alter table post_templates add column if not exists ai_enhance_announcement boolean not null default false;
alter table post_templates add column if not exists output_mode text not null default 'scheduled';
alter table post_templates add column if not exists prompt_hint text not null default '';
alter table post_templates add column if not exists tonality text not null default '';
alter table post_templates add column if not exists title_hint text not null default '';
alter table post_templates add column if not exists materialize_horizon_days integer not null default 0;

alter table post_templates add column if not exists announcement_enabled boolean not null default false;
alter table post_templates add column if not exists announcement_title text not null default '';
alter table post_templates add column if not exists announcement_content text not null default '';
alter table post_templates add column if not exists announcement_counter_next integer not null default 1;
alter table post_templates add column if not exists announcement_target_account_ids text not null default '[]';

update post_templates as parent
set announcement_enabled = true,
    announcement_title = child.title,
    announcement_content = child.content,
    announcement_days_before = coalesce(child.announcement_days_before, 2),
    announcement_counter_next = child.counter_next,
    announcement_target_account_ids = child.target_account_ids,
    updated_at = now()
from post_templates as child
where child.announces_template_id = parent.id;

update post_templates as child
set announces_template_id = null, updated_at = now()
where child.announces_template_id is not null
  and not exists (
    select 1 from post_templates as parent where parent.id = child.announces_template_id
  );

delete from post_templates where announces_template_id is not null;

create table if not exists post_template_skips (
    template_id uuid not null references post_templates(id) on delete cascade,
    occurrence_at timestamptz not null,
    shift_to timestamptz,
    primary key (template_id, occurrence_at)
);

alter table scheduled_posts add column if not exists post_template_id uuid references post_templates(id) on delete set null;
alter table scheduled_posts add column if not exists template_counter integer;
alter table scheduled_posts add column if not exists template_occurrence_at timestamptz;
alter table scheduled_posts add column if not exists template_post_role text not null default '';

alter table post_template_skips add column if not exists skip_scope text not null default 'occurrence';

create table if not exists log_entries (
    id uuid primary key default gen_random_uuid(),
    level text not null,
    message text not null,
    attributes_json jsonb not null default '{}',
    source_file text not null default '',
    source_line integer not null default 0,
    created_at timestamptz not null default now(),
    archived_at timestamptz
);

create table if not exists audit_events (
    id uuid primary key default gen_random_uuid(),
    team_id uuid not null references teams(id) on delete cascade,
    actor_user_id text not null default '',
    actor_name text not null default '',
    actor_email text not null default '',
    actor_kind text not null default '',
    token_id text,
    token_name text,
    action text not null,
    target_type text not null default '',
    target_id text,
    summary text not null default '',
    metadata_json jsonb not null default '{}',
    created_at timestamptz not null default now()
);
create index if not exists idx_audit_events_team on audit_events(team_id, created_at desc);

create table if not exists job_locks (
    lock_id text primary key,
    locked_at timestamptz not null default now(),
    expires_at timestamptz not null
);

alter table teams add column if not exists is_ai_enabled boolean not null default false;
alter table teams add column if not exists brand_color text not null default '';
alter table api_tokens add column if not exists scopes text not null default '';
alter table api_tokens add column if not exists team_id uuid references teams(id) on delete cascade;
alter table api_tokens add column if not exists description text not null default '';

create table if not exists external_post_monitor_settings (
    id uuid primary key default gen_random_uuid(),
    team_id uuid not null references teams(id) on delete cascade unique,
    enabled boolean not null default false,
    backfill_completed_at timestamptz,
    last_sync_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

alter table scheduled_posts add column if not exists source text not null default 'scheduled';

alter table scheduled_post_targets add column if not exists remote_post_id text;
create unique index if not exists ux_post_targets_account_remote_post
    on scheduled_post_targets(account_id, remote_post_id)
    where remote_post_id is not null and trim(remote_post_id) <> '';

update scheduled_post_targets
set remote_post_id = nullif(trim(publish_metadata::json->>'uri'), '')
where status = 'posted'
  and (remote_post_id is null or trim(remote_post_id) = '')
  and publish_metadata is not null
  and trim(publish_metadata) <> '{}'
  and nullif(trim(publish_metadata::json->>'uri'), '') is not null;

alter table rss_feed_configs add column if not exists prompt_hint text not null default '';
alter table rss_feed_configs add column if not exists target_account_ids text not null default '[]';
alter table rss_feed_configs add column if not exists tonality text not null default '';
alter table rss_feed_configs add column if not exists initial_sync_mode text not null default 'baseline';
alter table rss_feed_configs add column if not exists content_template text not null default '{title}

{link}';
alter table rss_feed_configs add column if not exists title_template text not null default '{title}';
alter table rss_feed_configs add column if not exists title_hint text not null default '';
alter table rss_feed_configs add column if not exists output_mode text not null default 'draft';
alter table rss_feed_configs add column if not exists max_posts_per_day integer not null default 10;
alter table rss_feed_configs add column if not exists counter_next integer not null default 1;
alter table rss_feed_configs add column if not exists ai_enhance_enabled boolean not null default false;

alter table scheduled_posts add column if not exists rss_feed_id uuid references rss_feed_configs(id) on delete set null;

update scheduled_posts
set source = 'scheduled'
where trim(source) = '' or source not in ('scheduled', 'imported', 'automation');

alter table scheduled_posts drop constraint if exists scheduled_posts_source_check;
alter table scheduled_posts add constraint scheduled_posts_source_check
    check (source in ('scheduled', 'imported', 'automation'));

create table if not exists rss_imported_items (
    id uuid primary key default gen_random_uuid(),
    feed_id uuid not null references rss_feed_configs(id) on delete cascade,
    item_key text not null,
    post_id uuid references scheduled_posts(id) on delete set null,
    created_at timestamptz not null default now(),
    unique (feed_id, item_key)
);

create index if not exists idx_rss_imported_items_feed on rss_imported_items(feed_id);
