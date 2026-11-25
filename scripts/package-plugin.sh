#!/bin/sh
# package-plugin.sh = build plugin แล้วเซ็นลง registry ในเครื่อง (./data/registry) ด้วย key ทิ้ง (./data/keys/sandbox.key) · ใช้กับแบบ 3
#
#   scripts/package-plugin.sh <package> <name> <version>
#
#   <package>  path ของโฟลเดอร์ main ของ plugin (build ใน module ของโฟลเดอร์นั้น) หรือ import path ที่ build ได้จาก repo นี้
#   เช่น scripts/package-plugin.sh github.com/MaiWittawat/edgeconnector/examples/hello-plugin hello 0.1.0
#        scripts/package-plugin.sh ../my-plugin my-plugin 0.1.0
#
# build เป็น linux/<arch ของ docker> (ตั้ง ARCH=amd64 เพื่อเปลี่ยน) · key สร้างครั้งแรกครั้งเดียว ใช้ซ้ำทุกครั้ง
# version ที่มีอยู่แล้วถูกปฏิเสธ: ออก version ใหม่ หรือลบ data/registry/<name>/<version> แล้วรันใหม่ (registry นี้ไม่ได้ push ไปไหน)
# key นี้เป็นของทิ้งสำหรับลองในเครื่อง ห้ามใช้เซ็นของจริง · data/ ไม่ถูก commit
set -eu

if [ $# -ne 3 ]; then
	sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//' >&2
	exit 2
fi
pkg=$1 name=$2 version=$3

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
arch=${ARCH:-$(docker version --format '{{.Server.Arch}}' 2>/dev/null || go env GOARCH)}
cc() { go run github.com/MaiWittawat/edgeconnector/cmd/cc-release "$@"; }

mkdir -p data/keys data/registry
if [ ! -f data/keys/sandbox.key ]; then
	cc keygen -dir data/keys -id sandbox >/dev/null
	echo "created throwaway key data/keys/sandbox.key"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "build $name $version linux/$arch"
if [ -d "$pkg" ]; then
	# โฟลเดอร์: build ใน module ของ plugin เอง (repo อื่นก็ได้)
	(cd "$pkg" && CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath \
		-ldflags "-s -w -X main.version=$version" -o "$tmp/$name" .)
else
	CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath \
		-ldflags "-s -w -X main.version=$version" -o "$tmp/$name" "$pkg"
fi

cc package -dir data/registry -name "$name" -version "$version" -os linux -arch "$arch" \
	-binary "$tmp/$name" -key data/keys/sandbox.key

echo
echo "put this line in .env (once), then: docker compose --profile local up --build machine-local"
echo "LOCAL_TRUSTED_KEYS=sandbox=$(cat data/keys/sandbox.pub)"
