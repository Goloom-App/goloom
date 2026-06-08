from __future__ import annotations

import json
import logging
from typing import Any

from app.adapters import LLMAdapter
from app.prompts import PromptBuilder
from app.scheduling.slots import format_datetime, resolve_scheduled_at
from app.services import GoloomClient

logger = logging.getLogger(__name__)


class VoiceEngineWorker:
    def __init__(self, adapter: LLMAdapter, goloom_client: GoloomClient, prompt_builder: PromptBuilder):
        self.adapter = adapter
        self.goloom_client = goloom_client
        self.prompt_builder = prompt_builder
        self.max_retries = 3

    async def process(self, job: dict) -> dict:
        job_id = str(job.get("job_id") or "")
        callback_sent = False

        try:
            author_user_id = str(job["author_user_id"])
            params = self._params(job)
            context = job.get("context") or {}
            selected_accounts = self._selected_accounts(context, params)
            if not selected_accounts:
                raise ValueError("target_account_ids must include at least one account")

            campaign_format = self._optional_campaign_format(context, params)
            scheduled_at = resolve_scheduled_at(params=params, campaign_format=campaign_format, context=context)

            primary = max(selected_accounts, key=lambda item: int(item["max_chars"]))
            primary_platform = str(primary.get("provider") or "general")
            primary_limit = int(primary["max_chars"])

            system_prompt = self.prompt_builder.build_system_prompt(context)
            constraints = self.prompt_builder.apply_platform_constraints(primary_platform)
            base_prompt = self._build_multi_account_prompt(
                context=context,
                params=params,
                selected_accounts=selected_accounts,
                primary_limit=primary_limit,
                campaign_format=campaign_format,
                scheduled_at=scheduled_at,
            )
            prompt = self.prompt_builder.inject_few_shot(base_prompt, context.get("style_examples", []))

            parsed = await self._generate_with_retries(
                prompt=prompt,
                system_prompt=system_prompt,
                primary_limit=primary_limit,
                selected_accounts=selected_accounts,
                author_user_id=author_user_id,
            )

            result = {
                "content": parsed["content"],
                "hashtags": parsed.get("hashtags") or [],
                "platform_metadata": parsed.get("platform_metadata") or {},
                "account_content_override": parsed.get("account_content_override") or {},
                "scheduled_at": format_datetime(scheduled_at),
                "primary_account_id": str(primary.get("id") or ""),
            }
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

    def _build_multi_account_prompt(
        self,
        *,
        context: dict,
        params: dict[str, Any],
        selected_accounts: list[dict[str, Any]],
        primary_limit: int,
        campaign_format: dict[str, Any] | None,
        scheduled_at,
    ) -> str:
        platform = str(selected_accounts[0].get("provider") or "general")
        base_prompt = self.prompt_builder.build_generation_prompt(context, params, platform)
        account_lines = [
            f"- {acc.get('username') or acc.get('id')} ({acc.get('provider')}): max {acc.get('max_chars')} characters"
            for acc in selected_accounts
        ]
        campaign_hint = ""
        if campaign_format:
            campaign_hint = (
                f"\nCampaign format: {campaign_format.get('name') or 'unnamed'}.\n"
                f"Template structure: {json.dumps(campaign_format.get('structure') or {}, ensure_ascii=False)}\n"
                f"Required hashtags: {', '.join(self._string_list(campaign_format.get('required_hashtags'))) or 'none'}"
            )
        schedule_hint = f"\nTarget schedule (UTC): {format_datetime(scheduled_at) or 'next available slot'}."
        return (
            f"{base_prompt}\n\n"
            "Write one primary post body that uses as much of the primary character budget as possible "
            f"without exceeding {primary_limit} characters.\n"
            "Also provide shorter per-account overrides for accounts with lower limits.\n"
            "Return JSON only with keys:\n"
            '- "content": primary text for the account with the highest limit\n'
            '- "account_content_override": object mapping account_id -> tailored text\n'
            '- "hashtags": array of hashtags\n'
            '- "platform_metadata": object\n'
            f"Accounts:\n" + "\n".join(account_lines) + campaign_hint + schedule_hint
        )

    async def _generate_with_retries(
        self,
        *,
        prompt: str,
        system_prompt: str,
        primary_limit: int,
        selected_accounts: list[dict[str, Any]],
        author_user_id: str,
    ) -> dict[str, Any]:
        current_prompt = prompt
        for attempt in range(self.max_retries + 1):
            response = await self.adapter.generate(
                current_prompt,
                system_prompt,
                model=self.adapter.config.model,
                temperature=0.7,
                max_tokens=1500,
                author_user_id=author_user_id,
            )
            result = self._parse_result(response.content)
            try:
                self._validate_lengths(result, selected_accounts, primary_limit)
                return result
            except ValueError:
                if attempt >= self.max_retries:
                    raise
            current_prompt = (
                f"{prompt}\n\nRevise the previous answer and return JSON only. "
                f"The primary content must be at most {primary_limit} characters and every override must respect its account limit."
            )

        raise RuntimeError("voice engine retry loop exited unexpectedly")

    def _validate_lengths(self, result: dict[str, Any], selected_accounts: list[dict[str, Any]], primary_limit: int) -> None:
        content = str(result.get("content") or "")
        if len(content) > primary_limit:
            raise ValueError(f"Generated primary content exceeds limit of {primary_limit} characters")
        overrides = result.get("account_content_override") or {}
        if not isinstance(overrides, dict):
            raise ValueError("account_content_override must be an object")
        limits = {str(acc["id"]): int(acc["max_chars"]) for acc in selected_accounts}
        for account_id, limit in limits.items():
            override = overrides.get(account_id)
            if override is None:
                if len(content) > limit:
                    raise ValueError(f"Missing override for account {account_id} with limit {limit}")
                continue
            if len(str(override)) > limit:
                raise ValueError(f"Override for account {account_id} exceeds limit of {limit}")

    @staticmethod
    def _params(job: dict) -> dict[str, Any]:
        params = job.get("params") or {}
        if not isinstance(params, dict):
            raise ValueError("job params must be an object")
        return params

    @staticmethod
    def _selected_accounts(context: dict, params: dict[str, Any]) -> list[dict[str, Any]]:
        raw_ids = params.get("target_account_ids") or params.get("targetAccountIds") or []
        if not isinstance(raw_ids, list):
            raise ValueError("target_account_ids must be an array")
        wanted = {str(item) for item in raw_ids if str(item).strip()}
        accounts = context.get("accounts") or []
        selected = [dict(item) for item in accounts if isinstance(item, dict) and str(item.get("id") or "") in wanted]
        if len(selected) != len(wanted):
            raise ValueError("One or more target accounts were not found in team context")
        return selected

    @staticmethod
    def _optional_campaign_format(context: dict, params: dict[str, Any]) -> dict[str, Any] | None:
        campaign_format_id = str(params.get("campaign_format_id") or params.get("campaignFormatId") or "").strip()
        if not campaign_format_id:
            return None
        campaign_formats = context.get("campaign_formats") or context.get("campaignFormats") or []
        for item in campaign_formats:
            if isinstance(item, dict) and str(item.get("id") or "") == campaign_format_id:
                if item.get("is_active") is False:
                    raise ValueError(f"Campaign format {campaign_format_id} is inactive")
                return dict(item)
        raise ValueError(f"Campaign format {campaign_format_id} not found")

    @staticmethod
    def _string_list(value: Any) -> list[str]:
        if not value:
            return []
        if isinstance(value, list):
            return [str(item) for item in value if str(item).strip()]
        return [str(value)]

    @staticmethod
    def _parse_result(raw_content: str) -> dict[str, Any]:
        cleaned = raw_content.strip()
        if cleaned.startswith("```"):
            cleaned = cleaned.split("\n", 1)[-1]
            cleaned = cleaned.rsplit("```", 1)[0]
        cleaned = cleaned.strip()

        try:
            payload = json.loads(cleaned)
        except json.JSONDecodeError as exc:
            start = cleaned.find("{")
            end = cleaned.rfind("}")
            if start != -1 and end != -1:
                try:
                    payload = json.loads(cleaned[start : end + 1])
                except json.JSONDecodeError:
                    raise ValueError("LLM response was not valid JSON") from exc
            else:
                raise ValueError("LLM response was not valid JSON") from exc

        if not isinstance(payload, dict):
            raise ValueError("LLM response must be a JSON object")

        content = payload.get("content")
        hashtags = payload.get("hashtags") or []
        platform_metadata = payload.get("platform_metadata") or {}
        overrides = payload.get("account_content_override") or {}

        if not isinstance(content, str) or not content.strip():
            raise ValueError("LLM response missing content")
        if not isinstance(hashtags, list):
            raise ValueError("LLM response hashtags must be a list")
        if not isinstance(platform_metadata, dict):
            raise ValueError("LLM response platform_metadata must be an object")
        if not isinstance(overrides, dict):
            raise ValueError("LLM response account_content_override must be an object")

        return {
            "content": content.strip(),
            "hashtags": [str(hashtag) for hashtag in hashtags],
            "platform_metadata": platform_metadata,
            "account_content_override": {str(key): str(value).strip() for key, value in overrides.items() if str(value).strip()},
        }
