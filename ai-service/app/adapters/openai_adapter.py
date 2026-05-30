import logging

import httpx

from .base import LLMAdapter
from .models import LLMResponse

logger = logging.getLogger(__name__)


class OpenAIAdapter(LLMAdapter):
    async def generate(self, prompt: str, system: str = "", **kwargs) -> LLMResponse:
        payload = {
            "model": kwargs.get("model", self.config.model),
            "messages": [
                {"role": "system", "content": system},
                {"role": "user", "content": prompt},
            ],
            "temperature": kwargs.get("temperature", self.config.temperature),
            "max_tokens": kwargs.get("max_tokens", self.config.max_tokens),
        }

        async with httpx.AsyncClient(timeout=kwargs.get("timeout", 30.0)) as client:
            response = await client.post(
                self._url("/v1/chat/completions"),
                headers={
                    "Authorization": f"Bearer {self.config.api_key}",
                    "Content-Type": "application/json",
                },
                json=payload,
            )
            response.raise_for_status()
            data = response.json()

        return LLMResponse(
            content=self._extract_content(data),
            model=data.get("model", self.config.model),
            usage=data.get("usage") or {},
        )

    async def classify(self, prompt: str, system: str = "", options: list[str] = []) -> str:
        if not options:
            raise ValueError("classification options are required")

        response = await self.generate(
            prompt=(
                f"{prompt}\n\nChoose exactly one option from: {', '.join(options)}. "
                "Respond with the option only."
            ),
            system=system,
            temperature=0,
        )
        return self._match_option(response.content, options)

    def _url(self, path: str) -> str:
        base_url = (self.config.base_url or "https://api.openai.com").rstrip("/")
        return f"{base_url}{path}"

    @staticmethod
    def _extract_content(data: dict) -> str:
        choices = data.get("choices") or []
        if not choices:
            raise ValueError("OpenAI response missing choices")

        message = choices[0].get("message") or {}
        content = message.get("content")
        if not isinstance(content, str):
            raise ValueError("OpenAI response missing message content")
        return content.strip()

    @staticmethod
    def _match_option(content: str, options: list[str]) -> str:
        candidate = content.strip()
        if candidate in options:
            return candidate

        normalized = candidate.strip(" \n\t\"'`.,")
        for option in options:
            if normalized.lower() == option.lower():
                return option

        lowered = normalized.lower()
        for option in options:
            if option.lower() in lowered:
                return option

        raise ValueError(f"model returned unexpected classification: {content}")
