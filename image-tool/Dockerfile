FROM python:3.11-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    U2NET_HOME=/opt/models \
    OMP_NUM_THREADS=2 \
    ORT_LOG_SEVERITY_LEVEL=3

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt \
    && mkdir -p "$U2NET_HOME" \
    && python -c 'from rembg import new_session; new_session("u2net_human_seg")'

COPY app.py .

RUN useradd --system --uid 10001 --home-dir /app image-tool \
    && chown -R image-tool:image-tool /app "$U2NET_HOME"

USER image-tool

EXPOSE 8090

CMD ["python", "/app/app.py"]
