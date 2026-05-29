FROM python:3.11-slim AS builder
WORKDIR /app
COPY pyproject.toml ./
COPY app/ ./app/
RUN pip install --no-cache-dir -e .

FROM python:3.11-slim
WORKDIR /app
COPY --from=builder /app /app
COPY --from=builder /usr/local/lib/python3.11/site-packages /usr/local/lib/python3.11/site-packages
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8090"]
