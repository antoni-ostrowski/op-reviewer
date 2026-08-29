FROM debian:trixie AS base-builder
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl git ca-certificates \
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
		mise build

FROM debian:trixie AS runner
WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl git ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=deps /app/op-reviewer .
CMD ["./op-reviewer"]
