ARG BIN_NAME=lychee-ai-organizer
ARG BIN_VERSION=<unknown>

FROM golang:1-alpine AS builder
ARG BIN_NAME
ARG BIN_VERSION
RUN update-ca-certificates
WORKDIR /src/${BIN_NAME}
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-X main.version=${BIN_VERSION}" -o ./out/${BIN_NAME} .

FROM scratch
ARG BIN_NAME
ARG BIN_VERSION
COPY --from=builder /src/${BIN_NAME}/out/${BIN_NAME} /usr/bin/${BIN_NAME}
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/usr/bin/lychee-ai-organizer"]

LABEL license="GPL3"
LABEL maintainer="Chris Dzombak <https://www.dzombak.com>"
LABEL org.opencontainers.image.authors="Chris Dzombak <https://www.dzombak.com>"
LABEL org.opencontainers.image.url="https://github.com/cdzombak/${BIN_NAME}"
LABEL org.opencontainers.image.documentation="https://github.com/cdzombak/${BIN_NAME}/blob/main/README.md"
LABEL org.opencontainers.image.source="https://github.com/cdzombak/${BIN_NAME}.git"
LABEL org.opencontainers.image.version="${BIN_VERSION}"
LABEL org.opencontainers.image.licenses="GPL3"
LABEL org.opencontainers.image.title="${BIN_NAME}"
LABEL org.opencontainers.image.description="Use local LLMs to organize your unsorted photos in Lychee"
