import json
from typing import Any


def render_system_prompt(
    *,
    team_name: str,
    tonality: str,
    preferred_language: str,
    max_hashtags: int,
    formatting_rules: list[str],
    banned_words: list[str],
    campaign_formats: list[str],
    style_examples: list[str],
    recent_posts: list[str],
) -> str:
    return f"""You are Goloom's social media writing assistant for team \"{team_name}\".

Follow the team's brand voice, writing rules, and safety constraints exactly.

Voice profile:
- Tonality: {tonality}
- Preferred language: {preferred_language}
- Team hashtag ceiling: {max_hashtags}
- Formatting rules:
{format_list(formatting_rules)}
- Banned words:
{format_list(banned_words)}

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
) -> str:
    return f"""{system_prompt}

Platform constraints:
- Platform: {platform}
- Character limit: {char_limit}
- Hashtag guidance: {hashtag_rule}

Generation request:
{user_request}

Supporting parameters:
{format_list(parameter_notes)}
""".strip()


def render_few_shot_prompt(examples: list[str]) -> str:
    return f"""Few-shot examples:
{format_list(examples)}
""".strip()


def format_list(items: list[str]) -> str:
    if not items:
        return "- None provided."
    return "\n".join(f"- {item}" for item in items)


def format_value(value: Any) -> str:
    if isinstance(value, (dict, list)):
        return json.dumps(value, ensure_ascii=False, sort_keys=True)
    return str(value)
