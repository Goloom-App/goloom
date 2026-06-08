package domain

import "time"

// ReviewQueueItem is an automation draft awaiting human review.
type ReviewQueueItem struct {
	ScheduledPost
	IsOverdue   bool   `json:"is_overdue"`
	RSSFeedName string `json:"rss_feed_name,omitempty"`
}

func NewReviewQueueItem(post ScheduledPost, feedName string, now time.Time) ReviewQueueItem {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return ReviewQueueItem{
		ScheduledPost: post,
		IsOverdue:     post.Status == PostStatusDraft && post.ScheduledAt.Before(now),
		RSSFeedName:   feedName,
	}
}
