import json
from unittest.mock import AsyncMock

import pytest

from app.adapters.models import LLMConfig, LLMResponse
from app.config import Settings
from app.prompts import PromptBuilder
from app.workers.router import JobRouter
from app.workers.voice_engine import VoiceEngineWorker


def sample_context() -> dict:
    return {
        "team": {"id": "team-1", "name": "Launch Crew"},
        "profile": {
            "style_metadata": {
                "tonality": "clear",
                "formatting_rules": ["Lead with the headline"],
                "max_hashtags": 2,
                "preferred_language": "en",
            }
        },
        "style_examples": [
            {"platform": "bluesky", "content": "Short and helpful updates.", "notes": "warm"}
        ],
        "recent_posts": [{"content": "Yesterday we shipped a fix."}],
    }


def sample_job(**params) -> dict:
    return {
        "id": "job-1",
        "type": "voice_engine",
        "team_id": "team-1",
        "author_user_id": "user-1",
        "params": {"platform": "bluesky", "prompt_hint": "Announce the release.", **params},
    }


@pytest.mark.asyncio
async def test_process_generates_platform_specific_post():
    adapter = AsyncMock()
    adapter.config = LLMConfig(provider="openai", model="gpt-4o", api_key="test-key")
    adapter.generate.return_value = LLMResponse(
        content=json.dumps(
            {
                "content": "We shipped the release today. Thanks for following along.",
                "hashtags": ["#launch"],
                "platform_metadata": {"platform": "bluesky"},
            }
        ),
        model="gpt-4o",
        usage={},
    )
    goloom_client = AsyncMock()
    goloom_client.get_ai_context.return_value = sample_context()
    worker = VoiceEngineWorker(adapter, goloom_client, PromptBuilder())

    result = await worker.process(sample_job())

    assert result == {
        "content": "We shipped the release today. Thanks for following along.",
        "hashtags": ["#launch"],
        "platform_metadata": {"platform": "bluesky"},
    }
    first_call = adapter.generate.await_args_list[0]
    assert "Platform: bluesky" in first_call.args[0]
    assert "Tonality: clear" in first_call.args[1]
    goloom_client.send_callback.assert_awaited_once_with("job-1", "completed", result)


@pytest.mark.asyncio
async def test_process_retries_until_content_fits_char_limit():
    adapter = AsyncMock()
    adapter.config = LLMConfig(provider="openai", model="gpt-4o", api_key="test-key")
    adapter.generate.side_effect = [
        LLMResponse(
            content=json.dumps(
                {
                    "content": "x" * 600,
                    "hashtags": ["#launch"],
                    "platform_metadata": {"platform": "mastodon"},
                }
            ),
            model="gpt-4o",
            usage={},
        ),
        LLMResponse(
            content=json.dumps(
                {
                    "content": "y" * 480,
                    "hashtags": ["#launch"],
                    "platform_metadata": {"platform": "mastodon"},
                }
            ),
            model="gpt-4o",
            usage={},
        ),
    ]
    goloom_client = AsyncMock()
    goloom_client.get_ai_context.return_value = sample_context()
    worker = VoiceEngineWorker(adapter, goloom_client, PromptBuilder())

    result = await worker.process(sample_job(platform="mastodon"))

    assert len(result["content"]) == 480
    assert adapter.generate.await_count == 2
    second_call = adapter.generate.await_args_list[1]
    assert "must be at most 500 characters" in second_call.args[0]


@pytest.mark.asyncio
async def test_process_sends_failure_callback_on_llm_error():
    adapter = AsyncMock()
    adapter.config = LLMConfig(provider="openai", model="gpt-4o", api_key="test-key")
    adapter.generate.side_effect = RuntimeError("LLM exploded")
    goloom_client = AsyncMock()
    goloom_client.get_ai_context.return_value = sample_context()
    worker = VoiceEngineWorker(adapter, goloom_client, PromptBuilder())

    with pytest.raises(RuntimeError, match="LLM exploded"):
        await worker.process(sample_job())

    goloom_client.send_callback.assert_awaited_once_with(
        "job-1", "failed", {}, error_message="LLM exploded"
    )


@pytest.mark.asyncio
async def test_router_dispatches_voice_engine_jobs():
    config = Settings(
        goloom_api_url="http://goloom.test",
        goloom_api_token="secret-token",
        llm_generator_provider="openai",
        llm_generator_model="gpt-4o",
    )
    router = JobRouter(config)
    router.workers["voice_engine"].process = AsyncMock(return_value={"content": "ok", "hashtags": [], "platform_metadata": {}})

    result = await router.route(sample_job())

    assert result == {"content": "ok", "hashtags": [], "platform_metadata": {}}
    router.workers["voice_engine"].process.assert_awaited_once()
