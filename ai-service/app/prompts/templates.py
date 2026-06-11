import json
from typing import Any


def render_brand_voice_prompt(
    *,
    team_name: str,
    preferred_language: str,
    max_hashtags: int,
    voice_summary: str,
    quality_principles: list[str],
    formatting_rules: list[str],
    banned_words: list[str],
    preferred_words: list[str],
    signature_phrases: list[str],
    knowledge_sources: list[str],
    style_examples: list[str],
) -> str:
    sections = [
        f'You write social media posts for "{team_name}".',
        "",
        "Brand voice:",
        voice_summary.strip() or "Write clearly and authentically for this account.",
        "",
        "Quality bar:",
        format_list(quality_principles),
    ]

    if formatting_rules:
        sections.extend(
            [
                "",
                "Style notes (soft guidelines — not a checklist; examples show patterns, not phrases to copy verbatim):",
                format_list(formatting_rules),
            ]
        )

    if preferred_words:
        sections.extend(
            [
                "",
                "Words that fit this account (only when relevant to this specific post — never force):",
                format_inline_list(preferred_words),
            ]
        )

    if signature_phrases:
        sections.extend(
            [
                "",
                "Signature phrases (only when they fit perfectly):",
                format_inline_list(signature_phrases),
            ]
        )

    if banned_words:
        sections.extend(
            [
                "",
                "Especially avoid these words/phrases:",
                format_inline_list(banned_words),
            ]
        )

    if knowledge_sources:
        sections.extend(
            [
                "",
                "Brand knowledge base (static facts about us — not about the specific item you are posting):",
                format_list(knowledge_sources),
            ]
        )

    sections.extend(
        [
            "",
            "Posts that sound like us (match tone and attitude, not structure or layout):",
            format_style_examples(style_examples),
            "",
            f"Language: {preferred_language} | Hashtag budget: up to {max_hashtags}",
            "Facts about the specific item you are posting about always come from the source material in the task message.",
        ]
    )

    return "\n".join(sections).strip()


def render_task_prompt(
    *,
    platform: str,
    char_limit: int,
    hashtag_rule: str,
    user_request: str,
    source_material: list[str],
    recent_posts: list[str],
    campaign_hint: str = "",
    output_format: str = "",
    mood_adjustments: list[str] | None = None,
    technical_notes: list[str] | None = None,
    output_constraints: list[str] | None = None,
    recurring_plan: str = "",
) -> str:
    sections: list[str] = [
        "## Task",
        user_request.strip() or "Write a platform-ready post for this account.",
    ]

    if recurring_plan.strip():
        sections.extend(["", "## Publication plan", recurring_plan.strip()])

    if source_material:
        sections.extend(["", "## Source material", *source_material])

    if recent_posts:
        sections.extend(
            [
                "",
                "## Do not repeat",
                "Recent posts below are for deduplication only — do not copy their openings, structure, phrasing, or subject matter.",
                format_list(recent_posts),
            ]
        )

    if campaign_hint.strip():
        sections.extend(["", "## Campaign goal", campaign_hint.strip()])

    format_hint = f"\n- Output shape: {output_format}" if output_format else ""
    mood_hint = ""
    if mood_adjustments:
        mood_hint = "\n\nMood for this draft:\n" + format_list(mood_adjustments)

    sections.extend(
        [
            "",
            "## Platform",
            f"- Platform: {platform}",
            f"- Character limit: {char_limit}",
            f"- Hashtag guidance: {hashtag_rule}{format_hint}",
        ]
    )

    if technical_notes:
        sections.extend(["", "## Technical notes", format_list(technical_notes)])

    if output_constraints:
        sections.extend(["", "## Output constraints", format_list(output_constraints)])

    sections.extend(
        [
            mood_hint,
            "",
            "Respond with a JSON object using this exact structure (no markdown, no code fences):",
            '{"content": "the post text including hashtags at the end", "hashtags": ["hashtag1", "hashtag2"], "platform_metadata": {"key": "value"}}',
            "Hashtags must appear in content, not only in the hashtags array.",
        ]
    )

    return "\n".join(section for section in sections if section is not None).strip()


# Backwards-compatible alias used by tests and external callers.
def render_system_prompt(**kwargs: Any) -> str:
    return render_brand_voice_prompt(**kwargs)


