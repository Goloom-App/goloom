from app.prompts import PromptBuilder


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
            },
        },
        "campaign_formats": [
            {
                "name": "Weekly roundup",
                "required_hashtags": ["#launch"],
                "is_active": True,
            }
        ],
        "style_examples": [
            {"platform": "mastodon", "content": "Big progress, shared simply.", "notes": "warm"}
        ],
        "recent_posts": [{"content": "Previous shipping update."}],
    }


def test_build_system_prompt_injects_writing_rules_and_banned_words():
    builder = PromptBuilder()

    prompt = builder.build_system_prompt(sample_context())

    assert 'team "Launch Crew"' in prompt
    assert "Writing rules:" in prompt
    assert "Keep sentences short" in prompt
    assert "synergy" in prompt
    assert "crypto" in prompt


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


def test_build_generation_prompt_combines_system_and_platform_constraints():
    builder = PromptBuilder()

    prompt = builder.build_generation_prompt(
        sample_context(),
        {
            "prompt_hint": "Announce the new feature rollout.",
            "target_account_ids": ["acct-1", "acct-2"],
        },
        "bluesky",
    )

    assert "Writing rules:" in prompt
    assert "Platform: bluesky" in prompt
    assert "Character limit: 300" in prompt
    assert "Announce the new feature rollout." in prompt
    assert 'target_account_ids: ["acct-1", "acct-2"]' in prompt
