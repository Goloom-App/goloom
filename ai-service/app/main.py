from __future__ import annotations

from typing import Any

from fastapi import BackgroundTasks, FastAPI, status

from app.config import settings
from app.services import GoloomClient
from app.workers import JobRouter

app = FastAPI(title="AI Service", version="0.1.0")
job_router = JobRouter(settings)
goloom_client = GoloomClient(settings.goloom_api_url, settings.goloom_api_token)


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.post("/api/v1/jobs", status_code=status.HTTP_202_ACCEPTED)
async def create_job(job: dict[str, Any], background_tasks: BackgroundTasks):
    background_tasks.add_task(job_router.route, dict(job))
    return {"status": "accepted", "job_id": job.get("id")}
