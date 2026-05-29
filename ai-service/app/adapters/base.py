from abc import ABC, abstractmethod

from .models import LLMConfig, LLMResponse


class LLMAdapter(ABC):
    def __init__(self, config: LLMConfig):
        self.config = config

    @abstractmethod
    async def generate(self, prompt: str, system: str = "", **kwargs) -> LLMResponse:
        """Generate text from prompt."""

    @abstractmethod
    async def classify(self, prompt: str, system: str = "", options: list[str] = []) -> str:
        """Classify prompt into one of the options."""
