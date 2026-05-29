from __future__ import annotations

from datetime import UTC, datetime
from unittest.mock import AsyncMock, MagicMock, call, patch

import feedparser as _feedparser_real
import pytest

from app.services import GoloomClient
from app.workers.proactive.rss import ContentItem, FeedItem, RSSExtractor, RSSHook

MOCK_RSS_XML = """\
<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test RSS Feed</title>
    <link>https://example.com</link>
    <description>Test feed for unit tests</description>
    <item>
      <title>Old Item 1</title>
      <link>https://example.com/old-1</link>
      <description>Old content 1</description>
      <pubDate>Mon, 01 Jan 2024 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Old Item 2</title>
      <link>https://example.com/old-2</link>
      <description>Old content 2</description>
      <pubDate>Tue, 01 Oct 2024 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>New Item 1</title>
      <link>https://example.com/new-1</link>
      <description>New content 1</description>
      <pubDate>Thu, 01 Jan 2025 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>New Item 2</title>
      <link>https://example.com/new-2</link>
      <description>New content 2</description>
      <pubDate>Tue, 01 Apr 2025 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>New Item 3</title>
      <link>https://example.com/new-3</link>
      <description>New content 3</description>
      <pubDate>Fri, 01 Aug 2025 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>
"""

_PARSED_MOCK_FEED = _feedparser_real.parse(MOCK_RSS_XML)

LAST_FETCHED = datetime(2024, 12, 1, 0, 0, 0, tzinfo=UTC)
FEED_URL = "https://example.com/feed.rss"


def _make_feed_config(
    *,
    feed_id: str = "feed-1",
    feed_url: str = FEED_URL,
    is_active: bool = True,
    last_fetched_at: str | None = LAST_FETCHED.isoformat(),
) -> dict:
    return {
        "id": feed_id,
        "team_id": "team-1",
        "feed_url": feed_url,
        "name": "Test Feed",
        "is_active": is_active,
        "last_fetched_at": last_fetched_at,
    }


@patch("app.workers.proactive.rss.feedparser")
def test_fetch_feed_parses_all_items(mock_fp: MagicMock) -> None:
    mock_fp.parse.return_value = _PARSED_MOCK_FEED

    extractor = RSSExtractor()
    items = extractor.fetch_feed(FEED_URL)

    mock_fp.parse.assert_called_once_with(FEED_URL)
    assert len(items) == 5
    assert all(isinstance(i, FeedItem) for i in items)


@patch("app.workers.proactive.rss.feedparser")
def test_fetch_feed_empty_on_parse_error(mock_fp: MagicMock) -> None:
    mock_fp.parse.side_effect = Exception("Connection refused")

    extractor = RSSExtractor()
    items = extractor.fetch_feed("https://bad.example.com/feed.rss")

    assert items == []


@patch("app.workers.proactive.rss.feedparser")
def test_fetch_feed_empty_on_bozo_no_entries(mock_fp: MagicMock) -> None:
    mock_fp.parse.return_value = {"entries": [], "bozo": True}

    extractor = RSSExtractor()
    items = extractor.fetch_feed("https://example.com/bad-feed.rss")

    assert items == []


@patch("app.workers.proactive.rss.feedparser")
def test_extract_content_filters_new_items(mock_fp: MagicMock) -> None:
    mock_fp.parse.return_value = _PARSED_MOCK_FEED

    extractor = RSSExtractor()
    feed_items = extractor.fetch_feed(FEED_URL)
    new_items = extractor.extract_content(feed_items, LAST_FETCHED, FEED_URL)

    assert len(new_items) == 3
    assert all(isinstance(i, ContentItem) for i in new_items)
    assert all(i.feed_url == FEED_URL for i in new_items)
    titles = {i.title for i in new_items}
    assert titles == {"New Item 1", "New Item 2", "New Item 3"}


@patch("app.workers.proactive.rss.feedparser")
def test_extract_content_empty_feed_returns_empty(mock_fp: MagicMock) -> None:
    mock_fp.parse.return_value = {"entries": [], "bozo": False}

    extractor = RSSExtractor()
    feed_items = extractor.fetch_feed(FEED_URL)
    new_items = extractor.extract_content(feed_items, LAST_FETCHED, FEED_URL)

    assert new_items == []


@pytest.mark.asyncio
@patch("app.workers.proactive.rss.feedparser")
async def test_rss_hook_triggers_new_items(mock_fp: MagicMock) -> None:
    mock_fp.parse.return_value = _PARSED_MOCK_FEED

    client = AsyncMock(spec=GoloomClient)
    client.list_rss_feeds.return_value = [_make_feed_config()]

    hook = RSSHook(client)
    result = await hook.run("team-1", {})

    assert result is True
    assert client.trigger_job.await_count == 3

    expected_calls = [
        call(
            "team-1",
            "proactive_trigger",
            {"trigger_type": "rss", "source_url": "https://example.com/new-1", "content_hint": "New Item 1"},
        ),
        call(
            "team-1",
            "proactive_trigger",
            {"trigger_type": "rss", "source_url": "https://example.com/new-2", "content_hint": "New Item 2"},
        ),
        call(
            "team-1",
            "proactive_trigger",
            {"trigger_type": "rss", "source_url": "https://example.com/new-3", "content_hint": "New Item 3"},
        ),
    ]
    client.trigger_job.assert_has_awaits(expected_calls, any_order=True)

    client.update_rss_feed.assert_awaited_once()
    update_call = client.update_rss_feed.await_args
    assert update_call.args[0] == "team-1"
    assert update_call.args[1] == "feed-1"
    assert "last_fetched_at" in update_call.args[2]


@pytest.mark.asyncio
@patch("app.workers.proactive.rss.feedparser")
async def test_rss_hook_skips_inactive_feeds(mock_fp: MagicMock) -> None:
    mock_fp.parse.return_value = _PARSED_MOCK_FEED

    client = AsyncMock(spec=GoloomClient)
    client.list_rss_feeds.return_value = [_make_feed_config(is_active=False)]

    hook = RSSHook(client)
    result = await hook.run("team-1", {})

    assert result is False
    client.trigger_job.assert_not_called()
    client.update_rss_feed.assert_not_called()


@pytest.mark.asyncio
@patch("app.workers.proactive.rss.feedparser")
async def test_rss_hook_returns_false_when_no_new_items(mock_fp: MagicMock) -> None:
    mock_fp.parse.return_value = _PARSED_MOCK_FEED

    client = AsyncMock(spec=GoloomClient)
    client.list_rss_feeds.return_value = [
        _make_feed_config(last_fetched_at="2030-01-01T00:00:00+00:00")
    ]

    hook = RSSHook(client)
    result = await hook.run("team-1", {})

    assert result is False
    client.trigger_job.assert_not_called()


@pytest.mark.asyncio
@patch("app.workers.proactive.rss.feedparser")
async def test_rss_hook_handles_no_last_fetched(mock_fp: MagicMock) -> None:
    mock_fp.parse.return_value = _PARSED_MOCK_FEED

    client = AsyncMock(spec=GoloomClient)
    client.list_rss_feeds.return_value = [_make_feed_config(last_fetched_at=None)]

    hook = RSSHook(client)
    result = await hook.run("team-1", {})

    assert result is True
    assert client.trigger_job.await_count == 5  # null last_fetched_at → epoch → all items new
