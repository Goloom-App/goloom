import json
from datetime import UTC, datetime
from importlib import import_module
from unittest.mock import AsyncMock

import pytest

from app.adapters.models import LLMConfig, LLMResponse
from app.config import Settings
from app.prompts import PromptBuilder
from app.workers.router import JobRouter

CampaignWorker = import_module("app.workers.campaign").CampaignWorker


def sample_context() -> dict:
    return {
        "team": {
            "id": "team-1",
            "name": "Launch Crew",
            "scheduling_preferences": {
                "timezone": "UTC",
                "posting_windows": [{"weekday": 2, "start": "11:30", "end": "13:00"}],
                "default_timeslots": ["09:00"],
            },
        },
        "profile": {
            "style_metadata": {
                "tonality": "helpful",
                "formatting_rules": ["Lead with the value"],
                "max_hashtags": 2,
                "preferred_language": "en",
            }
        },
        "campaign_formats": [
            {
                "id": "fmt-tools-day",
                "name": "Tools Day",
                "weekday": 2,
                "structure": {
                    "headline": "{weekday_name} toolkit spotlight",
                    "cta": "Share one practical tool before {month}/{day}.",
                    "topic": "{topic}",
                },
                "required_hashtags": ["#ToolsDay"],
                "is_active": True,
            }
        ],
        "style_examples": [
            {"platform": "mastodon", "content": "One useful tool, one clear tip.", "notes": "practical"}
        ],
        "recent_posts": [{"content": "Yesterday we shared a workflow shortcut."}],
    }


def sample_job(**params) -> dict:
    return {
        "job_id": "job-1",
        "type": "campaign_autopilot",
        "team_id": "team-1",
        "author_user_id": "user-1",
        "context": sample_context(),
        "params": {"campaign_format_id": "fmt-tools-day", "topic": "automation", **params},
    }


@pytest.mark.asyncio
async def test_campaign_worker_applies_template_and_finds_slot():
    adapter = AsyncMock()
    adapter.config = LLMConfig(provider="openai", model="gpt-4o", api_key="test-key")
    adapter.generate.return_value = LLMResponse(
        content=json.dumps(
            {
                "content": "Tue toolkit spotlight: three automation helpers to save your team time. #ToolsDay",
                "hashtags": ["#ToolsDay"],
            }
        ),
        model="gpt-4o",
        usage={},
    )
    goloom_client = AsyncMock()
    worker = CampaignWorker(adapter, goloom_client, PromptBuilder())
    worker._utcnow = lambda: datetime(2026, 6, 1, 8, 0, tzinfo=UTC)

    result = await worker.process(sample_job())

    assert result == {
        "content": "Tue toolkit spotlight: three automation helpers to save your team time. #ToolsDay",
        "hashtags": ["#ToolsDay"],
        "suggested_scheduled_at": "2026-06-02T11:30:00Z",
    }
    first_call = adapter.generate.await_args_list[0]
    assert "Tue toolkit spotlight" in first_call.args[0]
    assert "Share one practical tool before 06/02." in first_call.args[0]
    assert "Suggested schedule: 2026-06-02T11:30:00Z" in first_call.args[0]
    goloom_client.send_callback.assert_awaited_once_with("job-1", "completed", result, "")


@pytest.mark.asyncio
async def test_campaign_worker_ensures_required_hashtags_are_included():
    adapter = AsyncMock()
    adapter.config = LLMConfig(provider="openai", model="gpt-4o", api_key="test-key")
    adapter.generate.return_value = LLMResponse(
        content=json.dumps(
            {
                "content": "Automation teams can save an hour a day with one reliable tool stack.",
                "hashtags": [],
            }
        ),
        model="gpt-4o",
        usage={},
    )
    goloom_client = AsyncMock()
    worker = CampaignWorker(adapter, goloom_client, PromptBuilder())

    result = await worker.process(sample_job(target_date="2026-06-10"))

    assert "#ToolsDay" in result["content"]
    assert result["hashtags"] == ["#ToolsDay"]
    assert result["suggested_scheduled_at"] == "2026-06-10T11:30:00Z"


@pytest.mark.asyncio
async def test_campaign_worker_sends_failure_callback_on_llm_error():
    adapter = AsyncMock()
    adapter.config = LLMConfig(provider="openai", model="gpt-4o", api_key="test-key")
    adapter.generate.side_effect = RuntimeError("LLM exploded")
    goloom_client = AsyncMock()
    worker = CampaignWorker(adapter, goloom_client, PromptBuilder())

    with pytest.raises(RuntimeError, match="LLM exploded"):
        await worker.process(sample_job())

    goloom_client.send_callback.assert_awaited_once_with("job-1", "failed", {}, "LLM exploded")


@pytest.mark.asyncio
async def test_router_dispatches_campaign_autopilot_jobs():
    config = Settings(
        goloom_api_url="http://goloom.test",
        goloom_api_token="secret-token",
        llm_generator_provider="openai",
        llm_generator_model="gpt-4o",
    )
    router = JobRouter(config)
    router.workers["campaign_autopilot"].process = AsyncMock(
        return_value={"content": "ok", "hashtags": ["#ToolsDay"], "suggested_scheduled_at": None}
    )

    result = await router.route(sample_job())

    assert result == {"content": "ok", "hashtags": ["#ToolsDay"], "suggested_scheduled_at": None}
    router.workers["campaign_autopilot"].process.assert_awaited_once()
