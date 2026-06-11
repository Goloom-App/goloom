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
            scheduled_at = None if params.get("schedule") is False else resolve_scheduled_at(
                params=params, campaign_format=campaign_format, context=context
            )

            primary = max(selected_accounts, key=lambda item: int(item["max_chars"]))
            primary_platform = str(primary.get("provider") or "general")
            primary_limit = int(primary["max_chars"])

            system_prompt = self.prompt_builder.build_system_prompt(context)
            primary_account_id = str(primary.get("id") or "")
            refine_mode = self._is_refine_mode(params)
            include_title = self._include_title_in_response(params, refine_mode)
            if refine_mode:
                base_prompt = self._build_refine_prompt(
                    context=context,
                    params=params,
                    selected_accounts=selected_accounts,
                    primary_limit=primary_limit,
                    primary_account_id=primary_account_id,
                    campaign_format=campaign_format,
                    scheduled_at=scheduled_at,
                    include_title=include_title,
                )
            else:
                base_prompt = self._build_multi_account_prompt(
                    context=context,
                    params=params,
                    selected_accounts=selected_accounts,
                    primary_limit=primary_limit,
                    campaign_format=campaign_format,
                    scheduled_at=scheduled_at,
                    include_title=include_title,
                )
            prompt = base_prompt

            parsed = await self._generate_with_retries(
                prompt=prompt,
                system_prompt=system_prompt,
                primary_limit=primary_limit,
                primary_account_id=primary_account_id,
                selected_accounts=selected_accounts,
                author_user_id=author_user_id,
                refine_mode=refine_mode,
                include_title=include_title,
            )
            parsed = self._normalize_multi_account_result(
                parsed,
                selected_accounts=selected_accounts,
                primary_account_id=primary_account_id,
                primary_limit=primary_limit,
            )

            result = {
                "content": parsed["content"],
                "hashtags": parsed.get("hashtags") or [],
                "platform_metadata": parsed.get("platform_metadata") or {},
                "account_content_override": parsed.get("account_content_override") or {},
                "scheduled_at": format_datetime(scheduled_at),
                "primary_account_id": primary_account_id,
            }
            if parsed.get("title"):
                result["title"] = parsed["title"]
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
        include_title: bool = False,
    ) -> str:
        primary = max(selected_accounts, key=lambda item: int(item["max_chars"]))
        primary_id = str(primary.get("id") or "")
        primary_platform = str(primary.get("provider") or "general")
        base_prompt = self.prompt_builder.build_generation_prompt(context, params, primary_platform)
        account_lines = [
            f"- {acc.get('username') or acc.get('id')} (id={acc.get('id')}, {acc.get('provider')}): max {acc.get('max_chars')} characters"
            for acc in selected_accounts
        ]
        lower_limit_accounts = [
            acc for acc in selected_accounts if int(acc["max_chars"]) < primary_limit
        ]
        override_hint = (
            "No account_content_override entries are needed because every selected account shares the same limit."
            if not lower_limit_accounts
            else (
                "account_content_override must ONLY contain compressed variants for these lower-limit accounts "
                "when the primary text would exceed their limit: "
                + ", ".join(
                    f"{acc.get('username') or acc.get('id')} (id={acc.get('id')}, max {acc.get('max_chars')})"
                    for acc in lower_limit_accounts
                )
            )
        )
        schedule_hint = f"\nTarget schedule (UTC): {format_datetime(scheduled_at) or 'next available slot'}."
        title_hint = self._title_json_instruction(params, include_title)
        rss_rules = self._rss_generation_rules(params)
        return (
            f"{base_prompt}\n\n"
            f"{rss_rules}"
            "Multi-account output rules:\n"
            f"- Primary account: {primary.get('username') or primary_id} (id={primary_id}, {primary_platform}, "
            f"limit {primary_limit} characters).\n"
            f"- Write \"content\" ONLY for the primary account. Make it as long and complete as possible, "
            f"targeting roughly {primary_limit - 20} to {primary_limit} characters.\n"
            f"- {override_hint}\n"
            "- Do NOT create a separate version for every account.\n"
            "- Accounts with the same or higher limit than the primary use \"content\" unchanged.\n"
            "- Overrides must be shorter compressions of the same message, not alternate drafts.\n"
            "Return JSON only with keys:\n"
            f'{title_hint}- "content": primary text for account id {primary_id}\n'
            '- "account_content_override": object mapping account_id -> shorter text ONLY where required\n'
            '- "hashtags": array of hashtags\n'
            '- "platform_metadata": object\n'
            f"Accounts:\n" + "\n".join(account_lines) + schedule_hint
        )

    def _build_refine_prompt(
        self,
        *,
        context: dict,
        params: dict[str, Any],
        selected_accounts: list[dict[str, Any]],
        primary_limit: int,
        primary_account_id: str,
        campaign_format: dict[str, Any] | None,
        scheduled_at,
        include_title: bool = False,
    ) -> str:
        primary = max(selected_accounts, key=lambda item: int(item["max_chars"]))
        primary_platform = str(primary.get("provider") or "general")
        source_content = str(params.get("source_content") or params.get("existing_content") or "").strip()
        refinement_hint = str(params.get("prompt_hint") or params.get("instruction") or "").strip()
        if not refinement_hint:
            refinement_hint = (
                "Improve clarity, flow, and engagement while preserving the core message and team voice."
            )
        refine_params = {
            **params,
            "prompt_hint": refinement_hint,
        }
        base_prompt = self.prompt_builder.build_generation_prompt(context, refine_params, primary_platform)
        account_lines = [
            f"- {acc.get('username') or acc.get('id')} (id={acc.get('id')}, {acc.get('provider')}): max {acc.get('max_chars')} characters"
            for acc in selected_accounts
        ]
        lower_limit_accounts = [
            acc for acc in selected_accounts if int(acc["max_chars"]) < primary_limit
        ]
        override_hint = (
            "No account_content_override entries are needed when the refined primary text fits every account."
            if not lower_limit_accounts
            else (
                "account_content_override must ONLY contain compressed variants for: "
                + ", ".join(
                    f"{acc.get('username') or acc.get('id')} (id={acc.get('id')}, max {acc.get('max_chars')})"
                    for acc in lower_limit_accounts
                )
            )
        )
        schedule_hint = f"\nTarget schedule (UTC): {format_datetime(scheduled_at) or 'unchanged'}."
        post_kind_section = self._recurring_post_kind_section(params)
        title_hint = self._title_json_instruction(params, include_title)
        return (
            f"{base_prompt}\n\n"
            "Refine an existing draft for multi-account publishing.\n"
            f"{post_kind_section}"
            f"Primary account: {primary.get('username') or primary_account_id} (id={primary_account_id}, "
            f"limit {primary_limit} characters).\n"
            f"Refinement goal: {refinement_hint}\n"
            f"- \"content\": refined primary text for account id {primary_account_id}; "
            f"MUST NOT exceed {primary_limit} characters (hard limit)\n"
            f"- The source draft is {len(source_content)} characters; keep the refined text within {primary_limit}\n"
            f"- {override_hint}\n"
            f"{title_hint}"
            "- Do NOT create a separate version for every account.\n"
            "- Overrides must be shorter compressions of the refined primary text.\n"
            f"Return JSON only with keys {self._title_json_keys(include_title)}content, account_content_override, hashtags, platform_metadata.\n"
            f"Accounts:\n" + "\n".join(account_lines) + schedule_hint
        )

    @staticmethod
    def _recurring_post_kind_section(params: dict[str, Any]) -> str:
        kind = str(params.get("recurring_post_kind") or "").strip().lower()
        if kind == "announcement":
            return (
                "This is a recurring-template ANNOUNCEMENT post (teaser before the main event). "
                "Keep it shorter, build anticipation, and align tone with the paired main post.\n"
            )
        if kind == "main":
            return (
                "This is the MAIN recurring post. If an announcement reference is provided below, "
                "stay consistent with its wording, promises, and tone while expanding into the full post.\n"
            )
        return ""

    @staticmethod
    def _announcement_reference_section(params: dict[str, Any]) -> str:
        content = str(params.get("announcement_reference_content") or "").strip()
        title = str(params.get("announcement_reference_title") or "").strip()
        main_event = str(params.get("main_event_at") or "").strip()
        if not content and not title:
            return ""
        lines = ["Paired announcement reference (keep the main post aligned with this teaser):"]
        if main_event:
            lines.append(f"Main event schedule (UTC): {main_event}")
        if title:
            lines.append(f"Announcement title: {title}")
        if content:
            lines.append(f"Announcement text:\n---\n{content}\n---")
        return "\n".join(lines) + "\n"

    @staticmethod
    def _rss_article_section(params: dict[str, Any]) -> str:
        title = str(params.get("rss_article_title") or "").strip()
        content = str(params.get("rss_article_content") or params.get("rss_article_summary") or "").strip()
        link = str(params.get("rss_article_link") or "").strip()
        if not title and not content and not link:
            return ""
        lines = ["Source RSS article (use as factual basis for the post):"]
        if title:
            lines.append(f"Title: {title}")
        if link:
            lines.append(f"Link: {link}")
        if content:
            lines.append(f"Article text:\n---\n{content}\n---")
        return "\n".join(lines) + "\n"

    @staticmethod
    def _include_title_in_response(params: dict[str, Any], refine_mode: bool) -> bool:
        if not refine_mode:
            return True
        return bool(str(params.get("title_hint") or "").strip())

    @staticmethod
    def _title_json_keys(include_title: bool) -> str:
        return "title, " if include_title else ""

    def _title_json_instruction(self, params: dict[str, Any], include_title: bool) -> str:
        if not include_title:
            return ""
        hint = str(params.get("title_hint") or "").strip()
        if hint:
            return f'- "title": internal Goloom post title (max 120 characters). Instruction: {hint}\n'
        return '- "title": short internal Goloom post title (max 120 characters) for editors, based on the post content\n'

    @staticmethod
    def _normalize_title(value: Any, *, required: bool) -> str:
        title = str(value or "").strip()
        if len(title) > 120:
            title = title[:120].rstrip()
        if required and not title:
            raise ValueError("LLM response missing title")
        return title

    @staticmethod
    def _is_refine_mode(params: dict[str, Any]) -> bool:
        if params.get("rss_automation") or str(params.get("rss_article_title") or "").strip():
            return params.get("refine_content") is True or params.get("refine") is True
        if params.get("refine_content") is True or params.get("refine") is True:
            return True
        return bool(str(params.get("source_content") or params.get("existing_content") or "").strip())

    @staticmethod
    def _rss_generation_rules(params: dict[str, Any]) -> str:
        if not params.get("rss_automation") and not str(params.get("rss_article_title") or "").strip():
            return ""
        return (
            "RSS / episode rules:\n"
            "- This is a NEW post written from the show notes, not a polish of a previous draft.\n"
            "- Copy the episode title and number from the source material exactly — do not invent #382 when the source says #381.\n"
            "- Use the episode page link from the source — never substitute the RSS feed URL.\n"
            "- Include at least two concrete details from the show notes; skip generic filler about Open Source or cloud trends unless they are in the notes.\n\n"
        )

    async def _generate_with_retries(
        self,
        *,
        prompt: str,
        system_prompt: str,
        primary_limit: int,
        primary_account_id: str,
        selected_accounts: list[dict[str, Any]],
        author_user_id: str,
        refine_mode: bool = False,
        include_title: bool = False,
    ) -> dict[str, Any]:
        current_prompt = prompt
        last_error = "invalid multi-account output"
        for attempt in range(self.max_retries + 1):
            response = await self.adapter.generate(
                current_prompt,
                system_prompt,
                model=self.adapter.config.model,
                temperature=0.7,
                max_tokens=1500,
                author_user_id=author_user_id,
            )
            result = self._parse_result(response.content, include_title=include_title, refine_mode=refine_mode)
            result = self._normalize_multi_account_result(
                result,
                selected_accounts=selected_accounts,
                primary_account_id=primary_account_id,
                primary_limit=primary_limit,
            )
            try:
                self._validate_lengths(
                    result,
                    selected_accounts,
                    primary_limit=primary_limit,
                    primary_account_id=primary_account_id,
                    refine_mode=refine_mode,
                )
                return result
            except ValueError as exc:
                last_error = str(exc)
                if attempt >= self.max_retries:
                    raise
            if refine_mode:
                current_prompt = (
                    f"{prompt}\n\nRevise the previous answer and return JSON only. "
                    f"Fix this issue: {last_error}. "
                    f"The primary content MUST be at most {primary_limit} characters. "
                    "Keep the refined primary text faithful to the source draft while improving quality. "
                    "Only add account_content_override entries for lower-limit accounts that cannot fit the primary text."
                )
            else:
                current_prompt = (
                    f"{prompt}\n\nRevise the previous answer and return JSON only. "
                    f"Fix this issue: {last_error}. "
                    f"The primary content for account {primary_account_id} must be long "
                    f"(target {self._min_primary_length(primary_limit, selected_accounts)} to {primary_limit} characters). "
                    "Only add account_content_override entries for lower-limit accounts that cannot fit the primary text."
                )

        raise RuntimeError("voice engine retry loop exited unexpectedly")

    @staticmethod
    def _min_primary_length(primary_limit: int, selected_accounts: list[dict[str, Any]]) -> int:
        lower_limits = [
            int(acc["max_chars"])
            for acc in selected_accounts
            if int(acc["max_chars"]) < primary_limit
        ]
        if lower_limits:
            return max(min(primary_limit - 20, int(primary_limit * 0.85)), max(lower_limits) + 1)
        return max(min(primary_limit - 20, int(primary_limit * 0.85)), int(primary_limit * 0.7))

    @staticmethod
    def _truncate_to_limit(text: str, limit: int) -> str:
        if limit <= 0:
            return ""
        cleaned = text.strip()
        if len(cleaned) <= limit:
            return cleaned
        if limit == 1:
            return cleaned[:1]
        clipped = cleaned[:limit]
        window_start = max(0, limit - max(20, limit // 6))
        slice_ = clipped[window_start:limit]
        last_space = slice_.rfind(" ")
        if last_space > 0:
            return clipped[: window_start + last_space].rstrip()
        return clipped.rstrip()

    @staticmethod
    def _normalize_multi_account_result(
        result: dict[str, Any],
        *,
        selected_accounts: list[dict[str, Any]],
        primary_account_id: str,
        primary_limit: int,
    ) -> dict[str, Any]:
        content = str(result.get("content") or "")
        if len(content) > primary_limit:
            content = VoiceEngineWorker._truncate_to_limit(content, primary_limit)
        raw_overrides = VoiceEngineWorker._coerce_account_content_override(result.get("account_content_override"))
        limits = {str(acc["id"]): int(acc["max_chars"]) for acc in selected_accounts}
        overrides: dict[str, str] = {}

        for account_id, value in raw_overrides.items():
            account_key = str(account_id)
            if account_key == primary_account_id:
                continue
            limit = limits.get(account_key)
            if limit is None or limit >= primary_limit:
                continue
            text = str(value).strip()
            if not text:
                continue
            if len(content) <= limit:
                continue
            if len(text) > limit:
                text = VoiceEngineWorker._truncate_to_limit(text, limit)
            if len(text) >= len(content):
                continue
            overrides[account_key] = text

        result["content"] = content
        result["account_content_override"] = overrides
        return result

    def _validate_lengths(
        self,
        result: dict[str, Any],
        selected_accounts: list[dict[str, Any]],
        *,
        primary_limit: int,
        primary_account_id: str,
        refine_mode: bool = False,
    ) -> None:
        content = str(result.get("content") or "")
        if not refine_mode:
            min_primary = self._min_primary_length(primary_limit, selected_accounts)
            if len(content) < min_primary:
                raise ValueError(
                    f"Primary content is too short ({len(content)} chars); aim for at least {min_primary}"
                )
        if len(content) < 1:
            raise ValueError("Primary content is empty")
        if len(content) > primary_limit:
            raise ValueError(f"Generated primary content exceeds limit of {primary_limit} characters")
        overrides = result.get("account_content_override") or {}
        if not isinstance(overrides, dict):
            raise ValueError("account_content_override must be an object")
        limits = {str(acc["id"]): int(acc["max_chars"]) for acc in selected_accounts}

        if primary_account_id in overrides:
            raise ValueError("Primary account must not appear in account_content_override")

        for account_id, limit in limits.items():
            if account_id == primary_account_id:
                continue
            override = overrides.get(account_id)
            if len(content) <= limit:
                if override is not None:
                    raise ValueError(f"Remove override for account {account_id}; primary content already fits")
                continue
            if override is None:
                raise ValueError(f"Missing override for account {account_id} with limit {limit}")
            override_text = str(override)
            if len(override_text) > limit:
                raise ValueError(f"Override for account {account_id} exceeds limit of {limit}")
            if len(override_text) >= len(content):
                raise ValueError(f"Override for account {account_id} must be shorter than the primary content")

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
    def _coerce_account_content_override(value: Any) -> dict[str, str]:
        if value is None:
            return {}
        if isinstance(value, dict):
            return {
                str(key): str(item).strip()
                for key, item in value.items()
                if str(key).strip() and str(item).strip()
            }
        if isinstance(value, list):
            overrides: dict[str, str] = {}
            for item in value:
                if not isinstance(item, dict):
                    continue
                account_id = str(
                    item.get("account_id") or item.get("accountId") or item.get("id") or ""
                ).strip()
                text = str(item.get("content") or item.get("text") or item.get("override") or "").strip()
                if account_id and text:
                    overrides[account_id] = text
            return overrides
        if isinstance(value, str):
            cleaned = value.strip()
            if not cleaned or cleaned.casefold() in {"null", "none", "n/a"}:
                return {}
            try:
                parsed = json.loads(cleaned)
            except json.JSONDecodeError:
                return {}
            return VoiceEngineWorker._coerce_account_content_override(parsed)
        return {}

    @staticmethod
    def _parse_result(raw_content: str, *, include_title: bool = False, refine_mode: bool = False) -> dict[str, Any]:
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
        overrides = VoiceEngineWorker._coerce_account_content_override(payload.get("account_content_override"))

        if not isinstance(content, str) or not content.strip():
            raise ValueError("LLM response missing content")
        if not isinstance(hashtags, list):
            hashtags = []
        if not isinstance(platform_metadata, dict):
            platform_metadata = {}

        parsed = {
            "content": content.strip(),
            "hashtags": [str(hashtag) for hashtag in hashtags],
            "platform_metadata": platform_metadata,
            "account_content_override": overrides,
        }
        if include_title:
            title = VoiceEngineWorker._normalize_title(payload.get("title"), required=False)
            if title:
                parsed["title"] = title
            elif not refine_mode:
                raise ValueError("LLM response missing title")
        return parsed
