# เครื่องลูกค้าจำลอง: โหลด manager จาก registry แล้วตรวจลายเซ็นก่อนใส่ image
# ไม่ใช้โค้ดของ edgeconnector เลย เส้นทางเดียวกับที่เครื่องลูกค้าจริงใช้ (registry → ตรวจ → รัน)
#
# build จาก root ของ repo: docker build -f machine/Dockerfile .   (หรือ docker compose build)
#
# version ของ manager:
#   ไม่ตั้ง MANAGER_VERSION = อ่านจาก channel (ค่าเริ่มต้น stable) ของ registry → ได้ตัว stable ล่าสุดทุกครั้งที่ build
#   ตั้ง --build-arg MANAGER_VERSION=1.1.0 = ใช้ version นั้น
# manager ใน container อัปเดตตัวเองจาก center ไม่ได้ (manager.update ต้องมี -self-dir ที่ install-manager.sh สร้าง)
# เปลี่ยน version = build image ใหม่: docker compose build --no-cache

# ── ขั้น 1: โหลด + ตรวจ ───────────────────────────────────────
FROM golang:1.27-alpine AS fetch
ARG TARGETARCH
ARG REGISTRY=https://raw.githubusercontent.com/MaiWittawat/edgeconnector-registry/main
ARG CHANNEL=stable
ARG MANAGER_VERSION=
# key ที่เชื่อถือ (public) ต้องตรงกับ key ที่เซ็น manager ใน registry · เปลี่ยนได้ด้วย --build-arg
ARG TRUSTED_KEY=lab-2026=gK3QtGj9wHs/Vm07udIW31CySE3MVP5e/3dUj0aIXV8=

RUN apk add --no-cache curl
COPY machine/verify /src/verify
WORKDIR /src/verify
# verify ใช้แค่ stdlib จึง build เป็น module เดี่ยวในนี้ได้เลย ไม่ต้องดึง edgeconnector (ซึ่งเป็น repo private)
RUN go mod init verify >/dev/null 2>&1 && CGO_ENABLED=0 go build -o /verify .

WORKDIR /dl
RUN set -e; \
    v="$MANAGER_VERSION"; \
    if [ -z "$v" ]; then \
        v=$(curl -fsSL "$REGISTRY/manager/channels/$CHANNEL.json" | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'); \
        [ -n "$v" ] || { echo "no manager version in channel $CHANNEL of $REGISTRY" >&2; exit 1; }; \
        echo "channel $CHANNEL → manager $v"; \
    fi; \
    case "$v" in *[!A-Za-z0-9.+_-]*|.*) echo "bad manager version: $v" >&2; exit 1;; esac; \
    base="$REGISTRY/manager/$v/linux_$TARGETARCH"; \
    curl -fsSL "$base/manifest.json" -o manifest.json; \
    curl -fsSL "$base/manager_${v}_linux_${TARGETARCH}.tar.gz" -o manager.tar.gz; \
    /verify -manifest manifest.json -archive manager.tar.gz -key "$TRUSTED_KEY" \
        -name manager -version "$v" -os linux -arch "$TARGETARCH"; \
    tar -xzf manager.tar.gz bin/manager

# ── ขั้น 2: image ที่ใช้รัน ───────────────────────────────────
FROM alpine:3
RUN apk add --no-cache ca-certificates \
 && adduser -D -u 10001 edge \
 && mkdir -p /data /run/ec \
 && chown edge /data /run/ec
COPY --from=fetch /dl/bin/manager /usr/local/bin/manager
USER edge

# flag ทุกตัวตั้งผ่าน env CC_<NAME> ได้ (ดู compose.yml) · -run-dir สั้น ๆ เพราะ path ของ unix socket ยาวได้จำกัด
ENV CC_DATA=/data CC_RUN_DIR=/run/ec
VOLUME /data
ENTRYPOINT ["manager"]
