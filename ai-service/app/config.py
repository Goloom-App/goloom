from typing import Optional

from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    goloom_api_url: str
    goloom_api_token: str
    llm_router_provider: str = "openai"
    llm_router_model: str = "gpt-4o-mini"
    llm_generator_provider: str = "openai"
    llm_generator_model: str = "gpt-4o"
    redis_url: Optional[str] = None
    log_level: str = "INFO"

    proactive_enabled: bool = True
    proactive_interval_seconds: int = 3600

    class Config:
        env_file = ".env"


settings = Settings()  # pyright: ignore[reportCallIssue]
