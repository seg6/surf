FROM debian:bookworm-slim

ARG THEOS_REPO=https://github.com/theos/theos.git
ARG THEOS_REF=16362d3aa83a0acd56df4493d575d34306d42478
ARG TOOLCHAIN_TAG=test-210562a
ARG TOOLCHAIN_SHA256_X86_64=a72a7a577e2fbe2838b6b5e9c72034fa7d114af96f0e1d4b016f18730ce4056e
ARG TOOLCHAIN_SHA256_AARCH64=9051e9d531de5e511f0fab40d925d625bc6efe7ed9dce821d179c09de9bc690b
ARG LIBPLIST_REF=cb38f7271ac301abc9f3a55a75c2f337245a4030

ENV DEBIAN_FRONTEND=noninteractive \
    THEOS=/opt/theos \
    PATH=/opt/theos/toolchain/linux/iphone/bin:/opt/theos/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

RUN apt-get update && apt-get install -y --no-install-recommends \
      autoconf automake bash ca-certificates curl dpkg-dev fakeroot file g++ git \
      libtinfo6 libtool libxml2 make perl pkg-config rsync sudo xz-utils zip \
    && rm -rf /var/lib/apt/lists/*

# The pinned iOS toolchain's plistutil is linked against libplist ABI 4,
# newer than Debian bookworm's ABI 3 package. Build the matching upstream
# release so clean package builds work instead of depending on a host cache.
RUN git clone https://github.com/libimobiledevice/libplist.git /tmp/libplist \
    && cd /tmp/libplist \
    && git checkout --detach "$LIBPLIST_REF" \
    && ./autogen.sh --without-cython \
    && make -j"$(nproc)" \
    && make install \
    && ldconfig \
    && rm -rf /tmp/libplist

RUN git clone "$THEOS_REPO" "$THEOS" \
    && cd "$THEOS" \
    && git checkout --detach "$THEOS_REF" \
    && git submodule update --init --recursive

RUN set -eux; \
    arch="$(uname -m)"; \
    case "$arch" in \
      aarch64) toolchain_arch="$arch"; toolchain_sha="$TOOLCHAIN_SHA256_AARCH64" ;; \
      x86_64) toolchain_arch="$arch"; toolchain_sha="$TOOLCHAIN_SHA256_X86_64" ;; \
      *) echo "unsupported build arch: $arch" >&2; exit 1 ;; \
    esac; \
    mkdir -p "$THEOS/toolchain"; \
    toolchain="/tmp/iOSToolchain-${toolchain_arch}.tar.xz"; \
    curl -fsSL "https://github.com/L1ghtmann/llvm-project/releases/download/${TOOLCHAIN_TAG}/iOSToolchain-${toolchain_arch}.tar.xz" -o "$toolchain"; \
    printf '%s  %s\n' "$toolchain_sha" "$toolchain" | sha256sum -c -; \
    tar -xJf "$toolchain" -C "$THEOS/toolchain"; \
    rm -f "$toolchain"; \
    test -x "$THEOS/toolchain/linux/iphone/bin/clang"

COPY build.sh /usr/local/bin/surf-build
RUN chmod +x /usr/local/bin/surf-build

WORKDIR /src
ENTRYPOINT ["surf-build"]
CMD ["make", "-C", "/src/native/client", "package"]
