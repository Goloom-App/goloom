from __future__ import annotations

from collections.abc import Mapping, Sequence
from datetime import UTC, date, datetime, time, timedelta
from typing import Any


WEEKDAY_NAMES = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]


def resolve_scheduled_at(
    *,
    params: dict[str, Any],
    campaign_format: dict[str, Any] | None,
    context: dict[str, Any],
) -> datetime | None:
    preferred_time = preferred_posting_time(context, campaign_format)
    target_date = params.get("target_date")
    if isinstance(target_date, str) and target_date.strip():
        selected_date = date.fromisoformat(target_date.strip())
        return datetime.combine(selected_date, preferred_time, tzinfo=UTC)

    if campaign_format is None:
        return datetime.combine(_utcnow().date() + timedelta(days=1), preferred_time, tzinfo=UTC)

    weekday = campaign_format.get("weekday")
    if weekday is None:
        return datetime.combine(_utcnow().date() + timedelta(days=1), preferred_time, tzinfo=UTC)

    target_go_weekday = _coerce_int(weekday)
    if target_go_weekday is None:
        return None

    occupied_dates = occupied_campaign_dates(context, target_go_weekday)
    now = _utcnow().astimezone(UTC)
    for offset in range(0, 366):
        candidate_date = now.date() + timedelta(days=offset)
        if _go_weekday(datetime.combine(candidate_date, time.min, tzinfo=UTC)) != target_go_weekday:
            continue
        slot = datetime.combine(candidate_date, preferred_time, tzinfo=UTC)
        if slot <= now:
            continue
        if candidate_date.isoformat() in occupied_dates:
            continue
        return slot
    return None


def preferred_posting_time(context: dict[str, Any], campaign_format: dict[str, Any] | None) -> time:
    engagement_hours = context.get("engagement_hours") or context.get("engagementHours") or []
    peak_hour = _best_engagement_hour(engagement_hours)
    if peak_hour is not None:
        return time(hour=peak_hour, minute=0)

    scheduling = _team_scheduling_preferences(context)
    weekday = _coerce_int((campaign_format or {}).get("weekday"))
    posting_windows = scheduling.get("posting_windows") or []
    if weekday is not None:
        for window in posting_windows:
            if not isinstance(window, Mapping):
                continue
            if _coerce_int(window.get("weekday")) == weekday:
                parsed = _parse_clock(window.get("start"))
                if parsed is not None:
                    return parsed
    default_timeslots = scheduling.get("default_timeslots") or []
    for item in default_timeslots:
        parsed = _parse_clock(item)
        if parsed is not None:
            return parsed
    return time(hour=9, minute=0)


def occupied_campaign_dates(context: dict[str, Any], weekday: int) -> set[str]:
    occupied: set[str] = set()
    upcoming = context.get("upcoming_posts") or context.get("upcomingPosts") or []
    for item in upcoming:
        if not isinstance(item, Mapping):
            continue
        scheduled_at = item.get("scheduled_at") or item.get("scheduledAt")
        if not isinstance(scheduled_at, str) or not scheduled_at.strip():
            continue
        try:
            parsed = datetime.fromisoformat(scheduled_at.replace("Z", "+00:00")).astimezone(UTC)
        except ValueError:
            continue
        if _go_weekday(parsed) == weekday:
            occupied.add(parsed.date().isoformat())
    return occupied


def _best_engagement_hour(buckets: Sequence[Any]) -> int | None:
    best_hour: int | None = None
    best_score = -1.0
    for item in buckets:
        if not isinstance(item, Mapping):
            continue
        hour = _coerce_int(item.get("hour") or item.get("hour_utc") or item.get("hourUTC"))
        score_raw = item.get("score")
        try:
            score = float(score_raw)
        except (TypeError, ValueError):
            score = 0.0
        if hour is None or hour < 0 or hour > 23:
            continue
        if score > best_score:
            best_score = score
            best_hour = hour
    return best_hour


def _team_scheduling_preferences(context: dict[str, Any]) -> dict[str, Any]:
    team = context.get("team") or {}
    if isinstance(team, Mapping):
        prefs = team.get("scheduling_preferences") or team.get("schedulingPreferences") or {}
        if isinstance(prefs, Mapping):
            return dict(prefs)
    return {}


def _parse_clock(raw_value: Any) -> time | None:
    if not isinstance(raw_value, str) or not raw_value.strip():
        return None
    parts = raw_value.strip().split(":")
    if len(parts) < 2:
        return None
    try:
        hour = int(parts[0])
        minute = int(parts[1])
    except ValueError:
        return None
    if hour < 0 or hour > 23 or minute < 0 or minute > 59:
        return None
    return time(hour=hour, minute=minute)


def _coerce_int(value: Any) -> int | None:
    if value is None:
        return None
    try:
        return int(value)
    except (TypeError, ValueError):
        return None


def _go_weekday(value: datetime) -> int:
    return (value.weekday() + 1) % 7


def _utcnow() -> datetime:
    return datetime.now(UTC)


def format_datetime(value: datetime | None) -> str | None:
    if value is None:
        return None
    return value.astimezone(UTC).isoformat().replace("+00:00", "Z")
