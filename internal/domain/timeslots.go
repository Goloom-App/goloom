package domain

import (
	"sort"
	"time"
)

// PostEngagement is one posted post with its total engagement and the time it
// was scheduled (stored UTC). It is the raw input for timeslot analysis.
type PostEngagement struct {
	PostID      string    `json:"post_id"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Engagement  int64     `json:"engagement"`
}

// TimeslotPerformance aggregates engagement for all posts published in one
// weekday/hour bucket, evaluated in a chosen timezone.
type TimeslotPerformance struct {
	Weekday         time.Weekday `json:"weekday"`
	Hour            int          `json:"hour"`
	Posts           int          `json:"posts"`
	TotalEngagement int64        `json:"total_engagement"`
	AvgEngagement   float64      `json:"avg_engagement"`
	Score           float64      `json:"score"`
}

// TimeslotScoreSmoothing is k in score = total_engagement / (posts + k). It keeps
// a single lucky post in an otherwise-empty slot from topping the ranking the way
// a busy slot with a solid average should.
const TimeslotScoreSmoothing = 1

// AggregateTimeslots buckets posts by weekday and hour-of-day evaluated in loc,
// then ranks the buckets by smoothed average engagement. A nil loc defaults to
// UTC. A limit <= 0 returns every populated bucket.
func AggregateTimeslots(posts []PostEngagement, loc *time.Location, limit int) []TimeslotPerformance {
	if loc == nil {
		loc = time.UTC
	}
	type key struct {
		weekday time.Weekday
		hour    int
	}
	buckets := make(map[key]*TimeslotPerformance)
	for _, p := range posts {
		local := p.ScheduledAt.In(loc)
		k := key{weekday: local.Weekday(), hour: local.Hour()}
		b := buckets[k]
		if b == nil {
			b = &TimeslotPerformance{Weekday: k.weekday, Hour: k.hour}
			buckets[k] = b
		}
		b.Posts++
		b.TotalEngagement += p.Engagement
	}

	out := make([]TimeslotPerformance, 0, len(buckets))
	for _, b := range buckets {
		b.AvgEngagement = float64(b.TotalEngagement) / float64(b.Posts)
		b.Score = float64(b.TotalEngagement) / float64(b.Posts+TimeslotScoreSmoothing)
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Posts != out[j].Posts {
			return out[i].Posts > out[j].Posts
		}
		if out[i].Weekday != out[j].Weekday {
			return out[i].Weekday < out[j].Weekday
		}
		return out[i].Hour < out[j].Hour
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
