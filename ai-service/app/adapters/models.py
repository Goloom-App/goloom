from typing import Any

from pydantic import BaseModel, Field


class LLMConfig(BaseModel):
    provider: str
    model: str
    api_key: str
    base_url: str | None = None
    temperature: float = 0.7
    max_tokens: int = 1000


class LLMResponse(BaseModel):
    content: str
    model: str
    usage: dict[str, Any] = Field(default_factory=dict)
