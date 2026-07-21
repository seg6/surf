FROM debian:bookworm-slim

ARG THEOS_REPO=https://github.com/theos/theos.git
ARG THEOS_REF=master

ENV DEBIAN_FRONTEND=noninteractive \
    THEOS=/opt/theos \
    PATH=/opt/theos/toolchain/linux/iphone/bin:/opt/theos/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

RUN apt-get update && apt-get install -y --no-install-recommends \
      bash ca-certificates curl dpkg-dev fakeroot file git libtinfo6 libxml2 \
      make perl rsync sudo xz-utils zip \
    && rm -rf /var/lib/apt/lists/*

RUN git clone --recursive "$THEOS_REPO" "$THEOS" \
    && cd "$THEOS" \
    && git checkout "$THEOS_REF" \
    && git submodule update --init --recursive

RUN set -eux; \
    arch="$(uname -m)"; \
    case "$arch" in \
      aarch64|x86_64) toolchain_arch="$arch" ;; \
      *) echo "unsupported build arch: $arch" >&2; exit 1 ;; \
    esac; \
    mkdir -p "$THEOS/toolchain"; \
    curl -fsSL "https://github.com/L1ghtmann/llvm-project/releases/latest/download/iOSToolchain-${toolchain_arch}.tar.xz" \
      | tar -xJ -C "$THEOS/toolchain"; \
    test -x "$THEOS/toolchain/linux/iphone/bin/clang"

COPY build.sh /usr/local/bin/surf-build
RUN chmod +x /usr/local/bin/surf-build

WORKDIR /src
ENTRYPOINT ["surf-build"]
CMD ["make", "-C", "/src/native/client", "package"]
