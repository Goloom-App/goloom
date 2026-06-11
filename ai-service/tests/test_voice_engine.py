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
        "accounts": [
            {"id": "acc-bluesky", "provider": "bluesky", "username": "@launch", "max_chars": 300},
            {"id": "acc-mastodon", "provider": "mastodon", "username": "@launch", "max_chars": 500},
        ],
    }


def sample_job(**params) -> dict:
    return {
        "job_id": "job-1",
        "type": "voice_engine",
        "team_id": "team-1",
        "author_user_id": "user-1",
        "params": {
            "prompt_hint": "Announce the release.",
            "target_account_ids": ["acc-bluesky", "acc-mastodon"],
            **params,
        },
    }


def _long_primary_text(length: int = 480) -> str:
    base = "We shipped the release today. Thanks for following along. "
    return (base * ((length // len(base)) + 1))[:length]


def _generation_payload(**overrides):
    payload = {
        "content": _long_primary_text(480),
        "title": "Release update",
        "account_content_override": {"acc-bluesky": "Release shipped today."},
        "hashtags": ["#launch"],
        "platform_metadata": {"platform": "mastodon"},
    }
    payload.update(overrides)
    return payload


@pytest.mark.asyncio
async def test_process_generates_multi_account_post():
    adapter = AsyncMock()
    adapter.config = LLMConfig(provider="openai", model="gpt-4o", api_key="test-key")
    primary_text = _long_primary_text(480)
    adapter.generate.return_value = LLMResponse(
        content=json.dumps(
            _generation_payload(
                content=primary_text,
                title="Release day",
                account_content_override={
                    "acc-bluesky": "Release shipped today.",
                    "acc-mastodon": "should be stripped",
                },
            )
        ),
        model="gpt-4o",
        usage={},
    )
    goloom_client = AsyncMock()
    worker = VoiceEngineWorker(adapter, goloom_client, PromptBuilder())

    result = await worker.process({**sample_job(), "context": sample_context()})

    assert result["content"].startswith(primary_text)
    assert "#launch" in result["content"]
    assert result.get("title")
    assert result["account_content_override"] == {"acc-bluesky": "Release shipped today."}
    assert result["primary_account_id"] == "acc-mastodon"
    assert result["scheduled_at"] is not None
    first_call = adapter.generate.await_args_list[0]
    assert "Brand voice:" in first_call.args[1]
    assert "Quality bar:" in first_call.args[1]
    assert "## Brand voice for this post" not in first_call.args[0]
    assert "## Task" in first_call.args[0]
    goloom_client.send_callback.assert_awaited_once()
    callback_args = goloom_client.send_callback.await_args.args
    assert callback_args[0] == "job-1"
    assert callback_args[1] == "completed"


def test_merge_hashtags_into_content_appends_missing_tags_within_limit():
    parsed = VoiceEngineWorker._merge_hashtags_into_content(
        {"content": "New blog post is live.", "hashtags": ["#tech", "#update"]},
        max_hashtags=7,
        char_limit=500,
    )

    assert "#tech" in parsed["content"]
    assert "#update" in parsed["content"]
    assert parsed["hashtags"] == ["#tech", "#update"]


@pytest.mark.asyncio
async def test_process_refine_mode_skips_minimum_length():
    adapter = AsyncMock()
    adapter.config = LLMConfig(provider="openai", model="gpt-4o", api_key="test-key")
    refined = "Polished release note with clearer flow."
    adapter.generate.return_value = LLMResponse(
        content=json.dumps(
            {
                "content": refined,
                "account_content_override": {"acc-bluesky": "Release note."},
                "hashtags": [],
                "platform_metadata": {},
            }
        ),
        model="gpt-4o",
        usage={},
    )
    goloom_client = AsyncMock()
    worker = VoiceEngineWorker(adapter, goloom_client, PromptBuilder())

    result = await worker.process(
        {
            **sample_job(
                refine_content=True,
                source_content="Rough draft about the release.",
                prompt_hint="Tighten wording.",
            ),
            "context": sample_context(),
        }
    )

    assert result["content"] == refined
    first_call = adapter.generate.await_args_list[0]
    assert "Previous draft (facts and tone reference only" in first_call.args[0]
    assert adapter.generate.await_count == 1


@pytest.mark.asyncio
async def test_process_refine_mode_includes_announcement_reference_for_main_post():
    adapter = AsyncMock()
    adapter.config = LLMConfig(provider="openai", model="gpt-4o", api_key="test-key")
    refined = "Full episode post aligned with the teaser."
    adapter.generate.return_value = LLMResponse(
        content=json.dumps(
            {
                "content": refined,
                "account_content_override": {},
                "hashtags": [],
                "platform_metadata": {},
            }
        ),
        model="gpt-4o",
        usage={},
    )
    goloom_client = AsyncMock()
    worker = VoiceEngineWorker(adapter, goloom_client, PromptBuilder())

    await worker.process(
        {
            **sample_job(
                refine_content=True,
                source_content="Episode #{counter} drops soon.",
                recurring_post_kind="main",
                announcement_reference_title="Reminder",
                announcement_reference_content="Coming Friday on {main_weekday_name}.",
                main_event_at="2026-06-13T10:00:00Z",
            ),
            "context": sample_context(),
        }
    )

    prompt = adapter.generate.await_args_list[0].args[0]
    assert "Paired announcement to stay consistent with" in prompt
    assert "Coming Friday" in prompt
    assert "MAIN EVENT" in prompt
    assert "## Publication plan" in prompt


@pytest.mark.asyncio
async def test_process_refine_mode_coerces_invalid_account_content_override():
    adapter = AsyncMock()
    adapter.config = LLMConfig(provider="openai", model="gpt-4o", api_key="test-key")
    refined = "Polished release note with clearer flow."
    adapter.generate.return_value = LLMResponse(
        content=json.dumps(
            {
                "content": refined,
                "account_content_override": "none",
                "hashtags": "not-a-list",
                "platform_metadata": [],
            }
        ),
        model="gpt-4o",
        usage={},
    )
    goloom_client = AsyncMock()
    worker = VoiceEngineWorker(adapter, goloom_client, PromptBuilder())

    result = await worker.process(
        {
            **sample_job(
                refine_content=True,
                source_content="Rough draft about the release.",
                prompt_hint="Tighten wording.",
                target_account_ids=["acc-mastodon"],
            ),
            "context": sample_context(),
        }
    )

    assert result["content"] == refined
    assert result["account_content_override"] == {}


@pytest.mark.asyncio
async def test_process_coerces_array_account_content_override():
    adapter = AsyncMock()
    adapter.config = LLMConfig(provider="openai", model="gpt-4o", api_key="test-key")
    primary_text = _long_primary_text(480)
    adapter.generate.return_value = LLMResponse(
        content=json.dumps(
            _generation_payload(
                content=primary_text,
                account_content_override=[{"account_id": "acc-bluesky", "content": "Release shipped today."}],
                hashtags=[],
                platform_metadata={},
            )
        ),
        model="gpt-4o",
        usage={},
    )
    goloom_client = AsyncMock()
    worker = VoiceEngineWorker(adapter, goloom_client, PromptBuilder())

    result = await worker.process({**sample_job(), "context": sample_context()})

    assert result["account_content_override"] == {"acc-bluesky": "Release shipped today."}


@pytest.mark.asyncio
async def test_process_retries_when_primary_content_is_too_short():
    adapter = AsyncMock()
    adapter.config = LLMConfig(provider="openai", model="gpt-4o", api_key="test-key")
    adapter.generate.side_effect = [
        LLMResponse(
            content=json.dumps(
                _generation_payload(
                    content="Too short.",
                    account_content_override={"acc-bluesky": "Too short."},
                    hashtags=[],
                    platform_metadata={},
                )
            ),
            model="gpt-4o",
            usage={},
        ),
        LLMResponse(
            content=json.dumps(
                _generation_payload(
                    content=_long_primary_text(470),
                    account_content_override={"acc-bluesky": "Short release note."},
                    hashtags=[],
                    platform_metadata={},
                )
            ),
            model="gpt-4o",
            usage={},
        ),
    ]
    goloom_client = AsyncMock()
    worker = VoiceEngineWorker(adapter, goloom_client, PromptBuilder())

    result = await worker.process({**sample_job(), "context": sample_context()})

    assert len(result["content"]) >= 425
    assert adapter.generate.await_count == 2
    second_call = adapter.generate.await_args_list[1]
    assert "too short" in second_call.args[0].casefold()


@pytest.mark.asyncio
async def test_process_fits_primary_content_to_char_limit():
    adapter = AsyncMock()
    adapter.config = LLMConfig(provider="openai", model="gpt-4o", api_key="test-key")
    adapter.generate.return_value = LLMResponse(
        content=json.dumps(_generation_payload(content="x" * 600, account_content_override={"acc-bluesky": "short"})),
        model="gpt-4o",
        usage={},
    )
    goloom_client = AsyncMock()
    worker = VoiceEngineWorker(adapter, goloom_client, PromptBuilder())

    result = await worker.process({**sample_job(), "context": sample_context()})

    assert len(result["content"]) == 500
    assert adapter.generate.await_count == 1


@pytest.mark.asyncio
async def test_process_refine_mode_fits_oversized_primary_content():
    adapter = AsyncMock()
    adapter.config = LLMConfig(provider="openai", model="gpt-4o", api_key="test-key")
    adapter.generate.return_value = LLMResponse(
        content=json.dumps(
            {
                "content": "word " * 140,
                "account_content_override": {},
                "hashtags": [],
                "platform_metadata": {},
            }
        ),
        model="gpt-4o",
        usage={},
    )
    goloom_client = AsyncMock()
    worker = VoiceEngineWorker(adapter, goloom_client, PromptBuilder())

    result = await worker.process(
        {
            **sample_job(
                refine_content=True,
                source_content="Rough draft about the release.",
                target_account_ids=["acc-mastodon"],
            ),
            "context": sample_context(),
        }
    )

    assert len(result["content"]) <= 500
    assert result["content"]
    assert adapter.generate.await_count == 1


@pytest.mark.asyncio
async def test_process_retries_when_override_missing_for_lower_limit_account():
    adapter = AsyncMock()
    adapter.config = LLMConfig(provider="openai", model="gpt-4o", api_key="test-key")
    adapter.generate.side_effect = [
        LLMResponse(
            content=json.dumps(_generation_payload(content=_long_primary_text(480), account_content_override={})),
            model="gpt-4o",
            usage={},
        ),
        LLMResponse(
            content=json.dumps(
                _generation_payload(
                    content=_long_primary_text(480),
                    account_content_override={"acc-bluesky": "Short release note."},
                )
            ),
            model="gpt-4o",
            usage={},
        ),
    ]
    goloom_client = AsyncMock()
    worker = VoiceEngineWorker(adapter, goloom_client, PromptBuilder())

    result = await worker.process({**sample_job(), "context": sample_context()})

    assert len(result["content"]) >= 480
    assert "#launch" in result["content"]
    assert result["account_content_override"] == {"acc-bluesky": "Short release note."}
    assert adapter.generate.await_count == 2
    second_call = adapter.generate.await_args_list[1]
    assert "missing override" in second_call.args[0].casefold()


@pytest.mark.asyncio
async def test_process_sends_failure_callback_on_llm_error():
    adapter = AsyncMock()
    adapter.config = LLMConfig(provider="openai", model="gpt-4o", api_key="test-key")
    adapter.generate.side_effect = RuntimeError("LLM exploded")
    goloom_client = AsyncMock()
    worker = VoiceEngineWorker(adapter, goloom_client, PromptBuilder())

    with pytest.raises(RuntimeError, match="LLM exploded"):
        await worker.process({**sample_job(), "context": sample_context()})

    goloom_client.send_callback.assert_awaited_once_with("job-1", "failed", {}, "LLM exploded")


@pytest.mark.asyncio
async def test_router_dispatches_voice_engine_jobs():
    config = Settings(
        goloom_api_url="http://goloom.test",
        goloom_api_token="secret-token",
        llm_generator_provider="openai",
        llm_generator_model="gpt-4o",
    )
    router = JobRouter(config)
    router.workers["voice_engine"].process = AsyncMock(
        return_value={
            "content": "ok",
            "hashtags": [],
            "platform_metadata": {},
            "account_content_override": {},
            "scheduled_at": "2026-06-09T09:00:00Z",
            "primary_account_id": "acc-mastodon",
        }
    )

    result = await router.route(sample_job())

    assert result["content"] == "ok"
    router.workers["voice_engine"].process.assert_awaited_once()
