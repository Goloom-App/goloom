package api

import (
	"context"
	"errors"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/htmltext"
	"git.f4mily.net/goloom/internal/netguard"
	"git.f4mily.net/goloom/internal/pagefetch"
)

// urlFetchPolicy guards page fetches: URLs come from users and the chat model
// (injection-controllable), so only public addresses are reachable.
var urlFetchPolicy = netguard.NewPolicy(nil)

func fetchURLBody(ctx context.Context, rawURL string) (string, error) {
	return pagefetch.FetchBody(ctx, rawURL, urlFetchPolicy, 20*time.Second)
}

func fetchURLText(ctx context.Context, rawURL string) (string, error) {
	body, err := fetchURLBody(ctx, rawURL)
	if err != nil {
		return "", err
	}
	return extractReadableTextFromHTML(body), nil
}

func enrichParamsFromWebPage(ctx context.Context, params map[string]any, pageURL string) error {
	if strings.TrimSpace(stringParam(params["source_content"])) != "" {
		return nil
	}

	body, err := fetchURLBody(ctx, pageURL)
	if err != nil {
		return err
	}

	content := htmltext.ExtractReadableText(body)
	if strings.TrimSpace(content) == "" {
		return errors.New("source_url_empty")
	}

	params["source_content"] = content
	if title := htmltext.ExtractPageTitle(body); title != "" {
		params["page_title"] = title
	}
	return nil
}
