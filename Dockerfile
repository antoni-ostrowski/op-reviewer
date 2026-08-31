FROM debian:trixie AS base-builder
RUN apt-get update \
    && apt-get install -y --no-install-recommends bash curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*
ENV MISE_INSTALL_PATH="/usr/local/bin/mise" \
    PATH="/root/.local/share/mise/shims:$PATH"
WORKDIR /app
RUN curl https://mise.run | sh
COPY mise.toml ./
RUN mise install

FROM base-builder AS deps
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    mise run build

FROM debian:trixie-slim AS runner
RUN export DEBIAN_FRONTEND=noninteractive \
    && apt-get update \
    && apt-get install -y --no-install-recommends bash ca-certificates curl gh git \
    && rm -rf /var/lib/apt/lists/* \
    && curl -fsSL https://opencode.ai/install | bash
ENV PATH="/root/.opencode/bin:$PATH"
WORKDIR /app
COPY --from=deps /app/op-reviewer /usr/local/bin/op-reviewer
CMD ["/usr/local/bin/op-reviewer"]
