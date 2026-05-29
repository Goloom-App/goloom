from .anthropic_adapter import AnthropicAdapter
from .base import LLMAdapter
from .models import LLMConfig
from .openai_adapter import OpenAIAdapter


def create_adapter(
    provider: str,
    api_key: str,
    model: str,
    base_url: str | None = None,
) -> LLMAdapter:
    normalized_provider = provider.lower()
    config = LLMConfig(
        provider=normalized_provider,
        model=model,
        api_key=api_key,
        base_url=base_url,
    )
    if normalized_provider == "openai":
        return OpenAIAdapter(config)
    if normalized_provider == "anthropic":
        return AnthropicAdapter(config)
    raise ValueError(f"Unknown provider: {provider}")
