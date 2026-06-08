from datetime import UTC, datetime, timedelta
from unittest.mock import AsyncMock

import pytest

from app.services import GoloomClient
from app.workers.proactive import ProactiveScheduler, TriggerManager
from app.workers.proactive.event_hooks import ContentCalendarHook


def make_team(team_id: str) -> dict:
    return {"id": team_id, "name": f"Team {team_id}"}


def make_context(scheduled_posts: list | None = None) -> dict:
    return {
        "team": {"id": "team-1", "name": "Test Team"},
        "profile": {"style_metadata": {"tonality": "professional"}},
        "scheduled_posts": scheduled_posts or [],
        "style_examples": [],
        "recent_posts": [],
    }


@pytest.mark.asyncio
async def test_content_calendar_hook_detects_gap_and_triggers():
    client = AsyncMock(spec=GoloomClient)
    client.get_ai_context.return_value = make_context(scheduled_posts=[])
    hook = ContentCalendarHook(client)

    result = await hook.run("team-1", {"auto_fill_enabled": True})

    assert result is True
    client.trigger_job.assert_awaited_once_with(
        "team-1",
        "proactive_trigger",
        {"trigger_type": "content_gap", "content_hint": "Fill content gap"},
    )


@pytest.mark.asyncio
async def test_content_calendar_hook_no_gap_when_posts_exist():
    future = (datetime.now(UTC) + timedelta(days=1)).isoformat()
    client = AsyncMock(spec=GoloomClient)
    client.get_ai_context.return_value = make_context(
        scheduled_posts=[{"scheduled_at": future}]
    )
    hook = ContentCalendarHook(client)

    result = await hook.run("team-1", {"content_gap_threshold_days": 3, "auto_fill_enabled": True})

    assert result is False
    client.trigger_job.assert_not_called()


@pytest.mark.asyncio
async def test_content_calendar_hook_respects_custom_threshold():
    far_future = (datetime.now(UTC) + timedelta(days=10)).isoformat()
    client = AsyncMock(spec=GoloomClient)
    client.get_ai_context.return_value = make_context(
        scheduled_posts=[{"scheduled_at": far_future}]
    )
    hook = ContentCalendarHook(client)

    result = await hook.run("team-1", {"content_gap_threshold_days": 14, "auto_fill_enabled": True})

    assert result is False
    client.trigger_job.assert_not_called()


@pytest.mark.asyncio
async def test_rate_limiting_blocks_excessive_triggers():
    client = AsyncMock(spec=GoloomClient)
    client.get_ai_context.return_value = make_context(scheduled_posts=[])
    manager = TriggerManager(client)

    settings = {"max_triggers_per_day": 2, "content_gap_threshold_days": 3, "auto_fill_enabled": True}

    r1 = await manager.run_for_team("team-rate", settings)
    assert len(r1) == 3
    assert r1[0] is True
    assert r1[1] is False
    assert r1[2] is False

    r2 = await manager.run_for_team("team-rate", settings)
    assert r2 == [True]

    r3 = await manager.run_for_team("team-rate", settings)
    assert r3 == []

    assert client.trigger_job.await_count == 2


@pytest.mark.asyncio
async def test_rate_limiting_resets_on_new_day():
    client = AsyncMock(spec=GoloomClient)
    client.get_ai_context.return_value = make_context(scheduled_posts=[])
    manager = TriggerManager(client)

    settings = {"max_triggers_per_day": 1, "content_gap_threshold_days": 3, "auto_fill_enabled": True}

    r1 = await manager.run_for_team("team-rate2", settings)
    assert r1 != []

    r2 = await manager.run_for_team("team-rate2", settings)
    assert r2 == []

    today_key = datetime.now(UTC).date().isoformat()
    yesterday_key = (datetime.now(UTC) - timedelta(days=1)).date().isoformat()
    manager._rate_counter["team-rate2"][yesterday_key] = manager._rate_counter["team-rate2"][today_key]
    del manager._rate_counter["team-rate2"][today_key]

    r3 = await manager.run_for_team("team-rate2", settings)
    assert r3 != []


@pytest.mark.asyncio
async def test_scheduler_skips_teams_with_auto_fill_disabled():
    client = AsyncMock(spec=GoloomClient)
    client.list_ai_enabled_teams.return_value = [
        make_team("team-skip"),
        make_team("team-active"),
    ]

    async def proactive_settings(team_id: str) -> dict:
        if team_id == "team-skip":
            return {"auto_fill_enabled": False, "max_triggers_per_day": 5, "cron_schedule": "0 * * * *"}
        return {"auto_fill_enabled": True, "max_triggers_per_day": 5, "cron_schedule": "0 * * * *"}

    client.get_proactive_settings.side_effect = proactive_settings
    client.list_rss_feeds.return_value = []
    client.get_ai_context.return_value = make_context(scheduled_posts=[])

    scheduler = ProactiveScheduler(client, poll_seconds=99999)
    await scheduler._tick()

    assert client.trigger_job.await_count == 1
    assert client.trigger_job.await_args.args[0] == "team-active"


@pytest.mark.asyncio
async def test_scheduler_runs_for_all_enabled_teams():
    client = AsyncMock(spec=GoloomClient)
    client.list_ai_enabled_teams.return_value = [
        make_team("team-1"),
        make_team("team-2"),
    ]
    client.get_proactive_settings.return_value = {
        "auto_fill_enabled": True,
        "max_triggers_per_day": 5,
        "cron_schedule": "0 * * * *",
    }
    client.list_rss_feeds.return_value = []
    client.get_ai_context.return_value = make_context(scheduled_posts=[])

    scheduler = ProactiveScheduler(client, poll_seconds=99999)
    await scheduler._tick()

    assert client.trigger_job.await_count == 2
    called_teams = {call.args[0] for call in client.trigger_job.await_args_list}
    assert called_teams == {"team-1", "team-2"}


@pytest.mark.asyncio
async def test_scheduler_start_stop():
    client = AsyncMock(spec=GoloomClient)
    client.list_ai_enabled_teams.return_value = []

    scheduler = ProactiveScheduler(client, poll_seconds=0.01)

    await scheduler.start()
    assert scheduler._task is not None
    assert not scheduler._task.done()

    await scheduler.stop()
    assert scheduler._task is None


@pytest.mark.asyncio
async def test_scheduler_empty_teams_skips_gracefully():
    client = AsyncMock(spec=GoloomClient)
    client.list_ai_enabled_teams.return_value = []

    scheduler = ProactiveScheduler(client, poll_seconds=99999)
    await scheduler._tick()

    client.list_ai_enabled_teams.assert_awaited_once()
    client.get_proactive_settings.assert_not_called()
    client.trigger_job.assert_not_called()