def render_generation_prompt(
    *,
    system_prompt: str = "",
    platform: str,
    char_limit: int,
    hashtag_rule: str,
    user_request: str,
    parameter_notes: list[str] | None = None,
    source_material: list[str] | None = None,
    recent_posts: list[str] | None = None,
    campaign_hint: str = "",
    output_format: str = "",
    mood_adjustments: list[str] | None = None,
    output_constraints: list[str] | None = None,
    recurring_plan: str = "",
) -> str:
    """Build the per-request task prompt.

    ``system_prompt`` is accepted for backwards compatibility but is not
    duplicated into the task — brand voice lives only in the system message.
    """
    _ = system_prompt
    return render_task_prompt(
        platform=platform,
        char_limit=char_limit,
        hashtag_rule=hashtag_rule,
        user_request=user_request,
        source_material=source_material or [],
        recent_posts=recent_posts or [],
        campaign_hint=campaign_hint,
        output_format=output_format,
        mood_adjustments=mood_adjustments,
        technical_notes=parameter_notes or [],
        output_constraints=output_constraints or [],
        recurring_plan=recurring_plan,
    )


def render_few_shot_prompt(examples: list[str]) -> str:
    return f"""Few-shot examples:
{format_list(examples)}
""".strip()


def render_vibe_preview_prompt(*, team_name: str, profile_summary: str) -> str:
    return f"""You summarize a team's social media brand voice in one or two sentences, in German if the profile language is de, otherwise English.

Team: {team_name}
Profile:
{profile_summary}

Respond with ONLY valid JSON (no markdown):
{{"summary": "Ich klinge jetzt wie ...", "suggestion": "Optional one-line tweak suggestion or empty string"}}"""


def render_profile_assistant_prompt(*, brief: str, examples: list[str], language: str = "de") -> str:
    examples_block = ""
    if examples:
        examples_block = "\nExisting reference posts or quotes (mirror their voice):\n" + format_list(examples) + "\n"

    return f"""You design social-media brand profiles for the Goloom scheduler.
Your output is consumed directly by a prompt builder, so be specific and concrete.

A user described their account or project. Propose a complete profile that
sounds genuinely human — never like generic AI marketing copy.

Profile language preference: {language}
User brief:
\"\"\"
{brief.strip()}
\"\"\"
{examples_block}
Rules:
- Match the brief's domain precisely (a dentist sounds nothing like a tech podcast).
- Persona must read like a real person, not a corporate role.
- archetype is a 2-5 word label (e.g. "Tech Podcast", "Solo Indie Dev", "Zahnarztpraxis", "Boutique Werbeagentur").
- preferred_words and signature_phrases must be domain-specific, not generic.
- banned_words: at most 5 words this account should especially avoid.
- formatting_rules: 2-4 soft style notes, not rigid laws.
- main_value: one concrete sentence; no buzzwords.

Respond with ONLY valid JSON (no markdown, no code fences) matching this exact schema:
{{
  "identity": {{
    "archetype": "...",
    "persona": "...",
    "industry": "...",
    "main_value": "...",
    "target_audience": "..."
  }},
  "language_dna": {{
    "sentence_style": "...",
    "humor_style": "...",
    "preferred_words": ["..."],
    "signature_phrases": ["..."]
  }},
  "reach_strategy": {{
    "hook_style": "...",
    "cta_focus": "..."
  }},
  "banned_words": ["..."],
  "formatting_rules": ["..."],
  "preferred_language": "{language}",
  "max_hashtags": 3
}}"""


def render_analysis_prompt(
    *,
    team_name: str,
    recent_posts: list[str],
    existing_tonality: str = "",
    existing_rules: list[str] | None = None,
) -> str:
    rules_text = "\n".join(f"- {r}" for r in (existing_rules or []))
    posts_text = "\n\n".join(f"--- Post {i+1} ---\n{p}" for i, p in enumerate(recent_posts))

    return f"""Analyze the following recent social media posts from team "{team_name}" and extract their writing style.

Existing tonality: {existing_tonality or "Not set"}
Existing formatting rules:
{rules_text or "- None"}

Recent posts to analyze:
{posts_text}

Extract:
1. Tonality — what is the consistent voice?
2. Formatting rules — specific patterns (line breaks, emoji, caps, sentence length)
3. Banned words or topics — anything notably avoided
4. Preferred language
5. Typical hashtag count

Respond with ONLY valid JSON (no markdown):
{{"tonality": "...", "formatting_rules": [...], "banned_words": [...], "preferred_language": "...", "max_hashtags": N}}"""


def format_list(items: list[str]) -> str:
    if not items:
        return "- None provided."
    return "\n".join(f"- {item}" for item in items)


def format_inline_list(items: list[str]) -> str:
    if not items:
        return "- None."
    return "- " + ", ".join(items)


def format_style_examples(items: list[str]) -> str:
    if not items:
        return "- None provided — write in the brand voice described above."
    blocks = []
    for index, item in enumerate(items, start=1):
        blocks.append(f"Example {index}:\n---\n{item.strip()}\n---")
    return "\n\n".join(blocks)


def format_value(value: Any) -> str:
    if isinstance(value, (dict, list)):
        return json.dumps(value, ensure_ascii=False, sort_keys=True)
    return str(value)

