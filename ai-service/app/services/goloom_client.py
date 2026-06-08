from __future__ import annotations

from typing import Any

import httpx

from app.proactive_defaults import default_proactive_settings


class GoloomClient:
    def __init__(
        self,
        base_url: str,
        api_token: str,
        *,
        timeout: float = 30.0,
        transport: httpx.AsyncBaseTransport | None = None,
    ):
        self.base_url = base_url.rstrip("/")
        self.api_token = api_token
        self.timeout = timeout
        self.transport = transport

    async def get_ai_context(self, team_id: str) -> dict:
        return await self._request("GET", f"/v1/teams/{team_id}/ai-context")

    async def create_draft(self, team_id: str, content: str, account_ids: list) -> dict:
        return await self._request(
            "POST",
            f"/v1/teams/{team_id}/posts/draft",
            json={"content": content, "account_ids": account_ids},
        )

    async def send_callback(
        self,
        job_id: str,
        status: str,
        result: dict,
        error_message: str = "",
    ) -> None:
        await self._request(
            "POST",
            "/v1/webhooks/ai-callback",
            json={
                "job_id": job_id,
                "status": status,
                "result": result,
                "error_message": error_message,
            },
        )

    async def trigger_job(self, team_id: str, job_type: str, params: dict) -> dict:
        return await self._request(
            "POST",
            f"/v1/teams/{team_id}/ai-trigger",
            json={"type": job_type, "params": params},
        )

    async def list_ai_enabled_teams(self) -> list:
        payload = await self._request("GET", "/v1/admin/ai-enabled-teams")
        return payload.get("items", [])

    async def get_proactive_settings(self, team_id: str) -> dict:
        try:
            payload = await self._request("GET", f"/v1/teams/{team_id}/proactive-settings")
            if isinstance(payload, dict):
                return payload
        except httpx.HTTPStatusError as exc:
            if exc.response.status_code != 404:
                raise
        return default_proactive_settings()

    async def update_team_profile(self, team_id: str, profile: dict) -> dict:
        # Read current to preserve auto_publish_enabled
        current = {}
        try:
            current = await self._request("GET", f"/v1/teams/{team_id}/profile")
        except httpx.HTTPStatusError:
            pass  # first analysis, no existing profile

        payload: dict[str, Any] = {
            "style_metadata": profile,
            "auto_publish_enabled": current.get("auto_publish_enabled", False),
        }
        return await self._request("PUT", f"/v1/teams/{team_id}/profile", json=payload)

    async def create_style_example(self, team_id: str, example: dict) -> dict:
        return await self._request("POST", f"/v1/teams/{team_id}/style-examples", json=example)

    async def _request(self, method: str, path: str, json: dict[str, Any] | None = None) -> Any:
        async with httpx.AsyncClient(
            base_url=self.base_url,
            headers={
                "Authorization": f"Bearer {self.api_token}",
                "Content-Type": "application/json",
            },
            timeout=self.timeout,
            transport=self.transport,
        ) as client:
            response = await client.request(method, path, json=json)
            response.raise_for_status()
            if not response.content:
                return None
            return response.json()
