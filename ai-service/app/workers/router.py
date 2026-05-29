from __future__ import annotations

import importlib
import os

from app.adapters import create_adapter
from app.config import Settings
from app.prompts import PromptBuilder
from app.services import GoloomClient
from app.workers.voice_engine import VoiceEngineWorker


class JobRouter:
    def __init__(self, config: Settings):
        self.config = config
        self.prompt_builder = PromptBuilder()
        self.goloom_client = GoloomClient(config.goloom_api_url, config.goloom_api_token)
        self.adapter = create_adapter(
            config.llm_generator_provider,
            self._resolve_api_key(config.llm_generator_provider),
            config.llm_generator_model,
            self._resolve_base_url(config.llm_generator_provider),
        )
        campaign_worker_cls = importlib.import_module("app.workers.campaign").CampaignWorker
        self.workers = {
            "campaign_autopilot": campaign_worker_cls(self.adapter, self.goloom_client, self.prompt_builder),
            "voice_engine": VoiceEngineWorker(self.adapter, self.goloom_client, self.prompt_builder),
        }

    async def route(self, job: dict) -> dict:
        job_type = str(job.get("type") or "")
        worker = self.workers.get(job_type)
        if worker is None:
            raise ValueError(f"Unknown job type: {job['type']}")
        return await worker.process(job)

    @staticmethod
    def _resolve_api_key(provider: str) -> str:
        env_name = {
            "openai": "OPENAI_API_KEY",
            "anthropic": "ANTHROPIC_API_KEY",
        }.get(provider.lower())
        return os.getenv(env_name, "") if env_name else ""

    @staticmethod
    def _resolve_base_url(provider: str) -> str | None:
        env_name = {
            "openai": "OPENAI_BASE_URL",
            "anthropic": "ANTHROPIC_BASE_URL",
        }.get(provider.lower())
        if not env_name:
            return None
        value = os.getenv(env_name)
        return value or None
