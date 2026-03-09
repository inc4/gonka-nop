FROM python:3.11-slim

RUN pip install --no-cache-dir requests==2.31.0 prometheus_client==0.19.0

WORKDIR /app
COPY exporter.py .

CMD ["python", "-u", "exporter.py"]
