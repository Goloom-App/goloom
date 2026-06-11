from __future__ import annotations

from collections.abc import Mapping
from typing import Any

from .anti_ai import QUALITY_VOICE_PRINCIPLES, capped_banned_words
from .templates import (
    format_value,
    render_brand_voice_prompt,
    render_few_shot_prompt,
    render_generation_prompt,
    render_profile_assistant_prompt,
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
        "post": "Single post — structure is up to you.",
        "teaser": "Short teaser that builds curiosity; hook first, link or CTA second.",
        "poll": "Poll question with 2-4 answer options woven into the text.",
        "thread": "Opening post of a thread; do not summarise the whole thread.",
    }

    MOOD_ADJUSTMENT_HINTS = {
        "more_expertise": "Lean on concrete facts from the source material; show domain depth.",
        "shorter_punchier": "Cut length aggressively. Every word must earn its place.",
        "remove_marketing_speak": "Strip hype adjectives; replace with specifics.",
    }

    BRAND_STYLE_EXAMPLE_LIMIT = 3
    TASK_RECENT_POST_LIMIT = 3
    FORMATTING_RULE_LIMIT = 4
    RECENT_POST_EXCERPT_CHARS = 140

    def build_system_prompt(self, context: dict) -> str:
        return self.build_brand_voice_prompt(context)

    def build_brand_voice_prompt(self, context: dict) -> str:
        style_metadata = self._style_metadata(context)
        team_name = self._get_nested(context, ("team", "name"), ("team", "display_name"), default="unknown team")
        knowledge_sources = [
            self._format_knowledge_source(item)
            for item in self._get_nested(context, ("knowledge_sources",), ("knowledgeSources",), default=[])
        ]
        style_examples = self._limited_style_examples(context)

        dna = self._nested_mapping(style_metadata, "language_dna", "languageDna")
        anti_ai_override = bool(dna.get("anti_ai_override") or dna.get("antiAiOverride") or False)

        banned_words = capped_banned_words(
            self._string_list(style_metadata.get("banned_words")),
            override=anti_ai_override,
        )
        preferred_words = self._string_list(dna.get("preferred_words") or dna.get("preferredWords"))
        signature_phrases = self._string_list(dna.get("signature_phrases") or dna.get("signaturePhrases"))
        formatting_rules = self._string_list(style_metadata.get("formatting_rules"))[: self.FORMATTING_RULE_LIMIT]

        return render_brand_voice_prompt(
            team_name=str(team_name),
            preferred_language=str(style_metadata.get("preferred_language") or "unspecified"),
            max_hashtags=int(style_metadata.get("max_hashtags") or 0),
            voice_summary=self._brand_voice_summary(style_metadata),
            quality_principles=[] if anti_ai_override else list(QUALITY_VOICE_PRINCIPLES),
            formatting_rules=formatting_rules,
            banned_words=banned_words,
            preferred_words=preferred_words,
            signature_phrases=signature_phrases,
            knowledge_sources=knowledge_sources,
            style_examples=style_examples,
        )

    def build_generation_prompt(self, context: dict, params: dict, platform: str) -> str:
        constraints = self.apply_platform_constraints(platform)
        user_request = self._resolve_user_request(params)
        output_format = self._output_format_hint(params)
        mood_adjustments = self._mood_adjustments(params)

        return render_generation_prompt(
            system_prompt=self.build_brand_voice_prompt(context),
            platform=str(constraints["platform"]),
            char_limit=int(constraints["char_limit"]),
            hashtag_rule=str(constraints["hashtag_rule"]),
            user_request=user_request,
            source_material=self._source_material(params),
            recent_posts=self._recent_post_excerpts(context),
            campaign_hint=self._campaign_task_hint(context, params),
            output_format=output_format,
            mood_adjustments=mood_adjustments,
            parameter_notes=self._technical_notes(params),
        )

    def build_vibe_preview_prompt(self, context: dict) -> str:
        style_metadata = self._style_metadata(context)
        team_name = self._get_nested(context, ("team", "name"), default="unknown team")
        return render_vibe_preview_prompt(
            team_name=str(team_name),
            profile_summary=self._brand_voice_summary(style_metadata),
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

    def _brand_voice_summary(self, style_metadata: dict[str, Any]) -> str:
        identity = self._nested_mapping(style_metadata, "identity")
        dna = self._nested_mapping(style_metadata, "language_dna", "languageDna")
        reach = self._nested_mapping(style_metadata, "reach_strategy", "reachStrategy")

        paragraphs: list[str] = []

        persona = str(identity.get("persona") or "").strip()
        archetype = str(identity.get("archetype") or "").strip()
        if persona:
            paragraphs.append(persona)
        elif archetype:
            paragraphs.append(f"This is a {archetype} account.")

        industry = str(identity.get("industry") or "").strip()
        main_value = str(identity.get("main_value") or identity.get("mainValue") or "").strip()
        audience = str(identity.get("target_audience") or identity.get("targetAudience") or "").strip()
        context_bits = [bit for bit in (industry, main_value, audience) if bit]
        if context_bits:
            paragraphs.append(" ".join(context_bits))

        voice_bits: list[str] = []
        sentence_style = str(dna.get("sentence_style") or dna.get("sentenceStyle") or "").strip()
        if sentence_style:
            voice_bits.append(sentence_style)
        humor = str(dna.get("humor_style") or dna.get("humorStyle") or "").strip()
        if humor:
            voice_bits.append(f"Humor: {humor}")
        hook = str(reach.get("hook_style") or reach.get("hookStyle") or "").strip()
        if hook:
            voice_bits.append(f"Hooks: {hook}")
        cta = str(reach.get("cta_focus") or reach.get("ctaFocus") or "").strip()
        if cta:
            voice_bits.append(f"CTAs: {cta}")
        if voice_bits:
            paragraphs.append(" ".join(voice_bits))

        return "\n\n".join(paragraphs)

    def _limited_style_examples(self, context: dict) -> list[str]:
        examples = [
            self._format_style_example_content(item)
            for item in self._get_nested(context, ("style_examples",), ("styleExamples",), default=[])
        ]
        return [item for item in examples if item][: self.BRAND_STYLE_EXAMPLE_LIMIT]

    def _recent_post_excerpts(self, context: dict) -> list[str]:
        excerpts: list[str] = []
        for item in self._get_nested(context, ("recent_posts",), ("recentPosts",), default=[]):
            text = self._format_recent_post(item)
            if not text:
                continue
            compact = " ".join(text.split())
            if len(compact) > self.RECENT_POST_EXCERPT_CHARS:
                compact = compact[: self.RECENT_POST_EXCERPT_CHARS].rstrip() + "…"
            excerpts.append(compact)
            if len(excerpts) >= self.TASK_RECENT_POST_LIMIT:
                break
        return excerpts

    def _has_rss_source(self, params: dict) -> bool:
        return bool(
            str(params.get("rss_article_title") or "").strip()
            or str(params.get("rss_article_content") or params.get("rss_article_summary") or "").strip()
            or str(params.get("rss_article_link") or "").strip()
        )

    def _source_material(self, params: dict) -> list[str]:
        sections: list[str] = []

        rss_title = str(params.get("rss_article_title") or "").strip()
        rss_link = str(params.get("rss_article_link") or "").strip()
        rss_content = str(params.get("rss_article_content") or params.get("rss_article_summary") or "").strip()
        if rss_title or rss_link or rss_content:
            lines = [
                "SHOW NOTES / ARTICLE (primary factual source — every specific claim must come from here):",
            ]
            if rss_title:
                lines.append(f"Title: {rss_title}")
            if rss_link:
                lines.append(f"Link: {rss_link}")
            if rss_content:
                lines.append(f"Text:\n---\n{rss_content}\n---")
            lines.append(
                "The episode title, number, and link above are authoritative. "
                "Pick 2-4 concrete topics from the show notes and work them in naturally. "
                "Do not invent guests, episode numbers, links, or opinions."
            )
            sections.append("\n".join(lines))

        skeleton = str(params.get("post_skeleton") or "").strip()
        source_content = str(params.get("source_content") or params.get("existing_content") or "").strip()
        if not skeleton and source_content and self._has_rss_source(params):
            skeleton = source_content
        if skeleton and self._has_rss_source(params):
            sections.append(
                "RSS post skeleton (optional layout/CTA hints only — not factual content):\n"
                f"---\n{skeleton}\n---"
            )
        elif source_content:
            sections.append(
                "Previous draft (facts and tone reference only — do not copy structure or layout verbatim):\n"
                f"---\n{source_content}\n---"
            )

        announcement = str(params.get("announcement_reference_content") or "").strip()
        announcement_title = str(params.get("announcement_reference_title") or "").strip()
        if announcement or announcement_title:
            lines = ["Paired announcement to stay consistent with:"]
            if announcement_title:
                lines.append(f"Title: {announcement_title}")
            if announcement:
                lines.append(f"Text:\n---\n{announcement}\n---")
            sections.append("\n".join(lines))

        return sections

    def _campaign_task_hint(self, context: dict, params: dict) -> str:
        campaign_format_id = str(params.get("campaign_format_id") or params.get("campaignFormatId") or "").strip()
        if not campaign_format_id:
            return ""

        campaign_formats = self._get_nested(context, ("campaign_formats",), ("campaignFormats",), default=[])
        for item in campaign_formats:
            if not isinstance(item, Mapping):
                continue
            if str(item.get("id") or "") != campaign_format_id:
                continue
            name = str(item.get("name") or "Campaign").strip()
            hashtags = self._string_list(item.get("required_hashtags") or item.get("requiredHashtags"))
            structure = item.get("structure")
            lines = [
                f"Campaign: {name}",
                "Treat any template as a goal, not a form to fill in — vary openings and structure.",
            ]
            if structure:
                lines.append(f"Suggested elements (reorder or adapt freely): {format_value(structure)}")
            if hashtags:
                lines.append(f"Required hashtags: {', '.join(hashtags)}")
            return "\n".join(lines)
        return ""

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
        editorial = ""
        for key in ("occasion", "prompt_hint", "content_hint", "request", "prompt", "instruction"):
            value = params.get(key)
            if isinstance(value, str) and value.strip():
                editorial = value.strip()
                break

        if self._has_rss_source(params):
            base = (
                "Write a new social post promoting this podcast episode.\n"
                "- Use the exact episode title and episode number from the source material.\n"
                "- Use the episode page link from the source — never the RSS feed URL.\n"
                "- Mention 2-3 concrete topics from the show notes; do not invent themes or bump the episode number.\n"
                "- Do not force brand buzzwords (e.g. Open Source) unless they appear in the show notes."
            )
            if editorial:
                return f"{base}\n\nEditorial direction: {editorial}"
            return base

        if editorial:
            return editorial
        return "Write a post that fits this account and the source material."

    def _technical_notes(self, params: dict) -> list[str]:
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
            "source_content",
            "existing_content",
            "post_skeleton",
            "rss_feed_url",
            "refine_content",
            "refine",
            "rss_article_title",
            "rss_article_content",
            "rss_article_summary",
            "rss_article_link",
            "announcement_reference_content",
            "announcement_reference_title",
            "campaign_format_id",
            "campaignFormatId",
            "output_format",
            "format",
            "platform",
        }
        for key, value in params.items():
            if key in skip:
                continue
            if key.endswith("_at") or key.endswith("At"):
                continue
            notes.append(f"{key}: {format_value(value)}")
        return notes

    def _format_style_example_content(self, item: Any) -> str:
        if not isinstance(item, Mapping):
            return str(item).strip()
        return str(item.get("content") or "").strip()

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
        return title or ""

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
