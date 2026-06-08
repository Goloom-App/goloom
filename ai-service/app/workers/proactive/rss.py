from __future__ import annotations

import logging
import re
from dataclasses import dataclass
from datetime import UTC, datetime
from html import unescape
from typing import Any

import feedparser

from app.services import GoloomClient
from app.workers.proactive.event_hooks import BaseHook

logger = logging.getLogger(__name__)

_TAG_RE = re.compile(r"<[^>]+>")


@dataclass
class FeedItem:
    title: str
    link: str
    content: str
    published: datetime


@dataclass
class ContentItem:
    title: str
    link: str
    content: str
    published: datetime
    feed_url: str


class RSSExtractor:
    def fetch_feed(self, url: str) -> list[FeedItem]:
        try:
            parsed = feedparser.parse(url)
            entries = parsed.get("entries", [])
            if not entries and parsed.get("bozo"):
                logger.warning("Failed to parse feed (bozo): %s", url)
                return []
            items: list[FeedItem] = []
            for entry in entries:
                title = entry.get("title", "")
                link = entry.get("link", "")
                content_list = entry.get("content", [])
                if content_list:
                    content = content_list[0].get("value", "")
                else:
                    content = entry.get("summary", "")
                published_parsed = entry.get("published_parsed")
                if published_parsed:
                    published = datetime(*published_parsed[:6], tzinfo=UTC)
                else:
                    published = datetime.now(UTC)
                items.append(
                    FeedItem(title=title, link=link, content=content, published=published)
                )
            return items
        except Exception as exc:
            logger.warning("Error fetching feed %s: %s", url, exc)
            return []

    def extract_content(
        self,
        items: list[FeedItem],
        since: datetime,
        feed_url: str = "",
    ) -> list[ContentItem]:
        if since.tzinfo is None:
            since = since.replace(tzinfo=UTC)
        result: list[ContentItem] = []
        for item in items:
            pub = item.published
            if pub.tzinfo is None:
                pub = pub.replace(tzinfo=UTC)
            if pub > since:
                result.append(
                    ContentItem(
                        title=item.title,
                        link=item.link,
                        content=item.content,
                        published=item.published,
                        feed_url=feed_url,
                    )
                )
        return result


def _strip_html(value: str) -> str:
    text = unescape(_TAG_RE.sub(" ", value or ""))
    return " ".join(text.split())


def _build_content_hint(feed: dict[str, Any], item: ContentItem) -> str:
    parts: list[str] = []
    prompt_hint = str(feed.get("prompt_hint") or "").strip()
    if prompt_hint:
        parts.append(prompt_hint)
    if item.title:
        parts.append(f"Article title: {item.title}")
    if item.link:
        parts.append(f"Source URL: {item.link}")
    body = _strip_html(item.content)
    if body:
        parts.append(f"Article content:\n{body[:4000]}")
    return "\n\n".join(parts)


def _trigger_params(feed: dict[str, Any], item: ContentItem) -> dict[str, Any]:
    target_ids = feed.get("target_account_ids") or []
    if not isinstance(target_ids, list):
        target_ids = []
    tonality = str(feed.get("tonality") or "").strip()
    params: dict[str, Any] = {
        "trigger_type": "rss",
        "source_url": item.link,
        "content_hint": _build_content_hint(feed, item),
        "target_account_ids": [str(item_id) for item_id in target_ids if str(item_id).strip()],
        "schedule": False,
    }
    if tonality:
        params["tonality"] = tonality
    return params


class RSSHook(BaseHook):
    def __init__(self, client: GoloomClient) -> None:
        super().__init__(client)
        self._extractor = RSSExtractor()

    async def run(self, team_id: str, settings: dict[str, Any]) -> bool:
        feeds = await self.client.list_rss_feeds(team_id)
        now = datetime.now(UTC)
        any_processed = False

        for feed in feeds:
            if not feed.get("is_active", False):
                continue

            feed_id = feed["id"]
            feed_url = feed["feed_url"]
            last_fetched_str = feed.get("last_fetched_at")
            target_ids = feed.get("target_account_ids") or []
            if not target_ids:
                logger.warning(
                    "RSS feed %s has no target accounts configured — skipping triggers",
                    feed_id,
                )
                continue

            if not last_fetched_str:
                try:
                    await self.client.update_rss_feed(
                        team_id,
                        feed_id,
                        {"last_fetched_at": now.isoformat()},
                    )
                    logger.info(
                        "Baselined RSS feed %s without triggering historical items",
                        feed_id,
                    )
                except Exception as exc:
                    logger.warning(
                        "Failed to baseline last_fetched_at for feed %s: %s",
                        feed_id,
                        exc,
                    )
                continue

            try:
                last_fetched = datetime.fromisoformat(last_fetched_str)
                if last_fetched.tzinfo is None:
                    last_fetched = last_fetched.replace(tzinfo=UTC)
            except (ValueError, TypeError):
                last_fetched = datetime.min.replace(tzinfo=UTC)

            items = self._extractor.fetch_feed(feed_url)
            new_items = self._extractor.extract_content(items, last_fetched, feed_url)

            for item in new_items:
                try:
                    await self.client.trigger_job(
                        team_id,
                        "proactive_trigger",
                        _trigger_params(feed, item),
                    )
                    any_processed = True
                except Exception as exc:
                    logger.warning(
                        "Failed to trigger job for item %s in feed %s: %s",
                        item.link,
                        feed_url,
                        exc,
                    )

            try:
                await self.client.update_rss_feed(
                    team_id,
                    feed_id,
                    {"last_fetched_at": now.isoformat()},
                )
            except Exception as exc:
                logger.warning(
                    "Failed to update last_fetched_at for feed %s: %s",
                    feed_id,
                    exc,
                )

        return any_processed
