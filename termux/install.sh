#!/data/data/com.termux/files/usr/bin/sh
# ルミを Termux に入れる (リポジトリを登録して pkg install lumi)
# Install Lumi on Termux (adds the repository, then pkg install lumi)
#   curl -fsSL https://hotakacchi.github.io/ai-console/termux/install.sh | sh
set -e
REPO=https://hotakacchi.github.io/ai-console/termux
mkdir -p "$PREFIX/etc/apt/sources.list.d" "$PREFIX/etc/apt/trusted.gpg.d"
curl -fsSL "$REPO/lumi.asc" -o "$PREFIX/etc/apt/trusted.gpg.d/lumi.asc"
echo "deb $REPO stable main" > "$PREFIX/etc/apt/sources.list.d/lumi.list"
pkg update -y
pkg install -y lumi
echo
echo "lumi で起動します / Run: lumi"
