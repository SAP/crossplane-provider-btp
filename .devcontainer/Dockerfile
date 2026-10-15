FROM mcr.microsoft.com/devcontainers/go:1.24-bookworm@sha256:2fe929afae562b8c50a28052aa6bc772e8045578fcc7b0508bb6da8935da3fb1

# Remove expired yarn repo before features try to run apt-get update
RUN rm -f /etc/apt/sources.list.d/yarn.list