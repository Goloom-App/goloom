package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

// demoSeedTeamName identifies the demo workspace; seeding is idempotent per name.
const demoSeedTeamName = "Solstice Roasters"

// handleAdminSeedDemoData populates a realistic demo team (admin / E2E only).
// It powers the website screenshots: connected accounts, a filled calendar,
// engagement history, review-queue items and automations.
func (a *API) handleAdminSeedDemoData(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if !principal.User.IsAdmin {
		a.writeError(w, r, "forbidden", http.StatusForbidden)
		return
	}
	ctx := r.Context()

	teams, err := a.store.ListTeamsForUser(ctx, principal.User.ID, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, team := range teams {
		if team.Name == demoSeedTeamName {
			auth.WriteJSON(w, http.StatusOK, map[string]any{"team_id": team.ID, "created": false})
			return
		}
	}

	teamID, err := a.seedDemoTeam(ctx, principal)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, map[string]any{"team_id": teamID, "created": true})
}

func (a *API) seedDemoTeam(ctx context.Context, principal domain.AuthenticatedPrincipal) (string, error) {
	team, err := a.store.CreateTeam(ctx, principal.User.ID, domain.CreateTeamInput{
		Name:        demoSeedTeamName,
		Description: "Small-batch specialty coffee, roasted in Freiburg.",
	})
	if err != nil {
		return "", fmt.Errorf("create team: %w", err)
	}

	accounts := []domain.ConnectedAccount{
		{
			Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
			InstanceURL: "https://mastodon.social", Username: "solsticeroasters",
			RemoteAccountID: "109348271", AccessToken: "demo-token",
		},
		{
			Provider: "bluesky", AuthType: domain.AccountAuthTypeAppPassword,
			InstanceURL: "https://bsky.social", Username: "solstice-roasters.bsky.social",
			RemoteAccountID: "did:plc:demosolstice", AccessToken: "demo-password",
		},
		{
			Provider: "friendica", AuthType: domain.AccountAuthTypeOAuthToken,
			InstanceURL: "https://libranet.de", Username: "solsticeroasters",
			RemoteAccountID: "4711", AccessToken: "demo-token",
		},
	}
	accountIDs := make([]string, 0, len(accounts))
	for _, acc := range accounts {
		created, err := a.store.CreateAccount(ctx, team.ID, acc)
		if err != nil {
			return "", fmt.Errorf("create %s account: %w", acc.Provider, err)
		}
		accountIDs = append(accountIDs, created.ID)
	}

	now := time.Now().UTC()
	if err := a.seedDemoPublishedPosts(ctx, team.ID, principal, accountIDs, now); err != nil {
		return "", err
	}
	if err := a.seedDemoUpcomingPosts(ctx, team.ID, principal, accountIDs, now); err != nil {
		return "", err
	}
	if err := a.seedDemoAccountMetrics(ctx, accountIDs, now); err != nil {
		return "", err
	}
	if err := a.seedDemoAutomations(ctx, team.ID, principal, accountIDs); err != nil {
		return "", err
	}
	return team.ID, nil
}

type demoPublishedPost struct {
	title    string
	content  string
	daysAgo  int
	hour     int
	baseLike int64
}

