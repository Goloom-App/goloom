# ai-service

## Purpose

Python FastAPI microservice for AI-powered content generation. Handles LLM integration, prompt building, voice engine, campaign autopilot, profile analysis, proactive triggers, and RSS content pipeline.

## Ownership

Separate Python service with its own build system (uv), test framework (pytest), and Dockerfile. Runs as an independent container.

## Local Contracts

- FastAPI app entry: `app/main.py`
- Config via env vars loaded through Pydantic: `app/config.py`
- LLM adapters follow abstract interface in `app/adapters/base.py`
- Job workers registered in `app/workers/router.py`
- Communication with Go backend: `app/services/goloom_client.py` via HTTP

## Work Guidance

- Python 3.11+, type hints required (pyright checked)
- Use `uv` for dependency management, never pip directly
- Tests in `tests/` mirror `app/` structure
- LLM adapters must implement `base.py` interface
- Prompt templates live in `app/prompts/templates.py`
- Env vars documented in `.env.example`

## Verification

- `make ai-service-test` runs pytest
- `make ai-service-build` builds Docker image
- `uv run pyright` for type checking

## Child DOX Index

- `app/` — Main application package (adapters, models, prompts, workers, services)
- `tests/` — Test suite (mirrors app structure)
