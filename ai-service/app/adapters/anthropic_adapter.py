import httpx

from .base import LLMAdapter
from .models import LLMResponse


class AnthropicAdapter(LLMAdapter):
    async def generate(self, prompt: str, system: str = "", **kwargs) -> LLMResponse:
        payload = {
            "model": kwargs.get("model", self.config.model),
            "max_tokens": kwargs.get("max_tokens", self.config.max_tokens),
            "system": system,
            "messages": [{"role": "user", "content": prompt}],
        }

        async with httpx.AsyncClient(timeout=kwargs.get("timeout", 30.0)) as client:
            response = await client.post(
                self._url("/v1/messages"),
                headers={
                    "x-api-key": self.config.api_key,
                    "anthropic-version": "2023-06-01",
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
            max_tokens=min(self.config.max_tokens, 32),
        )
        return self._match_option(response.content, options)

    def _url(self, path: str) -> str:
        base_url = (self.config.base_url or "https://api.anthropic.com").rstrip("/")
        return f"{base_url}{path}"

    @staticmethod
    def _extract_content(data: dict) -> str:
        content_blocks = data.get("content") or []
        if not content_blocks:
            raise ValueError("Anthropic response missing content")

        text = content_blocks[0].get("text")
        if not isinstance(text, str):
            raise ValueError("Anthropic response missing text content")
        return text.strip()

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
