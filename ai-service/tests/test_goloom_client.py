import json

import httpx
import pytest

from app.services import GoloomClient


def make_client(response_json: dict | None, status_code: int = 200):
    captured: dict = {}

    def handler(request: httpx.Request) -> httpx.Response:
        captured["method"] = request.method
        captured["url"] = str(request.url)
        captured["authorization"] = request.headers.get("Authorization")
        captured["json"] = json.loads(request.content.decode()) if request.content else None
        return httpx.Response(status_code, json=response_json, request=request)

    transport = httpx.MockTransport(handler)
    client = GoloomClient("http://goloom.test", "secret-token", transport=transport)
    return client, captured


@pytest.mark.asyncio
async def test_get_ai_context_calls_correct_url_with_auth_header():
    client, captured = make_client({"team": {"id": "team-1"}})

    response = await client.get_ai_context("team-1")

    assert response == {"team": {"id": "team-1"}}
    assert captured["method"] == "GET"
    assert captured["url"] == "http://goloom.test/v1/teams/team-1/ai-context"
    assert captured["authorization"] == "Bearer secret-token"


@pytest.mark.asyncio
async def test_send_callback_sends_correct_payload():
    client, captured = make_client({"acknowledged": True})

    await client.send_callback(
        job_id="job-1",
        status="completed",
        result={"content": "Hello world"},
        error_message="",
    )

    assert captured["method"] == "POST"
    assert captured["url"] == "http://goloom.test/v1/webhooks/ai-callback"
    assert captured["authorization"] == "Bearer secret-token"
    assert captured["json"] == {
        "job_id": "job-1",
        "status": "completed",
        "result": {"content": "Hello world"},
        "error_message": "",
    }


@pytest.mark.asyncio
async def test_create_draft_sends_correct_body():
    client, captured = make_client({"id": "post-1", "content": "Draft text"}, status_code=201)

    response = await client.create_draft("team-1", "Draft text", ["acct-1", "acct-2"])

    assert response == {"id": "post-1", "content": "Draft text"}
    assert captured["method"] == "POST"
    assert captured["url"] == "http://goloom.test/v1/teams/team-1/posts/draft"
    assert captured["json"] == {
        "content": "Draft text",
        "account_ids": ["acct-1", "acct-2"],
    }


@pytest.mark.asyncio
async def test_non_2xx_responses_raise_http_status_error():
    client, _ = make_client({"error": "boom"}, status_code=500)

    with pytest.raises(httpx.HTTPStatusError):
        await client.get_ai_context("team-1")
