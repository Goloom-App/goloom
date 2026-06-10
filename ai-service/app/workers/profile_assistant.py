from __future__ import annotations

import json
import logging
from typing import Any

from app.adapters import LLMAdapter
from app.prompts import PromptBuilder
from app.services import GoloomClient

logger = logging.getLogger(__name__)


class ProfileAssistantWorker:
    """Generate a draft brand profile from a short user brief."""

    def __init__(self, adapter: LLMAdapter, goloom_client: GoloomClient, prompt_builder: PromptBuilder):
        self.adapter = adapter
        self.goloom_client = goloom_client
        self.prompt_builder = prompt_builder

    async def process(self, job: dict) -> dict:
        job_id = str(job.get("job_id") or "")
        author_user_id = str(job["author_user_id"])
        params = job.get("params") or {}
        if not isinstance(params, dict):
            raise ValueError("job params must be an object")

        try:
            prompt = self.prompt_builder.build_profile_assistant_prompt(params)
            system_prompt = (
                "You are a senior social media strategist. You write brand profiles "
                "that sound like the actual person or team, not like AI marketing copy."
            )
            response = await self.adapter.generate(
                prompt,
                system_prompt,
                model=self.adapter.config.model,
                temperature=0.6,
                max_tokens=1200,
                author_user_id=author_user_id,
            )
            proposal = self._parse_result(response.content)
            result = {"proposed_profile": proposal}
            await self._try_callback(job_id, "completed", result)
            return result
        except Exception as exc:
            await self._try_callback(job_id, "failed", {}, error_message=str(exc))
            raise

    async def _try_callback(self, job_id: str, status: str, result: dict, error_message: str = "") -> None:
        try:
            await self.goloom_client.send_callback(job_id, status, result, error_message)
        except Exception:
            pass

    @staticmethod
    def _parse_result(raw_content: str) -> dict[str, Any]:
        cleaned = raw_content.strip()
        if cleaned.startswith("```"):
            cleaned = cleaned.split("\n", 1)[-1]
            cleaned = cleaned.rsplit("```", 1)[0]
        cleaned = cleaned.strip()
        try:
            payload = json.loads(cleaned)
        except json.JSONDecodeError:
            start = cleaned.find("{")
            end = cleaned.rfind("}")
            if start == -1 or end == -1:
                raise ValueError("LLM response was not valid JSON")
            payload = json.loads(cleaned[start : end + 1])
        if not isinstance(payload, dict):
            raise ValueError("LLM response must be a JSON object")
        return payload