func (a *API) seedDemoPublishedPosts(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, accountIDs []string, now time.Time) error {
	published := []demoPublishedPost{
		{
			title:   "New single origin: Huila, Colombia",
			content: "Chocolate, red apple and a long caramel finish. Our new Colombian single origin from the Huila highlands just landed in the shop. ☕ #specialtycoffee #freshroast",
			daysAgo: 2, hour: 9, baseLike: 46,
		},
		{
			title:   "Behind the roast: drum profiles",
			content: "Ever wondered why the same bean tastes different from batch to batch? A peek at how we tune drum speed and airflow during first crack. 🔥",
			daysAgo: 4, hour: 15, baseLike: 38,
		},
		{
			title:   "Weekend tasting: Ethiopia Guji",
			content: "Cupping table is set! Come by on Saturday between 10 and 14 for a free tasting of our washed Ethiopia Guji — florals for days. 🌸",
			daysAgo: 6, hour: 11, baseLike: 52,
		},
		{
			title:   "Meet us at the farmers market",
			content: "We're back at the Münsterplatz farmers market every Wednesday. Fresh bags, brewed samples, and the occasional roastery dog. 🐕",
			daysAgo: 9, hour: 8, baseLike: 31,
		},
		{
			title:   "Cold brew season is back",
			content: "24 hours steeped, bright and sweet. Our seasonal cold brew blend is available on tap and in bottles starting today. 🧊",
			daysAgo: 12, hour: 13, baseLike: 44,
		},
		{
			title:   "Guatemala Antigua is back on the shelf",
			content: "Cocoa, orange zest and a syrupy body — the Antigua returns for its fourth year with us. Roasted for filter and espresso. 🇬🇹",
			daysAgo: 16, hour: 10, baseLike: 36,
		},
		{
			title:   "How we cup: scoring sheet explained",
			content: "Aroma, flavor, aftertaste, acidity, body, balance. A walkthrough of the SCA cupping form and how we score every incoming lot.",
			daysAgo: 20, hour: 14, baseLike: 41,
		},
		{
			title:   "Summer blend release party recap",
			content: "What a night! Thanks to everyone who came out for the summer blend launch — 200 cups poured, zero beans left. See you next year. 🥂",
			daysAgo: 24, hour: 18, baseLike: 57,
		},
	}

	for _, p := range published {
		scheduledAt := now.AddDate(0, 0, -p.daysAgo)
		scheduledAt = time.Date(scheduledAt.Year(), scheduledAt.Month(), scheduledAt.Day(), p.hour, 30, 0, 0, time.UTC)
		input := domain.CreatePostInput{
			Title:          p.title,
			Content:        p.content,
			ScheduledAt:    scheduledAt,
			TargetAccounts: accountIDs,
			// Draft avoids a pending post in the past that the scheduler would
			// immediately try to publish against the fake accounts.
			Draft:  true,
			Source: domain.PostSourceScheduled,
		}
		post, err := a.store.CreateScheduledPost(ctx, teamID, principal, input)
		if err != nil {
			return fmt.Errorf("create published post %q: %w", p.title, err)
		}
		if err := a.store.MarkPostResult(ctx, post.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
			return fmt.Errorf("mark posted %q: %w", p.title, err)
		}
		for i, accountID := range accountIDs {
			url := fmt.Sprintf("https://demo.invalid/%s/%d", post.ID, i)
			if err := a.store.MarkPostTargetResult(ctx, post.ID, accountID, domain.PostStatusPosted, url, "", nil, fmt.Sprintf("demo-%s-%d", post.ID, i)); err != nil {
				return fmt.Errorf("mark target posted %q: %w", p.title, err)
			}
		}
		// Engagement grows day by day after publication so the 30d trend chart
		// has a curve instead of a single point.
		for age := p.daysAgo; age >= 0; age-- {
			day := now.AddDate(0, 0, -age).Format("2006-01-02")
			growth := int64(p.daysAgo - age + 1)
			for i, accountID := range accountIDs {
				share := int64(len(accountIDs) - i) // first account most active
				metrics := map[string]int64{
					"likes":   (p.baseLike * share / 3) + growth*2,
					"reposts": (p.baseLike * share / 9) + growth/2,
					"replies": (p.baseLike * share / 14) + growth/3,
				}
				if err := a.store.UpsertPostMetrics(ctx, post.ID, accountID, metrics, day); err != nil {
					return fmt.Errorf("post metrics %q: %w", p.title, err)
				}
			}
		}
	}
	return nil
}

func (a *API) seedDemoUpcomingPosts(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, accountIDs []string, now time.Time) error {
	upcoming := []struct {
		title   string
		content string
		daysOut int
		hour    int
		draft   bool
	}{
		{
			title:   "Open roastery day — save the date",
			content: "On the 26th we open the roastery doors: tours every hour, cuppings all day, and the new harvest on the table. Free entry, bring friends! 🎉",
			daysOut: 1, hour: 10,
		},
		{
			title:   "Brew guide: V60 at home",
			content: "15g coffee, 250g water, 3 pours, 2:45 total. Our head roaster's V60 recipe for the Huila — full guide on the blog. ⏱️",
			daysOut: 2, hour: 16,
		},
		{
			title:   "Subscriber spotlight: Café Klara",
			content: "Our beans, their bar: Café Klara in the Sedanviertel has been pouring Solstice espresso for three years. Go say hi. ❤️",
			daysOut: 4, hour: 12,
		},
		{
			title:   "Espresso workshop — tickets live",
			content: "Dial in like a pro: our two-hour espresso workshop is back in October. Eight spots, all gear provided. Link in bio. 🎟️",
			daysOut: 6, hour: 9,
		},
		{
			title:   "Autumn lineup teaser",
			content: "Three new origins land next month. Here's a first hint: one of them is a honey-processed Costa Rica… 🍂",
			daysOut: 8, hour: 11, draft: true,
		},
		{
			title:   "Roaster Q&A: ask us anything",
			content: "Drop your coffee questions in the replies — grind size, storage, brew ratios, roast dates. We'll answer the best ones live on Friday. 💬",
			daysOut: 10, hour: 17,
		},
		{
			title:   "New decaf: sugarcane process",
			content: "Decaf that actually tastes like coffee: our new Colombian sugarcane-process decaf is smooth, sweet and 99.9% caffeine free. 😴",
			daysOut: 12, hour: 9,
		},
		{
			title:   "Latte art throwdown at the bar",
			content: "Friday night: baristas from all over town battle for the golden pitcher. Doors at 19:00, free entry, loud music, great milk. 🏆",
			daysOut: 15, hour: 19,
		},
		{
			title:   "Harvest report: visiting our partners in Huila",
			content: "Fresh photos from the farm: our green buyer is in Colombia this week visiting the families who grow our beans. Thread incoming. 🌱",
			daysOut: 18, hour: 8,
		},
		{
			title:   "Merch drop: enamel mugs",
			content: "The classic enamel mug is back in three colors, now with a matte finish. Limited run of 150 — online shop and roastery only. ⛺",
			daysOut: 21, hour: 12,
		},
		{
			title:   "Barista basics: milk steaming",
			content: "Microfoam, not bubbles: our five-minute guide to steaming milk at home, from purge to pour. Video on the blog. 🥛",
			daysOut: 24, hour: 16,
		},
	}

	for _, p := range upcoming {
		scheduledAt := now.AddDate(0, 0, p.daysOut)
		scheduledAt = time.Date(scheduledAt.Year(), scheduledAt.Month(), scheduledAt.Day(), p.hour, 0, 0, 0, time.UTC)
		input := domain.CreatePostInput{
			Title:          p.title,
			Content:        p.content,
			ScheduledAt:    scheduledAt,
			TargetAccounts: accountIDs,
			Draft:          p.draft,
			Source:         domain.PostSourceScheduled,
		}
		if _, err := a.store.CreateScheduledPost(ctx, teamID, principal, input); err != nil {
			return fmt.Errorf("create upcoming post %q: %w", p.title, err)
		}
	}
	return nil
}

