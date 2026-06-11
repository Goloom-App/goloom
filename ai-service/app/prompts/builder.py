from __future__ import annotations

from collections.abc import Mapping
from typing import Any

from app.scheduling.slots import format_schedule_label

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
    RECURRING_RECENT_POST_LIMIT = 6
    FORMATTING_RULE_LIMIT = 4
    RECENT_POST_EXCERPT_CHARS = 140

    RECURRING_USER_REQUEST_BASE = (
        "Write a fresh social post from a recurring template.\n"
        "Grounding — every specific claim (dates, times, places, names, prices, offers, "
        "links, numbers, agenda items) must come from one of:\n"
        "- the expanded template below\n"
        "- the editorial direction below (if provided)\n"
        "- the paired announcement reference (if provided)\n"
        "Enhancement — where AI adds value:\n"
        "- sharper wording, rhythm, and CTA; more engaging but faithful copy\n"
        "- a new opening and structure vs. the template and recent posts\n"
        "- persuasive emphasis on what the sources already state (event, campaign, meetup, product, etc.)\n"
        "Do not add factual specifics from brand knowledge, industry profile, or recent posts — "
        "use those only for tone and deduplication."
    )

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
        max_hashtags = int(self._style_metadata(context).get("max_hashtags") or 0)
        constraints = self.apply_platform_constraints(platform, max_hashtags=max_hashtags)
        user_request = self._resolve_user_request(params)
        output_format = self._output_format_hint(params)
        mood_adjustments = self._mood_adjustments(params)

        return render_generation_prompt(
            system_prompt=self.build_brand_voice_prompt(context),
            platform=str(constraints["platform"]),
            char_limit=int(constraints["char_limit"]),
            hashtag_rule=str(constraints["hashtag_rule"]),
            user_request=user_request,
            source_material=self._source_material(params, context),
            recent_posts=self._recent_post_excerpts(
                context,
                limit=self.RECURRING_RECENT_POST_LIMIT
                if self._recurring_post_kind(params) in {"announcement", "main"}
                else self.TASK_RECENT_POST_LIMIT,
            ),
            campaign_hint=self._campaign_task_hint(context, params),
            output_format=output_format,
            mood_adjustments=mood_adjustments,
            parameter_notes=self._technical_notes(params),
            output_constraints=self._output_constraints(context, params),
            recurring_plan=self._recurring_publication_plan(params, context),
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

    def apply_platform_constraints(self, platform: str, *, max_hashtags: int = 0) -> dict:
        normalized = (platform or "").strip().lower()
        key = normalized or "default"
        char_limit = self.PLATFORM_LIMITS.get(normalized, 500)
        hashtag_rule = self._hashtag_rule(normalized, max_hashtags)
        return {
            "platform": key,
            "char_limit": char_limit,
            "hashtag_rule": hashtag_rule,
        }

    def _hashtag_rule(self, platform: str, max_hashtags: int) -> str:
        if max_hashtags > 0:
            minimum = min(3, max_hashtags) if max_hashtags >= 3 else max_hashtags
            return (
                f"Include {minimum} to {max_hashtags} relevant hashtags at the end of the post text "
                "(also mirror them in the hashtags JSON field)."
            )
        return self.PLATFORM_HASHTAG_RULES.get(platform, self.PLATFORM_HASHTAG_RULES["default"])

    def _output_constraints(self, context: dict, params: dict | None = None) -> list[str]:
        constraints: list[str] = []
        style_metadata = self._style_metadata(context)
        german = str(style_metadata.get("preferred_language") or "en").strip().lower().startswith("de")
        if params and self._recurring_post_kind(params) in {"announcement", "main"}:
            constraints.extend(self._recurring_output_constraints(german))
        for rule in self._string_list(style_metadata.get("formatting_rules")):
            if "emoji" in rule.casefold():
                constraints.append(f"Respect this emoji rule: {rule}")

        max_hashtags = int(style_metadata.get("max_hashtags") or 0)
        if max_hashtags > 0:
            if params and self._recurring_post_kind(params) in {"announcement", "main"}:
                constraints.append(self._recurring_hashtag_constraint(max_hashtags, german))
            else:
                constraints.append(
                    f"Hashtags are important for reach — use up to {max_hashtags} relevant tags derived from the source topics."
                )

        constraints.append(
            "Style-note examples (e.g. sample sentence patterns) are not catchphrases — "
            "do not paste them unless they genuinely fit this item."
        )
        return constraints

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

    def _recent_post_excerpts(self, context: dict, *, limit: int | None = None) -> list[str]:
        cap = limit if limit is not None else self.TASK_RECENT_POST_LIMIT
        excerpts: list[str] = []
        for item in self._get_nested(context, ("recent_posts",), ("recentPosts",), default=[]):
            text = self._format_recent_post(item)
            if not text:
                continue
            compact = " ".join(text.split())
            if len(compact) > self.RECENT_POST_EXCERPT_CHARS:
                compact = compact[: self.RECENT_POST_EXCERPT_CHARS].rstrip() + "…"
            excerpts.append(compact)
            if len(excerpts) >= cap:
                break
        return excerpts

    def _has_rss_source(self, params: dict) -> bool:
        return bool(
            str(params.get("rss_article_title") or "").strip()
            or str(params.get("rss_article_content") or params.get("rss_article_summary") or "").strip()
            or str(params.get("rss_article_link") or "").strip()
        )

    def _recurring_post_kind(self, params: dict) -> str:
        kind = str(params.get("recurring_post_kind") or "").strip().lower()
        if kind:
            return kind
        auto = params.get("recurring_automation")
        if isinstance(auto, Mapping):
            return str(auto.get("post_kind") or "").strip().lower()
        return ""

    @staticmethod
    def _recurring_output_constraints(german: bool) -> list[str]:
        if german:
            return [
                "Neuer Text — keine Sätze oder Einstiege aus der Vorlage oder den letzten Posts recyceln.",
                "Keine erfundenen Fakten: Orte, Preise, Rabatte, Agenda-Themen, Produkte oder Gäste, "
                "die nicht in Vorlage, redaktioneller Anweisung oder Ankündigungs-Referenz stehen.",
                "Redaktionelle Anweisung darf Betonung und Winkel steuern — aber keine neuen Fakten einführen.",
                "Brand-Wissen und letzte Posts nur für Ton und Deduplizierung — nicht als Themenquelle.",
            ]
        return [
            "Fresh wording — do not recycle template sentences or recent post openings.",
            "No invented facts: venues, prices, discounts, agenda topics, products, or guests "
            "unless stated in the template, editorial direction, or announcement reference.",
            "Editorial direction may shape emphasis and angle — but cannot introduce new facts.",
            "Brand knowledge and recent posts are for tone and dedup only — not topic sources.",
        ]

    @staticmethod
    def _recurring_hashtag_constraint(max_hashtags: int, german: bool) -> str:
        if german:
            return (
                f"Bis zu {max_hashtags} Hashtags aus Vorlage oder redaktioneller Anweisung — "
                "keine generischen Branchen-/Hobby-Tags aus dem Brand-Profil, "
                "wenn sie nicht wörtlich in diesen Quellen stehen."
            )
        return (
            f"Use up to {max_hashtags} hashtags grounded in the template or editorial direction — "
            "not generic industry/hobby tags from the brand profile unless literal in those sources."
        )

    def _param_schedule_value(self, params: dict, key: str, nested: tuple[str, str]) -> str:
        direct = str(params.get(key) or "").strip()
        if direct:
            return direct
        auto = params.get(nested[0])
        if isinstance(auto, Mapping):
            return str(auto.get(nested[1]) or "").strip()
        return ""

    def _recurring_publication_plan(self, params: dict, context: dict) -> str:
        kind = self._recurring_post_kind(params)
        if kind not in {"announcement", "main"}:
            return ""

        language = str(self._style_metadata(context).get("preferred_language") or "en")
        german = language.strip().lower().startswith("de")
        post_at = self._param_schedule_value(params, "post_scheduled_at", ("recurring_automation", "scheduled_at"))
        main_at = self._param_schedule_value(params, "main_event_at", ("recurring_automation", "template_occurrence_at"))
        post_label = format_schedule_label(post_at, language=language) if post_at else ""
        main_label = format_schedule_label(main_at, language=language) if main_at else ""

        if kind == "announcement":
            lines = [
                (
                    "Rolle: ANKÜNDIGUNG (wird vor dem Event veröffentlicht)."
                    if german
                    else "Role: ANNOUNCEMENT (published before the event)."
                ),
                (
                    "Die Vorlage liefert Fakten und Zeitform (z. B. „Am Freitag …“ statt „heute“) — "
                    "frisch und einladend formulieren, ohne neue Fakten."
                    if german
                    else "The template supplies facts and timing (e.g. a weekday/date vs. “today”) — "
                    "write fresh, inviting copy without adding new facts."
                ),
            ]
        else:
            lines = [
                (
                    "Rolle: HAUPTPOST (wird am Event-Tag veröffentlicht)."
                    if german
                    else "Role: MAIN EVENT (published on the event day)."
                ),
                (
                    "Die Vorlage liefert Fakten und Zeitform (z. B. „heute Abend“) — "
                    "frisch und mitreißend formulieren, ohne neue Fakten."
                    if german
                    else "The template supplies facts and timing (e.g. “tonight”) — "
                    "write fresh, energetic copy without adding new facts."
                ),
            ]

        if post_label:
            lines.append(
                f"Veröffentlichung dieses Posts: {post_label}"
                if german
                else f"This post publishes: {post_label}"
            )
        if main_label and kind == "announcement":
            lines.append(
                f"Event-Datum: {main_label}"
                if german
                else f"Event date: {main_label}"
            )
        return "\n".join(lines)

    def _recurring_template_source(self, source_content: str, context: dict) -> str:
        german = str(self._style_metadata(context).get("preferred_language") or "en").strip().lower().startswith("de")
        if german:
            header = "Wiederkehrende Vorlage (ausgefüllt — Fakten übernehmen, Wortlaut neu schreiben):"
            rules = (
                "Alle Fakten oben beibehalten (Zeit, Ort, Angebot, Links, Zahlen, Namen).\n"
                "Neu formulieren und betonen — keine zusätzlichen Fakten jenseits der Quellen in der Aufgabe."
            )
        else:
            header = "Recurring template (expanded — keep facts, rewrite wording):"
            rules = (
                "Carry over every fact above (timing, venue, offer, links, numbers, names).\n"
                "Rephrase and emphasize — no extra facts beyond the grounding sources in the task."
            )
        return f"{header}\n---\n{source_content}\n---\n{rules}"

    def _source_material(self, params: dict, context: dict | None = None) -> list[str]:
        sections: list[str] = []

        rss_title = str(params.get("rss_article_title") or "").strip()
        rss_link = str(params.get("rss_article_link") or "").strip()
        rss_content = str(params.get("rss_article_content") or params.get("rss_article_summary") or "").strip()
        if rss_title or rss_link or rss_content:
            lines = [
                "RSS ITEM (primary factual source — every specific claim must come from here):",
            ]
            if rss_title:
                lines.append(f"Title: {rss_title}")
            if rss_link:
                lines.append(f"Link: {rss_link}")
            if rss_content:
                lines.append(f"Text:\n---\n{rss_content}\n---")
            lines.append(
                "The title and link above are authoritative. "
                "Pick 2-4 concrete points from the body text and work them in naturally. "
                "Do not invent facts, change numbers/identifiers, or swap the item link for a feed URL."
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
            if self._recurring_post_kind(params) in {"announcement", "main"}:
                sections.append(self._recurring_template_source(source_content, context or {}))
            else:
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
                "Write a new social post based on the RSS feed item below.\n"
                "- Use the exact title from the source (keep any numbers or identifiers).\n"
                "- Use the item link from the source — never the RSS subscription/feed URL.\n"
                "- Mention 2-3 concrete details from the body text; do not invent themes or change identifiers.\n"
                "- Do not force brand buzzwords unless they appear in the source."
            )
            if editorial:
                return f"{base}\n\nEditorial direction: {editorial}"
            return base

        if self._recurring_post_kind(params) in {"announcement", "main"}:
            base = self.RECURRING_USER_REQUEST_BASE
            if editorial:
                return (
                    f"{base}\n\n"
                    "Editorial direction (shapes emphasis and angle — cannot add facts beyond the template):\n"
                    f"{editorial}"
                )
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
            "recurring_post_kind",
            "recurring_automation",
            "post_scheduled_at",
            "main_event_at",
            "days_before_main_event",
            "template_occurrence_at",
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
