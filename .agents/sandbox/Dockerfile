FROM golang:1.25-bookworm

# Install common development tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    ripgrep \
    ca-certificates \
    make \
    gcc \
    pkg-config \
    && rm -rf /var/lib/apt/lists/*

# Install just from GitHub releases (not in Debian apt; it's a Rust binary)
RUN ARCH=$(uname -m) && \
    JUST_URL=$(curl -sSL https://api.github.com/repos/casey/just/releases/latest \
    | grep "browser_download_url" \
    | grep "${ARCH}-unknown-linux-musl.tar.gz" \
    | cut -d '"' -f 4) && \
    curl -sSL "$JUST_URL" | tar xz -C /usr/local/bin just

COPY Justfile .
# Runs as root inside the container — bootstrap must NOT use sudo
RUN just bootstrap
# Asimi runtime — appended to /etc/bash.bashrc so the command-wrapper protocol
# (__asimi_run) used by the podman runner is sourced by every interactive bash
# shell BEFORE /root/.bashrc, regardless of user customizations.
COPY .agents/sandbox/asimi_runtime.sh /tmp/asimi_runtime.sh
RUN cat /tmp/asimi_runtime.sh >> /etc/bash.bashrc && rm /tmp/asimi_runtime.sh

COPY .agents/sandbox/bashrc /root/.bashrc

# Default command: start shell (bashrc handles startup tasks)
CMD ["/bin/bash"]
