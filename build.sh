#!/bin/sh

name="pant"
version=$1

if [ "$1" = "" ]; then
  version=v5.0.0
fi

output="out"

Build() {
  goarm=$4
  if [ "$4" = "" ]; then
    goarm=7
  fi

  echo "Building $1..."
  export GOOS=$2 GOARCH=$3 GO386=sse2 CGO_ENABLED=0 GOARM=$goarm
  if [ "$2" = "windows" ]; then
    if command -v goversioninfo >/dev/null 2>&1; then
      goversioninfo -platform-specific
    fi
    go build -trimpath -ldflags "-X main.Version=$version -s -w" -o "$output/$1/$name.exe"
  else
    go build -trimpath -ldflags "-X main.Version=$version -s -w" -o "$output/$1/$name"
  fi

  Pack "$1"
}

# 归档打包
Pack() {
  cp README.md LICENSE NOTICE "$output/$1"

  cd "$output" || exit 1
  if [ "$GOOS" = "linux" ]; then
    tar -czf "$1.tar.gz" "$1"
  else
    zip -q -r "$1.zip" "$1"
  fi

  # 删除
  rm -rf "$1"

  cd ..
}

# macOS
Build "$name-$version-darwin-osx-amd64" darwin amd64
Build "$name-$version-darwin-osx-arm64" darwin arm64

# Windows
Build "$name-$version-windows-x86" windows 386
Build "$name-$version-windows-x64" windows amd64
Build "$name-$version-windows-arm64" windows arm64

# Linux
Build "$name-$version-linux-386" linux 386
Build "$name-$version-linux-amd64" linux amd64
Build "$name-$version-linux-arm" linux arm
Build "$name-$version-linux-arm64" linux arm64
GOMIPS=softfloat Build "$name-$version-linux-mips" linux mips
Build "$name-$version-linux-mips64" linux mips64
GOMIPS=softfloat Build "$name-$version-linux-mipsle" linux mipsle
Build "$name-$version-linux-mips64le" linux mips64le

# FreeBSD
Build "$name-$version-freebsd-386" freebsd 386
Build "$name-$version-freebsd-amd64" freebsd amd64
