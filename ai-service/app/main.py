from __future__ import annotations

import logging
from typing import Any

from fastapi import BackgroundTasks, FastAPI, status

from app.config import settings
from app.prompts import PromptBuilder
from app.workers import JobRouter

logging.basicConfig(level=getattr(logging, settings.log_level.upper(), logging.INFO), format="%(levelname)s:%(name)s:%(message)s")

app = FastAPI(title="AI Service", version="0.1.0")
job_router = JobRouter(settings)
prompt_builder = PromptBuilder()


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.post("/api/v1/jobs", status_code=status.HTTP_202_ACCEPTED)
async def create_job(job: dict[str, Any], background_tasks: BackgroundTasks):
    background_tasks.add_task(job_router.route, dict(job))
    return {"status": "accepted", "job_id": job.get("job_id")}


@app.post("/api/v1/prompt-preview")
async def prompt_preview(payload: dict[str, Any]):
    context = payload.get("context") or {}
    params = payload.get("params") or {}
    platform = str(params.get("platform") or "mastodon")
    system_prompt = prompt_builder.build_system_prompt(context)
    generation_prompt = prompt_builder.build_generation_prompt(context, params, platform)
    return {
        "system_prompt": system_prompt,
        "generation_prompt": generation_prompt,
    }
