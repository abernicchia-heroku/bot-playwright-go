# Adapted for Heroku from https://github.com/playwright-community/playwright-go/blob/v0.4802.0/Dockerfile.example
# Stage 1: Modules caching
FROM golang:1.22 AS modules
COPY go.mod go.sum /modules/
WORKDIR /modules
RUN go mod download

# Stage 2: Build
FROM golang:1.22 AS builder
COPY --from=modules /go/pkg /go/pkg
COPY . /workdir
WORKDIR /workdir
# Install playwright cli with right version for later use
RUN PWGO_VER=$(grep -oE "playwright-go v\S+" /workdir/go.mod | sed 's/playwright-go //g') \
    && go install github.com/playwright-community/playwright-go/cmd/playwright@${PWGO_VER}
# Build your app
RUN GOOS=linux GOARCH=amd64 go build -o /bin/bot-playwright-go

# Stage 3: Final
FROM heroku/heroku:24

USER root

RUN apt-get update \
    && apt-get install -y ca-certificates tzdata

RUN useradd -m -d /app dyno && mkdir -p /app

COPY --from=builder /go/bin/playwright /bin/bot-playwright-go /app/

RUN chown -R dyno:dyno /app

# Install playwright dependencies as root (required)
RUN /app/playwright install-deps && rm -rf /var/lib/apt/lists/*

USER dyno

# Install playwright browsers (in this case only Firefox is required, removing this it will install all the default browsers). 
# It will install under /app/.cache (user home dir) and can be executed by non-root user (dyno)
RUN /app/playwright install firefox

CMD ["/app/bot-playwright-go"]