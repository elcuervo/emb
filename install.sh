#!/bin/sh
set -eu

REPO="elcuervo/emb"
INSTALL_DIR="${EMB_INSTALL_DIR:-/usr/local/bin}"

case "$(uname -s)-$(uname -m)" in
  Darwin-arm64) TARGET="darwin_arm64"  ;;
  Linux-x86_64|Linux-amd64)  TARGET="linux_amd64"   ;;
  Linux-aarch64|Linux-arm64) TARGET="linux_arm64"   ;;
  *)
    echo "error: unsupported platform $(uname -s)-$(uname -m)"
    echo "supported: darwin/arm64, linux/amd64, linux/arm64"
    exit 1
    ;;
esac

echo "looking up latest release..."
VERSION=$(curl -sL "https://api.github.com/repos/$REPO/releases/latest" \
  | grep '"tag_name":' | sed 's/.*"tag_name": "//;s/".*//')
VNUM=$(echo "$VERSION" | sed 's/^v//')

TARBALL="emb_${VNUM}_${TARGET}.tar.gz"
URL="https://github.com/$REPO/releases/download/$VERSION/$TARBALL"

echo "downloading emb $VERSION ($TARGET)..."
curl -fsSL "$URL" | tar xz -C "$INSTALL_DIR"

# Report what the tarball actually carried: the latest release may predate a
# binary, and a fixed list would claim one that was never downloaded.
placed=""
for bin in emb emb-top emb-stress; do
  if [ -x "$INSTALL_DIR/$bin" ]; then
    placed="$placed $bin"
  fi
done

echo ""
echo "emb $VERSION installed to $INSTALL_DIR:$placed"
echo ""
echo "Quick start:"
echo "  emb -model-repo Xenova/all-MiniLM-L6-v2"
echo "  redis-cli EMB model \"hello world\""
