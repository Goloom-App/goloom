from unittest.mock import AsyncMock, patch

import pytest

from app.adapters.anthropic_adapter import AnthropicAdapter
from app.adapters.factory import create_adapter
from app.adapters.models import LLMConfig
from app.adapters.openai_adapter import OpenAIAdapter


class MockResponse:
    def __init__(self, payload: dict):
        self.payload = payload

    def raise_for_status(self):
        return None

    def json(self) -> dict:
        return self.payload


@pytest.mark.asyncio
async def test_openai_generate():
    adapter = OpenAIAdapter(
        LLMConfig(provider="openai", model="gpt-4o-mini", api_key="test-key")
    )
    response_payload = {
        "model": "gpt-4o-mini",
        "choices": [{"message": {"content": "generated text"}}],
        "usage": {"prompt_tokens": 12, "completion_tokens": 4},
    }

    with patch("app.adapters.openai_adapter.httpx.AsyncClient") as mock_client_cls:
        mock_client = mock_client_cls.return_value.__aenter__.return_value
        mock_client.post = AsyncMock(return_value=MockResponse(response_payload))

        response = await adapter.generate("write a post", system="be helpful")

    assert response.content == "generated text"
    assert response.model == "gpt-4o-mini"
    assert response.usage == {"prompt_tokens": 12, "completion_tokens": 4}
    mock_client.post.assert_awaited_once()


@pytest.mark.asyncio
async def test_anthropic_generate():
    adapter = AnthropicAdapter(
        LLMConfig(provider="anthropic", model="claude-3-5-haiku-latest", api_key="test-key")
    )
    response_payload = {
        "model": "claude-3-5-haiku-latest",
        "content": [{"text": "anthropic text"}],
        "usage": {"input_tokens": 7, "output_tokens": 3},
    }

    with patch("app.adapters.anthropic_adapter.httpx.AsyncClient") as mock_client_cls:
        mock_client = mock_client_cls.return_value.__aenter__.return_value
        mock_client.post = AsyncMock(return_value=MockResponse(response_payload))

        response = await adapter.generate("summarize", system="be concise")

    assert response.content == "anthropic text"
    assert response.model == "claude-3-5-haiku-latest"
    assert response.usage == {"input_tokens": 7, "output_tokens": 3}
    mock_client.post.assert_awaited_once()


@pytest.mark.asyncio
async def test_openai_classify_matches_option():
    adapter = OpenAIAdapter(
        LLMConfig(provider="openai", model="gpt-4o-mini", api_key="test-key")
    )
    response_payload = {
        "choices": [{"message": {"content": "approved"}}],
        "usage": {},
    }

    with patch("app.adapters.openai_adapter.httpx.AsyncClient") as mock_client_cls:
        mock_client = mock_client_cls.return_value.__aenter__.return_value
        mock_client.post = AsyncMock(return_value=MockResponse(response_payload))

        result = await adapter.classify("status?", options=["approved", "rejected"])

    assert result == "approved"
    mock_client.post.assert_awaited_once()


@pytest.mark.asyncio
async def test_anthropic_classify_matches_option_case_insensitive():
    adapter = AnthropicAdapter(
        LLMConfig(provider="anthropic", model="claude-3-5-haiku-latest", api_key="test-key")
    )
    response_payload = {
        "content": [{"text": "Approved"}],
        "usage": {},
    }

    with patch("app.adapters.anthropic_adapter.httpx.AsyncClient") as mock_client_cls:
        mock_client = mock_client_cls.return_value.__aenter__.return_value
        mock_client.post = AsyncMock(return_value=MockResponse(response_payload))

        result = await adapter.classify("status?", options=["approved", "rejected"])

    assert result == "approved"
    mock_client.post.assert_awaited_once()


def test_factory_openai():
    adapter = create_adapter("openai", "test-key", "gpt-4o-mini")
    assert isinstance(adapter, OpenAIAdapter)
    assert adapter.config.provider == "openai"


def test_factory_anthropic():
    adapter = create_adapter("anthropic", "test-key", "claude-3-5-haiku-latest")
    assert isinstance(adapter, AnthropicAdapter)
    assert adapter.config.provider == "anthropic"


def test_factory_unknown():
    with pytest.raises(ValueError, match="Unknown provider: unknown"):
        create_adapter("unknown", "test-key", "model")
