from __future__ import annotations

from collections.abc import Mapping
from typing import Any

from .templates import (
    format_value,
    render_few_shot_prompt,
    render_generation_prompt,
    render_system_prompt,
)


class PromptBuilder:
    PLATFORM_LIMITS = {
        "mastodon": 500,
        "bluesky": 300,
        "friendica": 5000,
    }

    PLATFORM_HASHTAG_RULES = {
        "mastodon": "Use readable hashtags sparingly, ideally 1-3 at the end.",
        "bluesky": "Use at most 2 concise hashtags only when they add discovery value.",
        "friendica": "Hashtags are allowed, but keep them relevant and non-spammy.",
        "default": "Use hashtags sparingly and only when they are clearly relevant.",
    }

    def build_system_prompt(self, context: dict) -> str:
        style_metadata = self._style_metadata(context)
        team_name = self._get_nested(context, ("team", "name"), ("team", "display_name"), default="unknown team")
        campaign_formats = [
            self._format_campaign_format(item)
            for item in self._get_nested(context, ("campaign_formats",), ("campaignFormats",), default=[])
        ]
        style_examples = [
            self._format_style_example(item)
            for item in self._get_nested(context, ("style_examples",), ("styleExamples",), default=[])
        ]
        recent_posts = [
            self._format_recent_post(item)
            for item in self._get_nested(context, ("recent_posts",), ("recentPosts",), default=[])
        ]

        return render_system_prompt(
            team_name=str(team_name),
            tonality=str(style_metadata.get("tonality") or "neutral"),
            preferred_language=str(style_metadata.get("preferred_language") or "unspecified"),
            max_hashtags=int(style_metadata.get("max_hashtags") or 0),
            formatting_rules=self._string_list(style_metadata.get("formatting_rules")),
            banned_words=self._string_list(style_metadata.get("banned_words")),
            campaign_formats=campaign_formats,
            style_examples=style_examples,
            recent_posts=recent_posts,
        )

    def build_generation_prompt(self, context: dict, params: dict, platform: str) -> str:
        constraints = self.apply_platform_constraints(platform)
        user_request = self._resolve_user_request(params)
        parameter_notes = self._parameter_notes(params)

        return render_generation_prompt(
            system_prompt=self.build_system_prompt(context),
            platform=str(constraints["platform"]),
            char_limit=int(constraints["char_limit"]),
            hashtag_rule=str(constraints["hashtag_rule"]),
            user_request=user_request,
            parameter_notes=parameter_notes,
        )

    def inject_few_shot(self, prompt: str, examples: list) -> str:
        if not examples:
            return prompt

        rendered_examples = [self._format_few_shot_example(index, example) for index, example in enumerate(examples, start=1)]
        return f"{prompt}\n\n{render_few_shot_prompt(rendered_examples)}"

    def apply_platform_constraints(self, platform: str) -> dict:
        normalized = (platform or "").strip().lower()
        key = normalized or "default"
        char_limit = self.PLATFORM_LIMITS.get(normalized, 500)
        hashtag_rule = self.PLATFORM_HASHTAG_RULES.get(normalized, self.PLATFORM_HASHTAG_RULES["default"])
        return {
            "platform": key,
            "char_limit": char_limit,
            "hashtag_rule": hashtag_rule,
        }

    def _style_metadata(self, context: dict) -> dict[str, Any]:
        profile = self._get_nested(context, ("profile",), default={})
        if isinstance(profile, Mapping):
            raw = self._get_nested(profile, ("style_metadata",), ("styleMetadata",), default={})
            if isinstance(raw, Mapping):
                return dict(raw)
        return {}

    def _resolve_user_request(self, params: dict) -> str:
        for key in ("prompt_hint", "content_hint", "request", "prompt", "instruction"):
            value = params.get(key)
            if isinstance(value, str) and value.strip():
                return value.strip()
        return "Create a platform-ready social media draft aligned with the team style."

    def _parameter_notes(self, params: dict) -> list[str]:
        notes: list[str] = []
        for key, value in params.items():
            if key in {"prompt_hint", "content_hint", "request", "prompt", "instruction"}:
                continue
            notes.append(f"{key}: {format_value(value)}")
        return notes

    def _format_campaign_format(self, item: Any) -> str:
        if not isinstance(item, Mapping):
            return str(item)

        name = item.get("name") or "unnamed format"
        hashtags = self._string_list(item.get("required_hashtags") or item.get("requiredHashtags"))
        active = item.get("is_active") if "is_active" in item else item.get("isActive")
        suffix = f"; hashtags={', '.join(hashtags)}" if hashtags else ""
        state = "active" if active is not False else "inactive"
        return f"{name} ({state}{suffix})"

    def _format_style_example(self, item: Any) -> str:
        if not isinstance(item, Mapping):
            return str(item)

        platform = item.get("platform") or "any"
        content = str(item.get("content") or "").strip() or "no content"
        notes = str(item.get("notes") or "").strip()
        if notes:
            return f"[{platform}] {content} (notes: {notes})"
        return f"[{platform}] {content}"

    def _format_recent_post(self, item: Any) -> str:
        if not isinstance(item, Mapping):
            return str(item)
        content = str(item.get("content") or "").strip()
        if content:
            return content
        title = str(item.get("title") or "").strip()
        return title or "Untitled recent post"

    def _format_few_shot_example(self, index: int, example: Any) -> str:
        if isinstance(example, Mapping):
            input_text = str(example.get("input") or example.get("prompt") or "").strip()
            output_text = str(example.get("output") or example.get("response") or example.get("content") or "").strip()
            notes = str(example.get("notes") or "").strip()
            parts = [f"Example {index}:"]
            if input_text:
                parts.append(f"Input: {input_text}")
            if output_text:
                parts.append(f"Output: {output_text}")
            if notes:
                parts.append(f"Notes: {notes}")
            return " | ".join(parts)
        return f"Example {index}: {example}"

    def _string_list(self, value: Any) -> list[str]:
        if not value:
            return []
        if isinstance(value, list):
            return [str(item) for item in value if str(item).strip()]
        return [str(value)]

    def _get_nested(self, source: Any, *paths: tuple[str, ...], default: Any) -> Any:
        for path in paths:
            current = source
            found = True
            for key in path:
                if not isinstance(current, Mapping) or key not in current:
                    found = False
                    break
                current = current[key]
            if found:
                return current
        return default
