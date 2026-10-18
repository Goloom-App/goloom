package postgres

// rssFeedConfigSelectList is the column list every rss_feed_configs SELECT passed
// to scanRSSFeedConfig must include, in this order.
const rssFeedConfigSelectList = `
		id, team_id, feed_url, name, is_active, ai_enhance_enabled, article_enrichment_enabled, content_template, title_template, title_hint, output_mode, max_posts_per_day, counter_next,
		prompt_hint, editorial_goal, desired_length, target_account_ids, tonality, initial_sync_mode, last_fetched_at, created_at, knowledge_source_ids,
 (select count(*) from rss_imported_items where feed_id = rss_feed_configs.id and post_id is null and attempts >= 3 and ` + rssItemRetryEligible + `) as exhausted_items_count`
