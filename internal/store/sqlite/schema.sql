create table if not exists users (
    id text primary key,
    subject text not null unique,
    email text not null,
    name text not null,
    is_admin integer not null default 0,
    tour_done integer not null default 0,
    created_at text not null,
    updated_at text not null
);

create table if not exists teams (
    id text primary key,
    name text not null unique,
    description text not null default '',
    is_personal integer not null default 0,
    personal_for_user_id text references users(id) on delete cascade,
    is_ai_enabled integer not null default 0,
    brand_color text not null default '',
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
    scopes text not null default '',
    description text not null default '',
    team_id text references teams(id) on delete cascade,
    created_at text not null
);

create table if not exists team_profiles (
    id text primary key,
    team_id text not null references teams(id) on delete cascade,
    style_metadata text not null default '{}',
    auto_publish_enabled integer not null default 0,
    created_at text not null default (datetime('now')),
    updated_at text not null default (datetime('now'))
);

create table if not exists campaign_formats (
    id text primary key,
    team_id text not null references teams(id) on delete cascade,
    name text not null,
    weekday integer,
    structure text not null default '{}',
    required_hashtags text not null default '[]',
    is_active integer not null default 1,
    created_at text not null default (datetime('now')),
    updated_at text not null default (datetime('now'))
);

create table if not exists style_examples (
    id text primary key,
    team_id text not null references teams(id) on delete cascade,
    platform text not null,
    content text not null,
    notes text not null default '',
    created_at text not null default (datetime('now'))
);

create table if not exists knowledge_sources (
    id text primary key,
    team_id text not null references teams(id) on delete cascade,
    source_type text not null check (source_type in ('text', 'url', 'file')),
    name text not null,
    content text not null default '',
    source_url text not null default '',
    media_id text not null default '',
    created_at text not null default (datetime('now')),
    updated_at text not null default (datetime('now'))
);

create table if not exists ai_jobs (
    id text primary key,
    team_id text not null references teams(id) on delete cascade,
    author_user_id text not null references users(id),
    job_type text not null,
    status text not null default 'pending',
    payload text not null default '{}',
    result text,
    error_message text,
    created_at text not null default (datetime('now')),
    updated_at text not null default (datetime('now')),
    completed_at text
);

create table if not exists ai_service_configs (
    id text primary key,
    team_id text references teams(id) on delete cascade,
    provider text not null default 'openai',
    model text not null default '',
    base_url text not null default '',
    api_key_ciphertext text not null default '',
    description text not null default '',
    created_at text not null default (datetime('now'))
);

create table if not exists rss_feed_configs (
    id text primary key,
    team_id text not null references teams(id) on delete cascade,
    feed_url text not null,
    name text not null,
    is_active integer not null default 1,
    ai_enhance_enabled integer not null default 0,
    content_template text not null default '{title}

{link}',
    title_template text not null default '{title}',
    title_hint text not null default '',
    output_mode text not null default 'draft',
    max_posts_per_day integer not null default 10,
    counter_next integer not null default 1,
    prompt_hint text not null default '',
    target_account_ids text not null default '[]',
    tonality text not null default '',
    initial_sync_mode text not null default 'baseline',
    last_fetched_at text,
    created_at text not null default (datetime('now'))
);

create table if not exists rss_imported_items (
    id text primary key,
    feed_id text not null references rss_feed_configs(id) on delete cascade,
    item_key text not null,
    post_id text references scheduled_posts(id) on delete set null,
    created_at text not null default (datetime('now')),
    unique (feed_id, item_key)
);

create index if not exists idx_rss_imported_items_feed on rss_imported_items(feed_id);

create table if not exists proactive_trigger_settings (
    id text primary key,
    team_id text not null unique references teams(id) on delete cascade,
    content_gap_threshold_days integer not null default 3,
    auto_fill_enabled integer not null default 0,
    max_triggers_per_day integer not null default 5,
    cron_schedule text not null default '0 9 * * *',
    created_at text not null default (datetime('now')),
    updated_at text not null default (datetime('now'))
);

create index if not exists idx_team_profiles_team on team_profiles(team_id);
create unique index if not exists idx_team_profiles_team_unique on team_profiles(team_id);
create index if not exists idx_campaign_formats_team on campaign_formats(team_id);
create index if not exists idx_style_examples_team on style_examples(team_id);
create index if not exists idx_knowledge_sources_team on knowledge_sources(team_id);
create index if not exists idx_ai_jobs_team_status on ai_jobs(team_id, status);
create index if not exists idx_ai_jobs_status on ai_jobs(status);
create index if not exists idx_rss_feed_configs_team on rss_feed_configs(team_id);

create table if not exists provider_instances (
    id text primary key,
    provider text not null check (provider <> ''),
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
    name text not null default '',
    provider text not null check (provider <> ''),
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
    source text not null default 'scheduled' check (source in ('scheduled', 'imported', 'automation')),
    attempt_count integer not null default 0,
    last_error text,
    acknowledged_at text,
    visibility text not null default 'public',
    media_ids text not null default '[]',
    media_exclude_by_account text not null default '{}',
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
    remote_post_id text,
    metrics_last_sync_date text,
    metrics_last_sync_at text,
    primary key (post_id, account_id)
);

