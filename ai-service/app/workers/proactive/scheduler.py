from __future__ import annotations

import asyncio
import logging
from typing import Any

from app.services import GoloomClient

from .trigger_manager import TriggerManager

logger = logging.getLogger(__name__)


class ProactiveScheduler:
    def __init__(
        self,
        client: GoloomClient,
        *,
        interval_seconds: int = 3600,
    ) -> None:
        self.client = client
        self.interval = interval_seconds
        self._manager = TriggerManager(client)
        self._task: asyncio.Task[Any] | None = None
        self._enabled = True

    async def start(self) -> None:
        if self._task is not None:
            logger.warning("ProactiveScheduler already running")
            return
        logger.info("ProactiveScheduler starting (interval=%ds)", self.interval)
        self._task = asyncio.create_task(self._run_loop())

    async def stop(self) -> None:
        self._enabled = False
        if self._task is not None:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass
            self._task = None
        logger.info("ProactiveScheduler stopped")

    async def _run_loop(self) -> None:
        while self._enabled:
            try:
                await self._tick()
            except Exception:
                logger.exception("ProactiveScheduler tick failed")
            await asyncio.sleep(self.interval)

    async def _tick(self) -> None:
        teams = await self.client.list_ai_enabled_teams()
        if not teams:
            logger.debug("No AI-enabled teams found — skipping tick")
            return

        for team in teams:
            team_id = _get_team_id(team)
            if not team_id:
                continue

            try:
                settings = await self.client.get_proactive_settings(team_id)
                feeds = await self.client.list_rss_feeds(team_id)
                has_active_rss = any(feed.get("is_active") for feed in feeds)
                if not settings.get("auto_fill_enabled", False) and not has_active_rss:
                    logger.debug(
                        "Team %s has no auto-fill and no active RSS feeds — skipping",
                        team_id,
                    )
                    continue

                await self._manager.run_for_team(team_id, settings)
            except Exception:
                logger.exception(
                    "Failed to process proactive triggers for team %s", team_id
                )


def _get_team_id(team: dict[str, Any]) -> str | None:
    raw = team.get("id")
    if raw is None:
        return None
    return str(raw)
