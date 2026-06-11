"""Lightweight defaults that steer voice quality without negative-programming overload.

Teams can opt out via ``language_dna.anti_ai_override`` — then only their own
banned words (if any) are used.
"""
from __future__ import annotations

# Positive principles replace long lists of micro-rules.
QUALITY_VOICE_PRINCIPLES: tuple[str, ...] = (
    "Write like someone who actually lives this topic — conversational, sometimes blunt, never salesy.",
    "Prefer concrete facts and observations over adjectives. When unsure, say less instead of padding.",
    "Vary rhythm naturally: short punches, longer asides, fragments are fine. Do not sound polished or 'optimized'.",
)

# Only the worst universal tells — merged only when the team has fewer than ``limit`` custom words.
CORE_AVOID_WORDS: tuple[str, ...] = (
    "tauche ein",
    "game-changer",
    "revolutionär",
)

DEFAULT_BANNED_WORD_LIMIT = 5


def capped_banned_words(
    profile_banned: list[str],
    *,
    override: bool = False,
    limit: int = DEFAULT_BANNED_WORD_LIMIT,
) -> list[str]:
    """Return at most ``limit`` banned words, prioritising team-specific terms."""
    team = []
    seen: set[str] = set()
    for raw in profile_banned:
        word = raw.strip()
        if not word:
            continue
        key = word.lower()
        if key in seen:
            continue
        seen.add(key)
        team.append(word)
        if len(team) >= limit:
            break

    if override:
        return team

    for phrase in CORE_AVOID_WORDS:
        if len(team) >= limit:
            break
        key = phrase.strip().lower()
        if key in seen:
            continue
        seen.add(key)
        team.append(phrase)

    return team
