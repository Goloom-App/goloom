from __future__ import annotations

import logging
from abc import ABC, abstractmethod
from datetime import UTC, datetime, timedelta
from typing import Any

from app.services import GoloomClient

logger = logging.getLogger(__name__)


class BaseHook(ABC):
    def __init__(self, client: GoloomClient) -> None:
        self.client = client

    @abstractmethod
    async def run(self, team_id: str, settings: dict[str, Any]) -> bool:
        ...


class ContentCalendarHook(BaseHook):
    async def run(self, team_id: str, settings: dict[str, Any]) -> bool:
        threshold = settings.get("content_gap_threshold_days", 3)
        context = await self.client.get_ai_context(team_id)
        scheduled_posts = context.get("scheduled_posts") or []

        has_upcoming = any(
            p for p in scheduled_posts if _within_days(p, threshold)
        )

        if has_upcoming:
            logger.debug("Team %s has upcoming posts within %d days — no gap", team_id, threshold)
            return False

        logger.info("Content gap detected for team %s (threshold=%d days)", team_id, threshold)
        await self.client.trigger_job(
            team_id,
            "proactive_trigger",
            {"trigger_type": "content_gap", "content_hint": "Fill content gap"},
        )
        return True


class TrendingTopicHook(BaseHook):
    async def run(self, team_id: str, settings: dict[str, Any]) -> bool:
        logger.debug("TrendingTopicHook stub — skipping team %s", team_id)
        return False


def _within_days(post: dict, days: int) -> bool:
    scheduled_str = post.get("scheduled_at")
    if not scheduled_str:
        return True
    try:
        scheduled = datetime.fromisoformat(scheduled_str)
        if scheduled.tzinfo is None:
            scheduled = scheduled.replace(tzinfo=UTC)
    except (ValueError, TypeError):
        return True

    now = datetime.now(UTC)
    return scheduled <= now + timedelta(days=days)
