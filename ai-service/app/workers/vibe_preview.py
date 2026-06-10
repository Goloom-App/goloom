from __future__ import annotations

import json
import logging

from app.adapters import LLMAdapter
from app.prompts import PromptBuilder
from app.services import GoloomClient

logger = logging.getLogger(__name__)


class VibePreviewWorker:
    def __init__(self, adapter: LLMAdapter, goloom_client: GoloomClient, prompt_builder: PromptBuilder):
        self.adapter = adapter
        self.goloom_client = goloom_client
        self.prompt_builder = prompt_builder

    async def process(self, job: dict) -> dict:
        job_id = str(job.get("job_id") or "")
        author_user_id = str(job["author_user_id"])
        context = job.get("context") or {}

        try:
            prompt = self.prompt_builder.build_vibe_preview_prompt(context)
            response = await self.adapter.generate(
                prompt,
                "You summarize brand voice profiles concisely.",
                model=self.adapter.config.model,
                temperature=0.5,
                max_tokens=300,
                author_user_id=author_user_id,
            )
            parsed = self._parse_result(response.content)
            await self._try_callback(job_id, "completed", parsed)
            return parsed
        except Exception as exc:
            await self._try_callback(job_id, "failed", {}, error_message=str(exc))
            raise

    async def _try_callback(self, job_id: str, status: str, result: dict, error_message: str = "") -> None:
        try:
            await self.goloom_client.send_callback(job_id, status, result, error_message)
        except Exception:
            pass

    @staticmethod
    def _parse_result(raw_content: str) -> dict:
        cleaned = raw_content.strip()
        if cleaned.startswith("```"):
            cleaned = cleaned.split("\n", 1)[-1]
            cleaned = cleaned.rsplit("```", 1)[0]
        cleaned = cleaned.strip()
        payload = json.loads(cleaned)
        if not isinstance(payload, dict):
            raise ValueError("LLM response must be a JSON object")
        summary = str(payload.get("summary") or "").strip()
        if not summary:
            raise ValueError("LLM response missing summary")
        return {
            "summary": summary,
            "suggestion": str(payload.get("suggestion") or "").strip(),
        }
