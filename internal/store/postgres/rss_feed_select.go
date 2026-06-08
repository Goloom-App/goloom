package postgres

// rssFeedConfigSelectList is the column list every rss_feed_configs SELECT passed
// to scanRSSFeedConfig must include, in this order.
const rssFeedConfigSelectList = `
		id, team_id, feed_url, name, is_active, ai_enhance_enabled, content_template, output_mode, max_posts_per_day, counter_next,
		prompt_hint, target_account_ids, tonality, initial_sync_mode, last_fetched_at, created_at`