create unique index if not exists ux_post_targets_account_remote_post
    on scheduled_post_targets(account_id, remote_post_id)
    where remote_post_id is not null and trim(remote_post_id) <> '';

create table if not exists external_post_monitor_settings (
    id text primary key,
    team_id text not null references teams(id) on delete cascade unique,
    enabled integer not null default 0,
    backfill_completed_at text,
    last_sync_at text,
    created_at text not null,
    updated_at text not null
);

create index if not exists idx_social_accounts_team on social_accounts(team_id);
create index if not exists idx_provider_instances_provider on provider_instances(provider, instance_url);
create index if not exists idx_scheduled_posts_due on scheduled_posts(status, scheduled_at);
create index if not exists idx_post_targets_post on scheduled_post_targets(post_id);
create index if not exists idx_post_targets_metrics_sync on scheduled_post_targets(metrics_last_sync_at);

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

create table if not exists post_hashtags (
    post_id text not null references scheduled_posts(id) on delete cascade,
    account_id text not null references social_accounts(id) on delete cascade,
    tag_norm text not null,
    tag_display text not null default '',
    primary key (post_id, account_id, tag_norm)
);

create index if not exists idx_post_hashtags_tag on post_hashtags(tag_norm);

create table if not exists account_metrics (
    account_id text not null references social_accounts(id) on delete cascade,
    metric text not null,
    value integer not null default 0,
    updated_at text not null,
    primary key (account_id, metric)
);

create table if not exists account_metrics_history (
    account_id text not null references social_accounts(id) on delete cascade,
    metric text not null,
    value integer not null default 0,
    recorded_at text not null,
    primary key (account_id, metric, recorded_at)
);
create index if not exists idx_account_metrics_history_recorded on account_metrics_history(recorded_at);

create table if not exists post_versions (
    post_id text not null references scheduled_posts(id) on delete cascade,
    account_id text not null references social_accounts(id) on delete cascade,
    content text not null default '',
    primary key (post_id, account_id)
);

create table if not exists media_items (
    id text primary key,
    team_id text not null references teams(id) on delete cascade,
    sha256 text not null,
    filename text not null,
    mime_type text not null,
    size_bytes integer not null,
    width integer,
    height integer,
    created_at text not null
);

create index if not exists idx_media_items_team on media_items(team_id);
create index if not exists idx_media_items_sha256 on media_items(sha256);
create unique index if not exists ux_media_items_team_sha256 on media_items(team_id, sha256);

create table if not exists media_provider_mappings (
    media_id text not null references media_items(id) on delete cascade,
    account_id text not null references social_accounts(id) on delete cascade,
    remote_id text not null,
    expires_at text,
    created_at text not null,
    primary key (media_id, account_id)
);

create index if not exists idx_media_mappings_account on media_provider_mappings(account_id);

create table if not exists post_templates (
    id text primary key,
    team_id text not null references teams(id) on delete cascade,
    author_user_id text not null references users(id) on delete restrict,
    title text not null default '',
    content text not null,
    recurrence_json text not null,
    visibility text not null default 'public',
    media_ids text not null default '[]',
    media_exclude_by_account text not null default '{}',
    target_account_ids text not null default '[]',
    enabled integer not null default 1,
    ai_enhance_enabled integer not null default 0,
    ai_enhance_announcement integer not null default 0,
    output_mode text not null default 'scheduled',
    prompt_hint text not null default '',
    title_hint text not null default '',
    tonality text not null default '',
    materialize_horizon_days integer not null default 0,
    next_materialize_at text,
    counter_next integer not null default 1,
    announces_template_id text references post_templates(id) on delete set null,
    announcement_enabled integer not null default 0,
    announcement_title text not null default '',
    announcement_content text not null default '',
    announcement_days_before integer not null default 0,
    announcement_counter_next integer not null default 1,
    announcement_target_account_ids text not null default '[]',
    created_at text not null,
    updated_at text not null
);

create index if not exists idx_post_templates_due on post_templates (enabled, next_materialize_at);

create table if not exists post_template_skips (
    template_id text not null references post_templates(id) on delete cascade,
    occurrence_at text not null,
    shift_to text,
    primary key (template_id, occurrence_at)
);

create table if not exists log_entries (
    id text primary key,
    level text not null,
    message text not null,
    attributes_json text not null default '{}',
    source_file text not null default '',
    source_line integer not null default 0,
    created_at text not null,
    archived_at text
);

create table if not exists audit_events (
    id text primary key,
    team_id text not null references teams(id) on delete cascade,
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
    metadata_json text not null default '{}',
    created_at text not null
);
create index if not exists idx_audit_events_team on audit_events(team_id, created_at desc);

create table if not exists job_locks (
    lock_id text primary key,
    locked_at text not null,
    expires_at text not null
);
