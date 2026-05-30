from __future__ import annotations

import json
import re
from collections.abc import Mapping, Sequence
from datetime import UTC, date, datetime, time, timedelta
from typing import Any

from app.adapters import LLMAdapter
from app.prompts import PromptBuilder
from app.services import GoloomClient


DAY_OFFSET_RE = re.compile(r"\{day([+-]\d+)\}")
MONTH_OFFSET_RE = re.compile(r"\{month([+-]\d+)\}")
WEEKDAY_NAMES = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]


class CampaignWorker:
    def __init__(self, adapter: LLMAdapter, goloom_client: GoloomClient, prompt_builder: PromptBuilder):
        self.adapter = adapter
        self.goloom_client = goloom_client
        self.prompt_builder = prompt_builder

    async def process(self, job: dict) -> dict:
        job_id = str(job.get("id") or "")
        callback_sent = False

        try:
            team_id = str(job["team_id"])
            author_user_id = str(job["author_user_id"])
            params = self._params(job)
            campaign_format_id = str(params.get("campaign_format_id") or "").strip()
            if not campaign_format_id:
                raise ValueError("campaign_format_id is required")

            context = job.get("context") or {}
            campaign_format = self._find_campaign_format(context, campaign_format_id)
            platform = str(params.get("platform") or "mastodon")
            constraints = self.prompt_builder.apply_platform_constraints(platform)
            suggested_scheduled_at = self._resolve_suggested_scheduled_at(params, campaign_format, context)
            rendered_template = self._render_structure_template(
                campaign_format.get("structure") or {},
                params,
                suggested_scheduled_at,
            )
            required_hashtags = self._string_list(campaign_format.get("required_hashtags"))
            system_prompt = self.prompt_builder.build_system_prompt(context)
            base_prompt = self.prompt_builder.build_generation_prompt(
                context,
                self._prompt_params(
                    params=params,
                    campaign_format=campaign_format,
                    rendered_template=rendered_template,
                    required_hashtags=required_hashtags,
                    suggested_scheduled_at=suggested_scheduled_at,
                ),
                platform,
            )
            prompt = self.prompt_builder.inject_few_shot(base_prompt, context.get("style_examples", []))

            response = await self.adapter.generate(
                prompt,
                system_prompt,
                model=self.adapter.config.model,
                temperature=0.7,
                max_tokens=1000,
                response_format="json",
                author_user_id=author_user_id,
            )

            result = self._validate_result(
                self._parse_result(response.content),
                required_hashtags=required_hashtags,
                char_limit=int(constraints["char_limit"]),
                suggested_scheduled_at=suggested_scheduled_at,
            )
            await self._try_callback(job_id, "completed", result)
            callback_sent = True
            return result
        except Exception as exc:
            if not callback_sent:
                await self._try_callback(job_id, "failed", {}, error_message=str(exc))
            raise

    async def _try_callback(self, job_id: str, status: str, result: dict, error_message: str = "") -> None:
        try:
            await self.goloom_client.send_callback(job_id, status, result, error_message)
        except Exception:
            pass

    @staticmethod
    def _params(job: dict) -> dict[str, Any]:
        params = job.get("params") or {}
        if not isinstance(params, dict):
            raise ValueError("job params must be an object")
        return params

    @staticmethod
    def _find_campaign_format(context: dict, campaign_format_id: str) -> dict[str, Any]:
        campaign_formats = context.get("campaign_formats") or context.get("campaignFormats") or []
        for item in campaign_formats:
            if isinstance(item, Mapping) and str(item.get("id") or "") == campaign_format_id:
                if item.get("is_active") is False:
                    raise ValueError(f"Campaign format {campaign_format_id} is inactive")
                return dict(item)
        raise ValueError(f"Campaign format {campaign_format_id} not found")

    def _prompt_params(
        self,
        *,
        params: dict[str, Any],
        campaign_format: dict[str, Any],
        rendered_template: Any,
        required_hashtags: list[str],
        suggested_scheduled_at: datetime | None,
    ) -> dict[str, Any]:
        prompt_hint = self._build_prompt_hint(
            campaign_format=campaign_format,
            rendered_template=rendered_template,
            required_hashtags=required_hashtags,
            suggested_scheduled_at=suggested_scheduled_at,
        )
        extra_params = {key: value for key, value in params.items() if key != "prompt_hint"}
        return {
            "prompt_hint": prompt_hint,
            "campaign_format_name": campaign_format.get("name") or "",
            "campaign_format_template": rendered_template,
            "required_hashtags": required_hashtags,
            "suggested_scheduled_at": self._format_datetime(suggested_scheduled_at),
            **extra_params,
        }

    def _build_prompt_hint(
        self,
        *,
        campaign_format: dict[str, Any],
        rendered_template: Any,
        required_hashtags: list[str],
        suggested_scheduled_at: datetime | None,
    ) -> str:
        schedule_text = self._format_datetime(suggested_scheduled_at) or "unscheduled"
        hashtags_text = ", ".join(required_hashtags) if required_hashtags else "none"
        return (
            f"Generate a campaign auto-pilot post for the format '{campaign_format.get('name') or 'unnamed format'}'.\n"
            f"Use this rendered structure template as the blueprint: {json.dumps(rendered_template, ensure_ascii=False, sort_keys=True)}\n"
            f"Required hashtags: {hashtags_text}. Every required hashtag must appear in the content.\n"
            f"Suggested schedule: {schedule_text}.\n"
            "Return JSON only with keys content and hashtags."
        )

    def _validate_result(
        self,
        result: dict[str, Any],
        *,
        required_hashtags: list[str],
        char_limit: int,
        suggested_scheduled_at: datetime | None,
    ) -> dict:
        content = str(result["content"]).strip()
        hashtags = self._string_list(result.get("hashtags"))
        missing_from_content = [tag for tag in required_hashtags if tag.casefold() not in content.casefold()]
        if missing_from_content:
            content = f"{content} {' '.join(missing_from_content)}".strip()
        for tag in required_hashtags:
            if tag not in hashtags:
                hashtags.append(tag)

        if len(content) > char_limit:
            raise ValueError(f"Generated content exceeds platform limit of {char_limit} characters")
        missing_after_fix = [tag for tag in required_hashtags if tag.casefold() not in content.casefold()]
        if missing_after_fix:
            raise ValueError(f"Generated content is missing required hashtags: {', '.join(missing_after_fix)}")

        return {
            "content": content,
            "hashtags": hashtags,
            "suggested_scheduled_at": self._format_datetime(suggested_scheduled_at),
        }

    @staticmethod
    def _parse_result(raw_content: str) -> dict[str, Any]:
        try:
            payload = json.loads(raw_content)
        except json.JSONDecodeError as exc:
            raise ValueError("LLM response was not valid JSON") from exc

        if not isinstance(payload, dict):
            raise ValueError("LLM response must be a JSON object")

        content = payload.get("content")
        hashtags = payload.get("hashtags") or []
        if not isinstance(content, str) or not content.strip():
            raise ValueError("LLM response missing content")
        if not isinstance(hashtags, list):
            raise ValueError("LLM response hashtags must be a list")

        return {
            "content": content.strip(),
            "hashtags": [str(hashtag) for hashtag in hashtags if str(hashtag).strip()],
        }

    def _resolve_suggested_scheduled_at(
        self,
        params: dict[str, Any],
        campaign_format: dict[str, Any],
        context: dict[str, Any],
    ) -> datetime | None:
        preferred_time = self._preferred_time(context, campaign_format)
        target_date = params.get("target_date")
        if isinstance(target_date, str) and target_date.strip():
            selected_date = self._parse_target_date(target_date)
            return datetime.combine(selected_date, preferred_time, tzinfo=UTC)

        weekday = campaign_format.get("weekday")
        if weekday is None:
            return None

        now = self._utcnow().astimezone(UTC)
        slot = datetime.combine(now.date(), preferred_time, tzinfo=UTC)
        target_go_weekday = self._coerce_int(weekday)
        if target_go_weekday is None:
            return None

        days_ahead = (target_go_weekday - self._go_weekday(now) + 7) % 7
        if days_ahead == 0 and slot <= now:
            days_ahead = 7
        selected_date = now.date() + timedelta(days=days_ahead)
        return datetime.combine(selected_date, preferred_time, tzinfo=UTC)

    def _preferred_time(self, context: dict[str, Any], campaign_format: dict[str, Any]) -> time:
        scheduling = self._team_scheduling_preferences(context)
        weekday = self._coerce_int(campaign_format.get("weekday"))
        posting_windows = scheduling.get("posting_windows") or []
        if weekday is not None:
            for window in posting_windows:
                if not isinstance(window, Mapping):
                    continue
                if self._coerce_int(window.get("weekday")) == weekday:
                    parsed = self._parse_clock(window.get("start"))
                    if parsed is not None:
                        return parsed
        default_timeslots = scheduling.get("default_timeslots") or []
        for item in default_timeslots:
            parsed = self._parse_clock(item)
            if parsed is not None:
                return parsed
        return time(hour=9, minute=0)

    @staticmethod
    def _team_scheduling_preferences(context: dict[str, Any]) -> dict[str, Any]:
        team = context.get("team") or {}
        if isinstance(team, Mapping):
            prefs = team.get("scheduling_preferences") or team.get("schedulingPreferences") or {}
            if isinstance(prefs, Mapping):
                return dict(prefs)
        return {}

    def _render_structure_template(self, template: Any, params: dict[str, Any], scheduled_at: datetime | None) -> Any:
        if isinstance(template, Mapping):
            return {str(key): self._render_structure_template(value, params, scheduled_at) for key, value in template.items()}
        if isinstance(template, Sequence) and not isinstance(template, (str, bytes, bytearray)):
            return [self._render_structure_template(value, params, scheduled_at) for value in template]
        if isinstance(template, str):
            return self._render_text_template(template, params, scheduled_at)
        return template

    def _render_text_template(self, text: str, params: dict[str, Any], scheduled_at: datetime | None) -> str:
        if not text:
            return ""

        basis = (scheduled_at or self._utcnow().astimezone(UTC)).astimezone(UTC)
        rendered = DAY_OFFSET_RE.sub(lambda match: self._zero_pad(self._clamp_day(basis.day + int(match.group(1)))), text)
        rendered = MONTH_OFFSET_RE.sub(
            lambda match: self._zero_pad(self._clamp_month(basis.month + int(match.group(1)))),
            rendered,
        )

        replacements = {
            "{year}": str(basis.year),
            "{month}": self._zero_pad(basis.month),
            "{day}": self._zero_pad(basis.day),
            "{weekday}": str(self._go_weekday(basis)),
            "{weekday_name}": WEEKDAY_NAMES[self._go_weekday(basis)],
            "{main_day}": "",
            "{main_month}": "",
            "{main_weekday_name}": "",
            "{campaign_name}": str(params.get("campaign_name") or params.get("name") or ""),
            "{format_name}": str(params.get("format_name") or ""),
        }
        for key, value in replacements.items():
            rendered = rendered.replace(key, value)

        for key, value in params.items():
            rendered = rendered.replace(f"{{{key}}}", self._format_placeholder_value(value))
        return rendered

    @staticmethod
    def _format_placeholder_value(value: Any) -> str:
        if value is None:
            return ""
        if isinstance(value, (dict, list)):
            return json.dumps(value, ensure_ascii=False, sort_keys=True)
        return str(value)

    @staticmethod
    def _parse_target_date(raw_value: str) -> date:
        try:
            return date.fromisoformat(raw_value.strip())
        except ValueError as exc:
            raise ValueError("target_date must be an ISO date (YYYY-MM-DD)") from exc

    @staticmethod
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

    @staticmethod
    def _coerce_int(value: Any) -> int | None:
        if value is None:
            return None
        try:
            return int(value)
        except (TypeError, ValueError):
            return None

    @staticmethod
    def _go_weekday(value: datetime) -> int:
        return (value.weekday() + 1) % 7

    @staticmethod
    def _string_list(value: Any) -> list[str]:
        if not value:
            return []
        if isinstance(value, list):
            return [str(item) for item in value if str(item).strip()]
        return [str(value)]

    @staticmethod
    def _zero_pad(value: int) -> str:
        return f"{value:02d}"

    @staticmethod
    def _clamp_day(value: int) -> int:
        return min(max(value, 1), 31)

    @staticmethod
    def _clamp_month(value: int) -> int:
        return min(max(value, 1), 12)

    @staticmethod
    def _format_datetime(value: datetime | None) -> str | None:
        if value is None:
            return None
        return value.astimezone(UTC).isoformat().replace("+00:00", "Z")

    @staticmethod
    def _utcnow() -> datetime:
        return datetime.now(UTC)


