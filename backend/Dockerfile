FROM golang:1.25-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends build-essential && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/ai-workbench ./cmd/server

FROM debian:bookworm-slim
RUN DEBIAN_FRONTEND=noninteractive apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    ca-certificates fonts-noto-cjk ghostscript gosu imagemagick libreoffice \
    poppler-utils python3 python3-venv qpdf tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system workbench \
    && useradd --system --gid workbench --home-dir /nonexistent --shell /usr/sbin/nologin workbench \
    && python3 -m venv /opt/ai-workbench-filetools \
    && /opt/ai-workbench-filetools/bin/pip install --no-cache-dir \
    pdf2docx==0.5.8 PyMuPDF==1.28.2 python-docx==1.2.0 fonttools==4.63.0 \
    numpy==2.4.6 opencv-python-headless==5.0.0.93 fire==0.7.1 \
    lxml==6.1.2 typing_extensions==4.16.0 termcolor==3.3.0
RUN DEBIAN_FRONTEND=noninteractive apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends wget \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/ai-workbench /usr/local/bin/ai-workbench
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
COPY scripts /app/scripts
RUN chmod 755 /usr/local/bin/entrypoint.sh && mkdir -p /data/attachments && chown -R workbench:workbench /data
EXPOSE 8087
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["/usr/local/bin/ai-workbench"]
