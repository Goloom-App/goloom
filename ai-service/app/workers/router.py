from __future__ import annotations

import importlib
import logging

from app.adapters import create_adapter
from app.config import Settings
from app.prompts import PromptBuilder
from app.services import GoloomClient
from app.workers.voice_engine import VoiceEngineWorker
from app.workers.profile_analysis import ProfileAnalysisWorker

logger = logging.getLogger(__name__)


class JobRouter:
    def __init__(self, config: Settings):
        self.config = config
        self.prompt_builder = PromptBuilder()
        self.goloom_client = GoloomClient(config.goloom_api_url, config.goloom_api_token)
        self.adapter = create_adapter(
            config.llm_generator_provider,
            self._resolve_api_key(config),
            config.llm_generator_model,
            self._resolve_base_url(config.llm_generator_provider),
        )
        campaign_worker_cls = importlib.import_module("app.workers.campaign").CampaignWorker
        voice_worker = VoiceEngineWorker(self.adapter, self.goloom_client, self.prompt_builder)
        self.workers = {
            "campaign_autopilot": campaign_worker_cls(self.adapter, self.goloom_client, self.prompt_builder),
            "voice_engine": voice_worker,
            "profile_analysis": ProfileAnalysisWorker(self.adapter, self.goloom_client, self.prompt_builder),
        }

    async def route(self, job: dict) -> dict:
        job_type = str(job.get("type") or "")
        job_id = job.get("job_id", "unknown")
        logger.info("Routing job %s type=%s", job_id, job_type)
        worker = self.workers.get(job_type)
        if worker is None:
            logger.error("Unknown job type: %s", job_type)
            raise ValueError(f"Unknown job type: {job['type']}")
        logger.info("Dispatching job %s to %s worker", job_id, job_type)
        result = await worker.process(job)
        logger.info("Job %s completed: %s", job_id, result)
        return result

    @staticmethod
    def _resolve_api_key(config: Settings) -> str:
        return {
            "openai": config.openai_api_key,
            "anthropic": config.anthropic_api_key,
        }.get(config.llm_generator_provider.lower(), "")

    @staticmethod
    def _resolve_base_url(provider: str) -> str | None:
        return None
