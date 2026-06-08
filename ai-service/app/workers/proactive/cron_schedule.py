from __future__ import annotations

from datetime import UTC, datetime, timedelta


def is_team_due(cron_schedule: str, now: datetime, last_run: datetime | None) -> bool:
    """Return True when a team should run proactive hooks for the given cron expression."""
    if now.tzinfo is None:
        now = now.replace(tzinfo=UTC)
    if last_run is not None and last_run.tzinfo is None:
        last_run = last_run.replace(tzinfo=UTC)

    cron = (cron_schedule or "0 * * * *").strip()
    parts = cron.split()
    if len(parts) != 5:
        return _interval_due(last_run, now, 3600)

    minute, hour, day, month, weekday = parts

    if minute == "0" and hour in {"*", "*/1"} and day == "*" and month == "*" and weekday == "*":
        return _interval_due(last_run, now, 3600)

    if minute == "0" and hour.isdigit() and day == "*" and month == "*" and weekday == "*":
        target_hour = int(hour)
        if now.hour < target_hour:
            return False
        if last_run is None:
            return True
        return last_run.date() < now.date()

    return _interval_due(last_run, now, 3600)


def _interval_due(last_run: datetime | None, now: datetime, interval_seconds: int) -> bool:
    if last_run is None:
        return True
    return (now - last_run) >= timedelta(seconds=interval_seconds)
