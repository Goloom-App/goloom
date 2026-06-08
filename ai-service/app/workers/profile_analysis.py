from __future__ import annotations

import json
import logging
import re
from typing import Any

from app.adapters import LLMAdapter
from app.prompts import PromptBuilder
from app.services import GoloomClient

logger = logging.getLogger(__name__)

TOP_STYLE_EXAMPLE_COUNT = 5
MIN_POST_CHARS = 40


class ProfileAnalysisWorker:
    def __init__(self, adapter: LLMAdapter, goloom_client: GoloomClient, prompt_builder: PromptBuilder):
        self.adapter = adapter
        self.goloom_client = goloom_client
        self.prompt_builder = prompt_builder

    async def process(self, job: dict) -> dict:
        job_id = str(job.get("job_id") or "")
        callback_sent = False

        try:
            params = self._params(job)
            post_count = int(params.get("post_count", 20))

            context = job.get("context") or {}
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

            proposed_profile = self._parse_analysis(result.content)
            suggested_examples = self._rank_style_examples(recent_posts)[:TOP_STYLE_EXAMPLE_COUNT]

            payload = {
                "proposed_profile": proposed_profile,
                "suggested_style_examples": suggested_examples,
                "analyzed_post_count": len(recent_posts),
            }
            await self._try_callback(job_id, "completed", payload)
            callback_sent = True
            return payload
        except Exception as exc:
            error_message = str(exc)
            if not callback_sent:
                await self._try_callback(job_id, "failed", {}, error_message)
            return {"error": error_message}

    async def _try_callback(self, job_id: str, status: str, result: dict, error_message: str = "") -> None:
        try:
            await self.goloom_client.send_callback(job_id, status, result, error_message)
        except Exception:
            pass

    def _params(self, job: dict) -> dict:
        raw = job.get("params") or {}
        if isinstance(raw, str):
            return json.loads(raw)
        return dict(raw)

    def _get_team_name(self, context: dict) -> str:
        team = context.get("team") or {}
        return str(team.get("name") or team.get("display_name") or "Unknown Team")

    def _get_recent_posts(self, context: dict, count: int) -> list[dict[str, Any]]:
        posts = context.get("recent_posts") or context.get("recentPosts") or []
        parsed: list[dict[str, Any]] = []
        for item in posts:
            if not isinstance(item, dict):
                continue
            content = str(item.get("content") or "").strip()
            if not content:
                continue
            parsed.append(
                {
                    "id": str(item.get("id") or ""),
                    "content": content,
                    "status": str(item.get("status") or ""),
                    "scheduled_at": str(item.get("scheduled_at") or item.get("scheduledAt") or ""),
                }
            )
        return parsed[:count]

    def _rank_style_examples(self, posts: list[dict[str, Any]]) -> list[dict[str, str]]:
        ranked = sorted(posts, key=self._post_score, reverse=True)
        examples: list[dict[str, str]] = []
        for post in ranked:
            content = post.get("content", "").strip()
            if len(content) < MIN_POST_CHARS:
                continue
            examples.append(
                {
                    "platform": "general",
                    "content": content,
                    "notes": "Suggested from published post analysis",
                    "source_post_id": post.get("id", ""),
                }
            )
            if len(examples) >= TOP_STYLE_EXAMPLE_COUNT:
                break
        return examples

    @staticmethod
    def _post_score(post: dict[str, Any]) -> float:
        content = str(post.get("content") or "")
        words = len(re.findall(r"\w+", content))
        length_bonus = min(len(content), 500) / 10.0
        word_bonus = min(words, 80)
        status_bonus = 20.0 if str(post.get("status") or "").lower() == "posted" else 0.0
        return status_bonus + length_bonus + word_bonus

    def _build_analysis_prompt(self, system_prompt: str, team_name: str, recent_posts: list[dict[str, Any]]) -> str:
        posts_text = "\n\n".join(
            f"--- Post {i + 1} ---\n{post['content']}" for i, post in enumerate(recent_posts)
        )

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
        if cleaned.startswith("```"):
            cleaned = cleaned.split("\n", 1)[-1]
            cleaned = cleaned.rsplit("```", 1)[0]
        cleaned = cleaned.strip()

        try:
            return json.loads(cleaned)
        except json.JSONDecodeError:
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
