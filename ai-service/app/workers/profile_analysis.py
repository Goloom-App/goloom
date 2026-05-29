from __future__ import annotations

import json
from typing import Any

from app.adapters import LLMAdapter
from app.prompts import PromptBuilder
from app.services import GoloomClient


class ProfileAnalysisWorker:
    def __init__(self, adapter: LLMAdapter, goloom_client: GoloomClient, prompt_builder: PromptBuilder):
        self.adapter = adapter
        self.goloom_client = goloom_client
        self.prompt_builder = prompt_builder

    async def process(self, job: dict) -> dict:
        job_id = str(job.get("id") or "")

        try:
            team_id = str(job["team_id"])
            params = self._params(job)
            post_count = int(params.get("post_count", 20))

            context = await self.goloom_client.get_ai_context(team_id)
            team_name = self._get_team_name(context)
            recent_posts = self._get_recent_posts(context, post_count)

            if not recent_posts:
                raise ValueError("No recent posts available for analysis")

            system_prompt = self.prompt_builder.build_system_prompt(context)
            analysis_prompt = self._build_analysis_prompt(system_prompt, team_name, recent_posts)

            result = await self.adapter.generate(
                prompt=analysis_prompt,
                system_prompt="You are a brand voice analyst. Extract the team's writing style from their recent posts.",
            )

            profile = self._parse_analysis(result)

            await self.goloom_client.update_team_profile(team_id, profile)

            examples = params.get("examples", [])
            for example in examples:
                await self.goloom_client.create_style_example(team_id, example)

            payload = {"profile": profile, "examples_count": len(examples)}
            await self.goloom_client.send_callback(job_id, "completed", payload)
            return payload
        except Exception as exc:
            error_message = str(exc)
            await self.goloom_client.send_callback(job_id, "failed", {}, error_message)
            return {"error": error_message}

    def _params(self, job: dict) -> dict:
        raw = job.get("params") or {}
        if isinstance(raw, str):
            return json.loads(raw)
        return dict(raw)

    def _get_team_name(self, context: dict) -> str:
        team = context.get("team") or {}
        return str(team.get("name") or team.get("display_name") or "Unknown Team")

    def _get_recent_posts(self, context: dict, count: int) -> list[str]:
        posts = context.get("recent_posts") or context.get("recentPosts") or []
        return [str(p.get("content", "")) for p in posts if isinstance(p, dict) and p.get("content")][:count]

    def _build_analysis_prompt(self, system_prompt: str, team_name: str, recent_posts: list[str]) -> str:
        posts_text = "\n\n".join(f"--- Post {i+1} ---\n{post}" for i, post in enumerate(recent_posts))

        return f"""{system_prompt}

Analyze the following recent posts from team "{team_name}" and extract the team's writing style.

For each aspect below, provide your analysis in JSON format:

1. Tonality: What is the overall tone? (e.g., professional, casual, humorous, authoritative)
2. Formatting rules: What formatting patterns do you observe? (e.g., use of line breaks, emoji placement, capitalization style, sentence length)
3. Banned words: Are there any words or phrases the team avoids?
4. Preferred language: What language are posts primarily written in?
5. Max hashtags: How many hashtags are typically used per post?

Recent posts to analyze:
{posts_text}

Respond with ONLY a valid JSON object using this exact structure (no markdown, no code fences):
{{
  "tonality": "description of tonality",
  "formatting_rules": ["rule 1", "rule 2"],
  "banned_words": ["word 1"],
  "preferred_language": "en",
  "max_hashtags": 3
}}"""

    def _parse_analysis(self, raw: str) -> dict:
        cleaned = raw.strip()
        # Strip markdown code fences if present
        if cleaned.startswith("```"):
            cleaned = cleaned.split("\n", 1)[-1]
            cleaned = cleaned.rsplit("```", 1)[0]
        cleaned = cleaned.strip()

        try:
            return json.loads(cleaned)
        except json.JSONDecodeError:
            # Attempt to extract JSON object from the text
            start = cleaned.find("{")
            end = cleaned.rfind("}")
            if start != -1 and end != -1:
                try:
                    return json.loads(cleaned[start : end + 1])
                except json.JSONDecodeError:
                    pass
            return {
                "tonality": cleaned[:200],
                "formatting_rules": [],
                "banned_words": [],
                "preferred_language": "en",
                "max_hashtags": 3,
            }
