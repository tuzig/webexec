# Use Go 1.22 on Debian as base image
FROM golang:1.22-bookworm

# Install common development tools
RUN apt-get update && apt-get install -y \
    git \
    curl \
    ripgrep \
    build-essential \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install just task runner
RUN curl --proto '=https' --tlsv1.2 -sSf https://just.systems/install.sh | bash -s -- --to /usr/local/bin

# Copy Justfile and run bootstrap
COPY Justfile .
RUN just bootstrap

# Copy bashrc for shell configuration
COPY .agents/sandbox/bashrc /root/.bashrc

# Default command
CMD ["just install && /bin/bash"]
