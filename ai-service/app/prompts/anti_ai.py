"""Default anti-AI-speak rules and banned phrases applied to every prompt.

These defaults keep generated posts from sounding like generic LLM output.
They are merged into the system prompt unless a team explicitly opts out
via ``language_dna.anti_ai_override``.
"""
from __future__ import annotations

# Phrases that strongly signal "this was written by an LLM" and almost never
# appear in genuine human social media writing. Merged into banned_words.
ANTI_AI_BANNED_PHRASES: tuple[str, ...] = (
    # German marketing-LLM tells
    "tauche ein",
    "tauchen sie ein",
    "spannend",
    "spannende reise",
    "revolutionär",
    "bahnbrechend",
    "in der heutigen schnelllebigen welt",
    "in einer welt, in der",
    "im wandel der zeit",
    "es ist wichtig zu beachten",
    "zusammenfassend lässt sich sagen",
    "lass uns gemeinsam",
    "lasst uns gemeinsam",
    "auf eine reise",
    "ein game-changer",
    # English LLM tells
    "in today's fast-paced world",
    "in a world where",
    "let's dive in",
    "dive into",
    "delve into",
    "game-changer",
    "game changer",
    "in conclusion",
    "moreover",
    "furthermore",
    "elevate your",
    "unleash",
    "unlock the power",
    "harness the power",
    "leverage",
    "seamless",
    "robust solution",
    "comprehensive solution",
    "cutting-edge",
    "best-in-class",
    "next-level",
    "it's not just",
)

# Structural style rules that fight typical LLM output patterns.
# Rendered as bullet points in the system prompt.
ANTI_AI_STYLE_RULES: tuple[str, ...] = (
    "Write like a human writes social posts. Imperfect, conversational, sometimes blunt.",
    "Never open with 'In a world where…', 'In today's…', 'Stell dir vor…', or any other rhetorical scene-setter.",
    "Avoid the 'It's not just X, it's Y' pattern.",
    "Avoid three-part rhetorical lists ('faster, smarter, better'). Pick one thing and say it.",
    "Em-dashes (—) only when a comma genuinely will not do. Never two em-dashes in one post.",
    "No empty hype adjectives (amazing, incredible, revolutionary). Use concrete facts or numbers instead.",
    "Sentence fragments are fine. Starting a sentence with 'Und', 'Aber', 'And', 'But' is fine.",
    "No closing summary like 'Zusammengefasst…' or 'In short…'. Stop when the point is made.",
    "Do not announce what the post is about ('Heute geht es um…'). Just say it.",
)


def merged_banned_words(profile_banned: list[str], override: bool = False) -> list[str]:
    """Combine team banned words with the anti-AI defaults.

    If ``override`` is True the team has explicitly opted out and only their
    own banned words are returned.
    """
    if override:
        return sorted({w.strip() for w in profile_banned if w and w.strip()})
    combined = {w.strip().lower() for w in profile_banned if w and w.strip()}
    for phrase in ANTI_AI_BANNED_PHRASES:
        combined.add(phrase.strip().lower())
    return sorted(combined)
