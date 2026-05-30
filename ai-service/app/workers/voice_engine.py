from __future__ import annotations

import json
import logging
from typing import Any

from app.adapters import LLMAdapter
from app.prompts import PromptBuilder
from app.services import GoloomClient

logger = logging.getLogger(__name__)


class VoiceEngineWorker:
    def __init__(self, adapter: LLMAdapter, goloom_client: GoloomClient, prompt_builder: PromptBuilder):
        self.adapter = adapter
        self.goloom_client = goloom_client
        self.prompt_builder = prompt_builder
        self.max_retries = 3

    async def process(self, job: dict) -> dict:
        job_id = str(job.get("job_id") or "")
        callback_sent = False

        try:
            team_id = str(job["team_id"])
            author_user_id = str(job["author_user_id"])
            params = self._params(job)
            context = job.get("context") or {}
            system_prompt = self.prompt_builder.build_system_prompt(context)
            platform = str(params.get("platform") or "mastodon")
            constraints = self.prompt_builder.apply_platform_constraints(platform)
            base_prompt = self.prompt_builder.build_generation_prompt(context, params, platform)
            prompt = self.prompt_builder.inject_few_shot(base_prompt, context.get("style_examples", []))

            result = await self._generate_with_retries(
                prompt=prompt,
                system_prompt=system_prompt,
                char_limit=int(constraints["char_limit"]),
                author_user_id=author_user_id,
            )
            await self._try_callback(job_id, "completed", result)
            callback_sent = True
            return result
        except Exception as exc:
            if not callback_sent:
                await self._try_callback(job_id, "failed", {}, error_message=str(exc))
            raise

    async def _try_callback(self, job_id: str, status: str, result: dict, error_message: str = "") -> None:
        try:
            await self.goloom_client.send_callback(job_id, status, result, error_message)
        except Exception:
            pass

    async def _generate_with_retries(
        self,
        *,
        prompt: str,
        system_prompt: str,
        char_limit: int,
        author_user_id: str,
    ) -> dict:
        current_prompt = prompt
        for attempt in range(self.max_retries + 1):
            response = await self.adapter.generate(
                current_prompt,
                system_prompt,
                model=self.adapter.config.model,
                temperature=0.7,
                max_tokens=1000,
                author_user_id=author_user_id,
            )
            result = self._parse_result(response.content)
            if len(result["content"]) <= char_limit:
                return result

            if attempt >= self.max_retries:
                raise ValueError(f"Generated content exceeds platform limit of {char_limit} characters")

            current_prompt = self._build_retry_prompt(current_prompt, char_limit)

        raise RuntimeError("voice engine retry loop exited unexpectedly")

    @staticmethod
    def _params(job: dict) -> dict[str, Any]:
        params = job.get("params") or {}
        if not isinstance(params, dict):
            raise ValueError("job params must be an object")
        return params

    @staticmethod
    def _parse_result(raw_content: str) -> dict:
        cleaned = raw_content.strip()
        if cleaned.startswith("```"):
            cleaned = cleaned.split("\n", 1)[-1]
            cleaned = cleaned.rsplit("```", 1)[0]
        cleaned = cleaned.strip()

        try:
            payload = json.loads(cleaned)
        except json.JSONDecodeError as exc:
            start = cleaned.find("{")
            end = cleaned.rfind("}")
            if start != -1 and end != -1:
                try:
                    payload = json.loads(cleaned[start : end + 1])
                except json.JSONDecodeError:
                    raise ValueError("LLM response was not valid JSON") from exc
            else:
                raise ValueError("LLM response was not valid JSON") from exc

        if not isinstance(payload, dict):
            raise ValueError("LLM response must be a JSON object")

        content = payload.get("content")
        hashtags = payload.get("hashtags") or []
        platform_metadata = payload.get("platform_metadata") or {}

        if not isinstance(content, str) or not content.strip():
            raise ValueError("LLM response missing content")
        if not isinstance(hashtags, list):
            raise ValueError("LLM response hashtags must be a list")
        if not isinstance(platform_metadata, dict):
            raise ValueError("LLM response platform_metadata must be an object")

        return {
            "content": content.strip(),
            "hashtags": [str(hashtag) for hashtag in hashtags],
            "platform_metadata": platform_metadata,
        }

    @staticmethod
    def _build_retry_prompt(prompt: str, char_limit: int) -> str:
        return (
            f"{prompt}\n\n"
            "Revise the previous answer and return JSON only. "
            f"The content field must be at most {char_limit} characters while preserving the core message."
        )


