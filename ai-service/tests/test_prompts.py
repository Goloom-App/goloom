from app.prompts import PromptBuilder
from app.prompts.anti_ai import CORE_AVOID_WORDS, DEFAULT_BANNED_WORD_LIMIT


def sample_context() -> dict:
    return {
        "team": {"id": "team-1", "name": "Launch Crew"},
        "profile": {
            "team_id": "team-1",
            "style_metadata": {
                "tonality": "playful",
                "formatting_rules": ["Keep sentences short", "Lead with the key update"],
                "banned_words": ["synergy", "crypto"],
                "max_hashtags": 2,
                "preferred_language": "en",
                "identity": {
                    "persona": "A small team that ships in public.",
                    "main_value": "Practical release notes without hype.",
                },
                "language_dna": {
                    "sentence_style": "Short and direct.",
                    "humor_style": "Dry.",
                },
                "reach_strategy": {
                    "hook_style": "Lead with the change users care about.",
                },
            },
        },
        "campaign_formats": [
            {
                "id": "fmt-1",
                "name": "Weekly roundup",
                "required_hashtags": ["#launch"],
                "is_active": True,
            }
        ],
        "style_examples": [
            {"platform": "mastodon", "content": "Big progress, shared simply.", "notes": "warm"},
            {"platform": "mastodon", "content": "Second example.", "notes": ""},
            {"platform": "mastodon", "content": "Third example.", "notes": ""},
            {"platform": "mastodon", "content": "Fourth example.", "notes": ""},
        ],
        "recent_posts": [
            {"content": "Previous shipping update."},
            {"content": "Another older post."},
            {"content": "Third older post."},
            {"content": "Fourth older post should not appear."},
        ],
    }


def test_build_brand_voice_prompt_is_positive_and_compact():
    builder = PromptBuilder()

    prompt = builder.build_brand_voice_prompt(sample_context())

    assert 'team "Launch Crew"' not in prompt  # new format uses quotes differently
    assert 'You write social media posts for "Launch Crew".' in prompt
    assert "Brand voice:" in prompt
    assert "Quality bar:" in prompt
    assert "A small team that ships in public." in prompt
    assert "Keep sentences short" in prompt
    assert "synergy" in prompt
    assert "crypto" in prompt
    assert "Posts that sound like us" in prompt
    assert "Fourth example." not in prompt
    assert "Recent posts to avoid duplicating" not in prompt
    assert "Available campaign formats" not in prompt
    assert "Sound human, not AI:" not in prompt


def test_build_brand_voice_caps_banned_words_and_style_examples():
    builder = PromptBuilder()
    ctx = sample_context()
    ctx["profile"]["style_metadata"]["banned_words"] = [
        "one",
        "two",
        "three",
        "four",
        "five",
        "six",
        "seven",
    ]

    prompt = builder.build_brand_voice_prompt(ctx)

    assert prompt.count("Example ") == 3
    for word in ("six", "seven"):
        assert word not in prompt


def test_anti_ai_override_drops_quality_defaults():
    builder = PromptBuilder()
    ctx = sample_context()
    ctx["profile"]["style_metadata"]["language_dna"] = {"anti_ai_override": True}

    prompt = builder.build_brand_voice_prompt(ctx)

    assert "tauche ein" not in prompt
    assert "Write like someone who actually lives this topic" not in prompt
    assert "synergy" in prompt


def test_core_avoid_words_fill_remaining_slots():
    from app.prompts.anti_ai import capped_banned_words

    words = capped_banned_words(["alpha", "beta"], override=False, limit=DEFAULT_BANNED_WORD_LIMIT)
    assert words[:2] == ["alpha", "beta"]
    assert any(word in words for word in CORE_AVOID_WORDS)


def test_profile_assistant_prompt_includes_brief_and_schema():
    builder = PromptBuilder()

    prompt = builder.build_profile_assistant_prompt(
        {"brief": "Wir sind ein Selfhosting-Podcast für Anfänger.", "language": "de"}
    )

    assert "Selfhosting-Podcast" in prompt
    assert '"archetype"' in prompt
    assert '"signature_phrases"' in prompt


def test_apply_platform_constraints_returns_expected_limits():
    builder = PromptBuilder()

    assert builder.apply_platform_constraints("mastodon")["char_limit"] == 500
    assert builder.apply_platform_constraints("bluesky")["char_limit"] == 300
    assert builder.apply_platform_constraints("friendica")["char_limit"] == 5000
    assert builder.apply_platform_constraints("unknown")["char_limit"] == 500


def test_inject_few_shot_appends_examples():
    builder = PromptBuilder()

    prompt = builder.inject_few_shot(
        "Base prompt",
        [
            {"input": "Announce release", "output": "We shipped today.", "notes": "brief"},
            "Keep the CTA subtle.",
        ],
    )

    assert prompt.startswith("Base prompt")
    assert "Few-shot examples:" in prompt
    assert "Input: Announce release" in prompt
    assert "Output: We shipped today." in prompt
    assert "Keep the CTA subtle." in prompt


def test_build_generation_prompt_is_task_only_without_brand_voice():
    builder = PromptBuilder()

    prompt = builder.build_generation_prompt(
        sample_context(),
        {
            "occasion": "Announce the new feature rollout.",
            "target_account_ids": ["acct-1", "acct-2"],
            "campaign_format_id": "fmt-1",
        },
        "bluesky",
    )

    assert "Brand voice:" not in prompt
    assert "## Task" in prompt
    assert "Announce the new feature rollout." in prompt
    assert "Platform: bluesky" in prompt
    assert "Character limit: 300" in prompt
    assert 'target_account_ids: ["acct-1", "acct-2"]' in prompt
    assert "Campaign: Weekly roundup" in prompt
    assert "Previous shipping update." in prompt
    assert "Fourth older post should not appear." not in prompt
