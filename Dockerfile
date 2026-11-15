# The image of the server: the API and the web app on one origin. The stages run on the build machine and
# cross-compile, so an arm64 image costs no emulation. `--target server` stops before the web app: the API alone,
# for `make api` (compose.yaml), which mounts no app.
FROM --platform=$BUILDPLATFORM oven/bun:1.4.4 AS web
WORKDIR /src
COPY web/package.json web/bun.lock ./
COPY web/patches ./patches
RUN bun install --frozen-lockfile
COPY web .
# Empty API base: the app calls /api on its own origin.
RUN NUXT_PUBLIC_API_BASE= bun --bun run generate

FROM --platform=$BUILDPLATFORM golang:1.27 AS api
ARG TARGETOS TARGETARCH
WORKDIR /src
# Modules first: a code change does not download them again.
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY config ./config
COPY core ./core
COPY cli ./cli
# The version shown to the admin: the context has no .git for go build to read the commit from.
ARG VERSION
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags "-X github.com/cyril-fl/tide/config.version=$VERSION" -o /tide-server ./cmd/tide-server
# scratch has no /tmp, where an import writes its body instead of holding it in memory (#119).
RUN install -d /image-tmp

FROM scratch AS server
COPY --from=api /tide-server /tide-server
COPY --from=api --chown=65534:65534 /image-tmp /tmp
USER 65534
EXPOSE 8080
ENTRYPOINT ["/tide-server"]

FROM server
COPY --from=web /src/.output/public /web
CMD ["serve", "--addr", ":8080", "--web", "/web"]
