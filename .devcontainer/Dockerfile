FROM mcr.microsoft.com/devcontainers/go:1.26-trixie

USER vscode

# renovate: datasource=github-releases depName=golangci/golangci-lint extractVersion=^v(?<version>.+)$
ARG GOLANGCI_VERSION=2.12.2
RUN curl -sSfL  https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
  | sudo sh -s -- v${GOLANGCI_VERSION}

# renovate: datasource=github-releases depName=vektra/mockery extractVersion=^v(?<version>.+)$
ARG MOCKERY_VERSION=3.7.0
RUN curl -sSfL https://github.com/vektra/mockery/releases/download/v${MOCKERY_VERSION}/mockery_${MOCKERY_VERSION}_Linux_x86_64.tar.gz \
  | sudo tar -C /usr/local/bin -xzf - mockery
