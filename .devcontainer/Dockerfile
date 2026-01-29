FROM mcr.microsoft.com/devcontainers/go:1.24-bookworm

# Remove expired yarn repo before features try to run apt-get update
RUN rm -f /etc/apt/sources.list.d/yarn.list