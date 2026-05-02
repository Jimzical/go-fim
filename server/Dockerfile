FROM python:3.12-slim
WORKDIR /app
# Convert Pipfile.lock → requirements.txt so we don't need pipenv at runtime
# and dodge --deploy's strict python_version check (Pipfile pins "3", which
# matches no exact release).
COPY Pipfile Pipfile.lock ./
RUN pip install --no-cache-dir pipenv \
    && pipenv requirements > requirements.txt \
    && pip install --no-cache-dir -r requirements.txt \
    && pip uninstall -y pipenv
COPY . .
EXPOSE 8000
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8000"]
