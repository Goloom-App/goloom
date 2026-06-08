from datetime import UTC, datetime, timedelta

from app.workers.proactive.cron_schedule import is_team_due


def test_hourly_cron_due_when_never_run() -> None:
    now = datetime(2026, 6, 8, 10, 30, tzinfo=UTC)
    assert is_team_due("0 * * * *", now, None) is True


def test_hourly_cron_not_due_within_interval() -> None:
    now = datetime(2026, 6, 8, 10, 30, tzinfo=UTC)
    last_run = now - timedelta(minutes=30)
    assert is_team_due("0 * * * *", now, last_run) is False


def test_hourly_cron_due_after_interval() -> None:
    now = datetime(2026, 6, 8, 11, 5, tzinfo=UTC)
    last_run = now - timedelta(hours=1, minutes=5)
    assert is_team_due("0 * * * *", now, last_run) is True


def test_daily_cron_due_once_per_day() -> None:
    now = datetime(2026, 6, 8, 9, 15, tzinfo=UTC)
    assert is_team_due("0 9 * * *", now, None) is True

    later_same_day = datetime(2026, 6, 8, 12, 0, tzinfo=UTC)
    assert is_team_due("0 9 * * *", later_same_day, now) is False

    next_day = datetime(2026, 6, 9, 9, 5, tzinfo=UTC)
    assert is_team_due("0 9 * * *", next_day, now) is True
