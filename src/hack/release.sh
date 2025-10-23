#!/bin/sh

set -o errexit
set -o nounset

readonly BIN_PATH=bin

package() {
  last_dir=$(pwd)
  cd $BIN_PATH
  file=$1
  type=$2
  case $type in
  zip)
    res=${file%.exe}.zip
    zip $res ${file} -j ../config.yml >/dev/null 2>&1
    ;;
  tar)
    res=${file}.tar.gz
    tar zcvf $res ${file} -C ../ config.yml >/dev/null 2>&1
    ;;
  7z)
    res=${file}.7z
    7z a $res ${file} ../config.yml >/dev/null 2>&1
    ;;
  *) ;;

  esac
  cd "$last_dir"
  echo $BIN_PATH/$res
}

# Build all targets in parallel on a single runner to speed up CI
# Determine concurrency (fallback to 2 if detection fails)
JOBS=${JOBS:-$(getconf _NPROCESSORS_ONLN 2>/dev/null || nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 2)}

echo "[deps] Warming Go module cache..."
go mod download

# Generate target list and filter unsupported ones
TARGETS=$(go tool dist list | awk '!/^(linux\/loong64|android\/|ios\/|js\/wasm)/')

# 说明：此前同时使用了 -n1/--max-args 与 -I/--replace，会触发 xargs 互斥参数 warning。
# 这里去掉 -I，占位通过 sh -c 的参数传递完成，保留 -n1 和 -P 以保持一条记录一次执行且并行。
printf "%s\n" "$TARGETS" | xargs -n1 -P "$JOBS" sh -c '
  dist="$1"
  case "$dist" in
    linux/loong64|android/*|ios/*|js/wasm)
      exit 0 ;; # filtered above, keep safe-guard
    *) ;;
  esac
  platform="${dist%/*}"
  arch="${dist#*/}"
  echo "[build] PLATFORM=$platform ARCH=$arch"
  make PLATFORM="$platform" ARCH="$arch" bililive
' _

for file in $(ls $BIN_PATH); do
  case $file in
  *.tar.gz | *.zip | *.7z | *.yml | *.yaml)
    continue
    ;;
  *windows*)
    package_type=zip
    ;;
  *)
    package_type=tar
    ;;
  esac
  res=$(package $file $package_type)
  rm -f $BIN_PATH/$file
done
