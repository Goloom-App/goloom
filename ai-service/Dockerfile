FROM python:3.14-slim AS builder
WORKDIR /app
COPY pyproject.toml ./
COPY app/ ./app/
RUN pip install --no-cache-dir .

FROM python:3.14-slim
WORKDIR /app
COPY --from=builder /usr/local /usr/local
COPY --from=builder /app /app
CMD ["python", "-m", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8090"]
