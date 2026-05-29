import pytest
from pydantic import ValidationError
from pydantic_settings import BaseSettings


def test_config_requires_goloom_api_url(monkeypatch):
    monkeypatch.delenv("GOLOOM_API_URL", raising=False)
    monkeypatch.delenv("GOLOOM_API_TOKEN", raising=False)
    class TestSettings(BaseSettings):
        goloom_api_url: str
        goloom_api_token: str

    with pytest.raises(ValidationError):
        TestSettings()  # pyright: ignore[reportCallIssue]
