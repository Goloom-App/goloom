from __future__ import annotations

import asyncio
import logging
from datetime import UTC, datetime
from typing import Any

from app.services import GoloomClient

from .cron_schedule import is_team_due
from .trigger_manager import TriggerManager

logger = logging.getLogger(__name__)


class ProactiveScheduler:
    def __init__(
        self,
        client: GoloomClient,
        *,
        poll_seconds: int = 60,
    ) -> None:
        self.client = client
        self.poll_seconds = max(15, poll_seconds)
        self._manager = TriggerManager(client)
        self._task: asyncio.Task[Any] | None = None
        self._enabled = True
        self._team_last_run: dict[str, datetime] = {}

    async def start(self) -> None:
        if self._task is not None:
            logger.warning("ProactiveScheduler already running")
            return
        logger.info("ProactiveScheduler starting (poll=%ds)", self.poll_seconds)
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
            await asyncio.sleep(self.poll_seconds)

    async def _tick(self) -> None:
        teams = await self.client.list_ai_enabled_teams()
        if not teams:
            logger.debug("No AI-enabled teams found — skipping tick")
            return

        now = datetime.now(UTC)
        for team in teams:
            team_id = _get_team_id(team)
            if not team_id:
                continue

            try:
                settings = await self.client.get_proactive_settings(team_id)
                if not settings.get("auto_fill_enabled", False):
                    logger.debug(
                        "Team %s has auto-fill disabled — skipping",
                        team_id,
                    )
                    continue

                cron_schedule = str(settings.get("cron_schedule") or "0 * * * *")
                last_run = self._team_last_run.get(team_id)
                if not is_team_due(cron_schedule, now, last_run):
                    logger.debug(
                        "Team %s not due yet (cron=%s, last_run=%s)",
                        team_id,
                        cron_schedule,
                        last_run.isoformat() if last_run else "never",
                    )
                    continue

                await self._manager.run_for_team(team_id, settings)
                self._team_last_run[team_id] = now
            except Exception:
                logger.exception(
                    "Failed to process proactive triggers for team %s", team_id
                )


def _get_team_id(team: dict[str, Any]) -> str | None:
    raw = team.get("id")
    if raw is None:
        return None
    return str(raw)
