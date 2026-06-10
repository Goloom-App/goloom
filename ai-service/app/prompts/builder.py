from __future__ import annotations

from collections.abc import Mapping
from typing import Any

from .anti_ai import ANTI_AI_STYLE_RULES, merged_banned_words
from .templates import (
    format_value,
    render_few_shot_prompt,
    render_generation_prompt,
    render_profile_assistant_prompt,
    render_system_prompt,
    render_vibe_preview_prompt,
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

    OUTPUT_FORMAT_HINTS = {
        "post": "Standard single post.",
        "teaser": "Short teaser that builds curiosity for linked content.",
        "poll": "Frame as a poll question with 2-4 answer options in the text.",
        "thread": "First post of a thread; hint that more follows.",
    }

    MOOD_ADJUSTMENT_HINTS = {
        "more_expertise": "Emphasize domain expertise and concrete facts from the knowledge base.",
        "shorter_punchier": "Cut length aggressively. Every word must earn its place.",
        "remove_marketing_speak": "Replace hype adjectives with hard facts. Strip every buzzword.",
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
        knowledge_sources = [
            self._format_knowledge_source(item)
            for item in self._get_nested(context, ("knowledge_sources",), ("knowledgeSources",), default=[])
        ]

        dna = self._nested_mapping(style_metadata, "language_dna", "languageDna")
        anti_ai_override = bool(dna.get("anti_ai_override") or dna.get("antiAiOverride") or False)

        banned_words = merged_banned_words(
            self._string_list(style_metadata.get("banned_words")),
            override=anti_ai_override,
        )
        preferred_words = self._string_list(dna.get("preferred_words") or dna.get("preferredWords"))
        signature_phrases = self._string_list(dna.get("signature_phrases") or dna.get("signaturePhrases"))

        return render_system_prompt(
            team_name=str(team_name),
            preferred_language=str(style_metadata.get("preferred_language") or "unspecified"),
            max_hashtags=int(style_metadata.get("max_hashtags") or 0),
            formatting_rules=self._string_list(style_metadata.get("formatting_rules")),
            banned_words=banned_words,
            preferred_words=preferred_words,
            signature_phrases=signature_phrases,
            identity_lines=self._identity_lines(style_metadata),
            language_dna_lines=self._language_dna_lines(style_metadata),
            reach_strategy_lines=self._reach_strategy_lines(style_metadata),
            anti_ai_rules=[] if anti_ai_override else list(ANTI_AI_STYLE_RULES),
            knowledge_sources=knowledge_sources,
            campaign_formats=campaign_formats,
            style_examples=style_examples,
            recent_posts=recent_posts,
        )

    def build_generation_prompt(self, context: dict, params: dict, platform: str) -> str:
        constraints = self.apply_platform_constraints(platform)
        user_request = self._resolve_user_request(params)
        parameter_notes = self._parameter_notes(params)
        output_format = self._output_format_hint(params)
        mood_adjustments = self._mood_adjustments(params)

        return render_generation_prompt(
            system_prompt=self.build_system_prompt(context),
            platform=str(constraints["platform"]),
            char_limit=int(constraints["char_limit"]),
            hashtag_rule=str(constraints["hashtag_rule"]),
            user_request=user_request,
            parameter_notes=parameter_notes,
            output_format=output_format,
            mood_adjustments=mood_adjustments,
        )

    def build_vibe_preview_prompt(self, context: dict) -> str:
        style_metadata = self._style_metadata(context)
        team_name = self._get_nested(context, ("team", "name"), default="unknown team")
        summary_parts = [
            *self._identity_lines(style_metadata),
            *self._language_dna_lines(style_metadata),
            *self._reach_strategy_lines(style_metadata),
            f"Preferred language: {style_metadata.get('preferred_language') or 'unspecified'}",
        ]
        return render_vibe_preview_prompt(
            team_name=str(team_name),
            profile_summary="\n".join(f"- {line}" for line in summary_parts if line),
        )

    def build_profile_assistant_prompt(self, params: dict) -> str:
        brief = str(params.get("brief") or params.get("description") or "").strip()
        if not brief:
            raise ValueError("profile_assistant requires a non-empty brief")
        examples = self._string_list(params.get("examples") or params.get("reference_posts"))
        language = str(params.get("language") or "de").strip() or "de"
        return render_profile_assistant_prompt(brief=brief, examples=examples, language=language)

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

    def _nested_mapping(self, source: Mapping[str, Any], *keys: str) -> dict[str, Any]:
        for key in keys:
            value = source.get(key)
            if isinstance(value, Mapping):
                return dict(value)
        return {}

    def _identity_lines(self, style_metadata: dict[str, Any]) -> list[str]:
        identity = self._nested_mapping(style_metadata, "identity")
        lines: list[str] = []
        if archetype := str(identity.get("archetype") or "").strip():
            lines.append(f"Archetype: {archetype}")
        if persona := str(identity.get("persona") or "").strip():
            lines.append(f"Voice persona: {persona}")
        if industry := str(identity.get("industry") or "").strip():
            lines.append(f"Industry: {industry}")
        if main_value := str(identity.get("main_value") or identity.get("mainValue") or "").strip():
            lines.append(f"Core value proposition: {main_value}")
        if audience := str(identity.get("target_audience") or identity.get("targetAudience") or "").strip():
            lines.append(f"Target audience: {audience}")
        return lines

    def _language_dna_lines(self, style_metadata: dict[str, Any]) -> list[str]:
        dna = self._nested_mapping(style_metadata, "language_dna", "languageDna")
        lines: list[str] = []
        sentence_style = str(dna.get("sentence_style") or dna.get("sentenceStyle") or "").strip()
        if sentence_style:
            lines.append(f"Sentence style: {sentence_style}")
        humor = str(dna.get("humor_style") or dna.get("humorStyle") or "").strip()
        if humor:
            lines.append(f"Humor: {humor}")
        return lines

    def _reach_strategy_lines(self, style_metadata: dict[str, Any]) -> list[str]:
        reach = self._nested_mapping(style_metadata, "reach_strategy", "reachStrategy")
        lines: list[str] = []
        hook = str(reach.get("hook_style") or reach.get("hookStyle") or "").strip()
        if hook:
            lines.append(f"Hook style: {hook}")
        cta = str(reach.get("cta_focus") or reach.get("ctaFocus") or "").strip()
        if cta:
            lines.append(f"CTA focus: {cta}")
        return lines

    def _format_knowledge_source(self, item: Any) -> str:
        if not isinstance(item, Mapping):
            return str(item)
        name = str(item.get("name") or "source").strip()
        content = str(item.get("content") or "").strip()
        if len(content) > 4000:
            content = content[:4000] + "…"
        source_url = str(item.get("source_url") or item.get("sourceUrl") or "").strip()
        prefix = f"[{name}]"
        if source_url:
            prefix += f" ({source_url})"
        return f"{prefix}\n{content or 'No extracted content'}"

    def _output_format_hint(self, params: dict) -> str:
        raw = str(params.get("output_format") or params.get("format") or "").strip().lower()
        return self.OUTPUT_FORMAT_HINTS.get(raw, "")

    def _mood_adjustments(self, params: dict) -> list[str]:
        hints: list[str] = []
        flags = params.get("mood_adjustments") or params.get("moodAdjustments") or []
        if isinstance(flags, list):
            for flag in flags:
                key = str(flag).strip()
                if key in self.MOOD_ADJUSTMENT_HINTS:
                    hints.append(self.MOOD_ADJUSTMENT_HINTS[key])
        for key in ("more_expertise", "shorter_punchier", "remove_marketing_speak"):
            if params.get(key) is True and key in self.MOOD_ADJUSTMENT_HINTS:
                hint = self.MOOD_ADJUSTMENT_HINTS[key]
                if hint not in hints:
                    hints.append(hint)
        return hints

    def _resolve_user_request(self, params: dict) -> str:
        for key in ("occasion", "prompt_hint", "content_hint", "request", "prompt", "instruction"):
            value = params.get(key)
            if isinstance(value, str) and value.strip():
                return value.strip()
        return "Create a platform-ready social media draft aligned with the team style."

    def _parameter_notes(self, params: dict) -> list[str]:
        notes: list[str] = []
        skip = {
            "prompt_hint",
            "content_hint",
            "request",
            "prompt",
            "instruction",
            "occasion",
            "mood_adjustments",
            "moodAdjustments",
            "more_expertise",
            "shorter_punchier",
            "remove_marketing_speak",
        }
        for key, value in params.items():
            if key in skip:
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
            if found and current is not None:
                return current
        return default
