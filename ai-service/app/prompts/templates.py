import json
from typing import Any


def render_system_prompt(
    *,
    team_name: str,
    preferred_language: str,
    max_hashtags: int,
    formatting_rules: list[str],
    banned_words: list[str],
    preferred_words: list[str],
    identity_lines: list[str],
    language_dna_lines: list[str],
    reach_strategy_lines: list[str],
    knowledge_sources: list[str],
    campaign_formats: list[str],
    style_examples: list[str],
    recent_posts: list[str],
) -> str:
    return f"""You are Goloom's social media writing assistant for team \"{team_name}\".

Follow the team's brand voice, writing rules, and safety constraints exactly.

Brand identity:
{format_list(identity_lines)}

Language DNA:
{format_list(language_dna_lines)}

Reach strategy:
{format_list(reach_strategy_lines)}

Writing rules:
- Preferred language: {preferred_language}
- Team hashtag ceiling: {max_hashtags}
- Formatting rules:
{format_list(formatting_rules)}
- Banned words (never use):
{format_list(banned_words)}
- Preferred words (use when natural):
{format_list(preferred_words)}

Knowledge base (exclusive factual source — CRITICAL):
{format_list(knowledge_sources)}
If a fact is not present in the knowledge base above, do NOT invent it. Say less rather than hallucinate.

Available campaign formats:
{format_list(campaign_formats)}

Reference style examples:
{format_list(style_examples)}

Recent posts to avoid duplicating:
{format_list(recent_posts)}
""".strip()


def render_generation_prompt(
    *,
    system_prompt: str,
    platform: str,
    char_limit: int,
    hashtag_rule: str,
    user_request: str,
    parameter_notes: list[str],
    output_format: str = "",
    mood_adjustments: list[str] | None = None,
) -> str:
    format_hint = f"\nOutput format: {output_format}" if output_format else ""
    mood_hint = ""
    if mood_adjustments:
        mood_hint = "\nMood adjustments:\n" + format_list(mood_adjustments)

    return f"""{system_prompt}

Platform constraints:
- Platform: {platform}
- Character limit: {char_limit}
- Hashtag guidance: {hashtag_rule}{format_hint}{mood_hint}

Generation request:
{user_request}

Supporting parameters:
{format_list(parameter_notes)}

Respond with a JSON object using this exact structure (no markdown, no code fences):
{{"content": "the post text", "hashtags": ["hashtag1", "hashtag2"], "platform_metadata": {{"key": "value"}}}}
""".strip()


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


def format_value(value: Any) -> str:
    if isinstance(value, (dict, list)):
        return json.dumps(value, ensure_ascii=False, sort_keys=True)
    return str(value)
