FROM debian:trixie AS base-builder
RUN apt-get update \
    && apt-get install -y --no-install-recommends bash curl git ca-certificates \
    && rm -rf /var/lib/apt/lists/*
ENV MISE_INSTALL_PATH="/usr/local/bin/mise" \
    PATH="/root/.local/share/mise/shims:$PATH"
WORKDIR /app
RUN curl https://mise.run | sh
COPY mise.toml ./
RUN mise install


FROM base-builder AS deps
RUN GOBIN=/usr/local/bin go install github.com/cli/cli/v2/cmd/gh@latest
RUN gh --help
RUN curl -fsSL https://opencode.ai/install | bash
ENV PATH="/root/.opencode/bin:$PATH"
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
		mise build

FROM deps AS runner
CMD ["./op-reviewer"]
