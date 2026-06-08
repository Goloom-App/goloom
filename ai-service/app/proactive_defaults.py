def default_proactive_settings() -> dict:
    return {
        "auto_fill_enabled": False,
        "content_gap_threshold_days": 3,
        "max_triggers_per_day": 5,
        "cron_schedule": "0 * * * *",
    }