func (a *API) seedDemoAccountMetrics(ctx context.Context, accountIDs []string, now time.Time) error {
	base := []map[string]int64{
		{"followers": 1180, "following": 214, "posts": 382}, // mastodon
		{"followers": 640, "following": 87, "posts": 191},   // bluesky
		{"followers": 152, "following": 63, "posts": 120},   // friendica
	}
	growth := []int64{6, 8, 1}
	for day := 29; day >= 0; day-- {
		recordedAt := now.AddDate(0, 0, -day)
		elapsed := int64(29 - day)
		for i, accountID := range accountIDs {
			wobble := (elapsed * int64(i+3)) % 4
			metrics := map[string]int64{
				"followers": base[i]["followers"] + elapsed*growth[i] + wobble,
				"following": base[i]["following"] + elapsed/9,
				"posts":     base[i]["posts"] + elapsed/2,
			}
			if err := a.store.UpsertAccountMetrics(ctx, accountID, metrics, recordedAt); err != nil {
				return fmt.Errorf("account metrics: %w", err)
			}
		}
	}
	return nil
}

func (a *API) seedDemoAutomations(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, accountIDs []string) error {
	reviewDrafts := []struct {
		title    string
		content  string
		hoursAgo int
	}{
		{
			title:    "Coffee news weekly #34",
			content:  "This week in coffee: SCA announces the 2027 world brewers cup host city, and a new study on grind distribution. Read the roundup: https://solstice-roasters.example/news/34",
			hoursAgo: 3,
		},
		{
			title:    "Roastery blog: water chemistry 101",
			content:  "Why your brew water matters more than your grinder — our deep dive into bicarbonates, magnesium and what actually changes in the cup.",
			hoursAgo: 1,
		},
	}
	for _, d := range reviewDrafts {
		input := domain.CreatePostInput{
			Title:          d.title,
			Content:        d.content,
			ScheduledAt:    time.Now().UTC().Add(-time.Duration(d.hoursAgo) * time.Hour),
			TargetAccounts: accountIDs,
			Draft:          true,
			Source:         domain.PostSourceAutomation,
		}
		if _, err := a.store.CreateScheduledPost(ctx, teamID, principal, input); err != nil {
			return fmt.Errorf("create review draft %q: %w", d.title, err)
		}
	}

	if _, err := a.store.CreateRSSFeedConfig(ctx, teamID, domain.RSSFeedConfig{
		Name:             "Roastery blog",
		FeedURL:          "https://solstice-roasters.example/blog/feed.xml",
		IsActive:         true,
		MaxPostsPerDay:   3,
		TargetAccountIDs: accountIDs,
	}); err != nil {
		return fmt.Errorf("create rss feed: %w", err)
	}

	enabled := true
	if _, err := a.store.CreatePostTemplate(ctx, teamID, principal, domain.CreatePostTemplateInput{
		Title:            "Freshly roasted this week",
		Content:          "Fresh batch day! ☕ This week's roasts are bagged and ready: Huila, Guji and our house espresso blend. Order before noon and it ships today.",
		RecurrenceJSON:   `{"kind":"weekly","weekdays":[5],"hour":8,"minute":30,"timezone":"Europe/Berlin"}`,
		TargetAccountIDs: accountIDs,
		Enabled:          &enabled,
	}); err != nil {
		return fmt.Errorf("create post template: %w", err)
	}
	if _, err := a.store.CreatePostTemplate(ctx, teamID, principal, domain.CreatePostTemplateInput{
		Title:            "Monthly cupping night",
		Content:          "First Thursday of the month means public cupping night: six coffees on the table, all palates welcome. 19:00 at the roastery, no ticket needed.",
		RecurrenceJSON:   `{"kind":"monthly_ordinal_weekday","occurrences":[{"ordinal":1,"weekday":4}],"hour":19,"minute":0,"timezone":"Europe/Berlin"}`,
		TargetAccountIDs: accountIDs,
		Enabled:          &enabled,
	}); err != nil {
		return fmt.Errorf("create cupping template: %w", err)
	}
	return nil
}
