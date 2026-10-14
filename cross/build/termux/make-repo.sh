#!/usr/bin/env bash
# Termux 用の apt リポジトリを作る: make-repo.sh <.deb のフォルダ> <リポジトリのフォルダ>
# 環境変数 GPG_KEY (秘密鍵) があれば署名する (InRelease / Release.gpg)
set -euo pipefail
debs=$1
repo=$2
rm -rf "$repo/pool" "$repo/dists"
mkdir -p "$repo/pool/main" "$repo/dists/stable/main/binary-aarch64"
cp "$debs"/*.deb "$repo/pool/main/"
cd "$repo"
dpkg-scanpackages --arch aarch64 pool/main /dev/null > dists/stable/main/binary-aarch64/Packages
gzip -9kf dists/stable/main/binary-aarch64/Packages
apt-ftparchive \
  -o APT::FTPArchive::Release::Origin=Lumi -o APT::FTPArchive::Release::Label=Lumi \
  -o APT::FTPArchive::Release::Suite=stable -o APT::FTPArchive::Release::Codename=stable \
  -o APT::FTPArchive::Release::Architectures=aarch64 -o APT::FTPArchive::Release::Components=main \
  release dists/stable > dists/stable/Release
if [ -n "${GPG_KEY:-}" ]; then
  export GNUPGHOME=$(mktemp -d)
  echo "$GPG_KEY" | gpg --batch --import
  gpg --batch --yes --clearsign -o dists/stable/InRelease dists/stable/Release
  gpg --batch --yes --armor --detach-sign -o dists/stable/Release.gpg dists/stable/Release
  gpg --batch --verify dists/stable/InRelease
  rm -rf "$GNUPGHOME"
fi
cat dists/stable/main/binary-aarch64/Packages
