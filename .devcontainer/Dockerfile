FROM mcr.microsoft.com/devcontainers/go:2.1-bookworm

# Remove expired yarn repo before features try to run apt-get update
RUN rm -f /etc/apt/sources.list.d/yarn.list