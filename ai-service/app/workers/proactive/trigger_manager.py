from __future__ import annotations

import logging
from collections import defaultdict
from datetime import date
from typing import Any

from app.services import GoloomClient

from .event_hooks import BaseHook, ContentCalendarHook, TrendingTopicHook

logger = logging.getLogger(__name__)


class TriggerManager:
    def __init__(self, client: GoloomClient) -> None:
        self.client = client
        self.hooks: list[BaseHook] = [
            ContentCalendarHook(client),
            TrendingTopicHook(client),
        ]
        self._rate_counter: dict[str, dict[str, int]] = defaultdict(
            lambda: defaultdict(int)
        )

    async def run_for_team(self, team_id: str, settings: dict[str, Any]) -> list[bool]:
        max_per_day = settings.get("max_triggers_per_day", 5)

        if self._is_rate_limited(team_id, max_per_day):
            logger.warning(
                "Rate limit exceeded for team %s (max %d/day) — skipping triggers",
                team_id,
                max_per_day,
            )
            return []

        today_key = date.today().isoformat()
        results: list[bool] = []
        for hook in self.hooks:
            triggered = await hook.run(team_id, settings)
            if triggered:
                self._rate_counter[team_id][today_key] += 1
            results.append(triggered)

            if self._is_rate_limited(team_id, max_per_day):
                logger.warning(
                    "Rate limit hit mid-cycle for team %s — stopping further hooks",
                    team_id,
                )
                break

        return results

    def _is_rate_limited(self, team_id: str, max_per_day: int) -> bool:
        today_key = date.today().isoformat()
        for tid in list(self._rate_counter):
            for d in list(self._rate_counter[tid]):
                if d != today_key:
                    del self._rate_counter[tid][d]
        return self._rate_counter[team_id][today_key] >= max_per_day
