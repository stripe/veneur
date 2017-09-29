#!/bin/bash
set -ex

CIRCLEUTIL_TAG="v1.39"
DEFAULT_GOLANG_VERSION="1.6"
GO_TESTED_VERSIONS="1.4.3 1.5.1 1.6"
IMPORT_PATH="github.com/$CIRCLE_PROJECT_USERNAME/$CIRCLE_PROJECT_REPONAME"

export GOROOT="$HOME/go_circle"
export GOPATH="$HOME/.go_circle"
export GOPATH_INTO="$HOME/lints"
export DOCKER_STORAGE="$HOME/docker_images"
PATH="$GOROOT/bin:$GOPATH/bin:$GOPATH_INTO:$PATH"

GO_COMPILER_PATH="$HOME/gover"
SRC_PATH="$GOPATH/src/$IMPORT_PATH"

# Cache phase of circleci
function do_cache() {
  [ ! -d "$HOME/circleutil" ] && git clone https://github.com/signalfx/circleutil.git "$HOME/circleutil"
  (
    cd "$HOME/circleutil"
    git fetch -a -v
    git fetch --tags
    git reset --hard $CIRCLEUTIL_TAG
  )
  . "$HOME/circleutil/scripts/common.sh"
  mkdir -p "$GO_COMPILER_PATH"
  install_all_go_versions "$GO_COMPILER_PATH"
  install_go_version "$GO_COMPILER_PATH" "$DEFAULT_GOLANG_VERSION"
  mkdir -p "$GOPATH_INTO"
  install_circletasker "$GOPATH_INTO"
  versioned_goget "github.com/signalfx/gobuild:v1.6"
  copy_local_to_path "$SRC_PATH"
}


export IDX=0
function gobuildit() {
  IDX=$((IDX+1))
  env GOCONVEY_REPORTER=silent GOLIB_LOG=/dev/null gobuild -verbose -filename_prefix "$IDX" -verbosefile "$CIRCLE_ARTIFACTS/${IDX}gobuildout.txt" check "$1"
  RET="$?"
  return $RET
}

# Test phase of circleci
function do_test() {
  . "$HOME/circleutil/scripts/common.sh"
  # The go get install only works on go 1.5+ (no -f parameter)
  install_go_version "$GO_COMPILER_PATH" "$DEFAULT_GOLANG_VERSION"
  (
    cd "$SRC_PATH"
    gobuild -verbose install
  )

  for GO_VERSION in $GO_TESTED_VERSIONS; do
    install_go_version "$GO_COMPILER_PATH" "$GO_VERSION"
    rm -rf "$GOPATH/pkg"
    go env
    go version
    go tool cover || go get golang.org/x/tools/cmd/cover
    (
      cd "$SRC_PATH"
      mkdir -p "$CIRCLE_ARTIFACTS/$GO_VERSION"
      go clean -x ./...
      go get -d -v -t ./...
      # Test all versions of Go, lint checks only work in go1.6
      if [ "$CIRCLE_NODE_INDEX" == "0" ]; then
        go test -timeout 15s -race ./...
      fi
    )
  done
  (
      install_go_version "$GO_COMPILER_PATH" "$DEFAULT_GOLANG_VERSION"
      cd "$SRC_PATH"
      go clean -x ./...
      go get -d -v -t ./...
      # gobuild lint checks only work in go 1.6
      if [ "$CIRCLE_NODE_INDEX" == "0" ]; then
        gobuild list | circletasker serve &
      fi
      circletasker_execute gobuildit 
  )
}

# Deploy phase of circleci
function do_deploy() {
  echo "No deploy phase for library"
}

function do_all() {
  do_cache
  do_test
  do_deploy
}

case "$1" in
  cache)
    do_cache
    ;;
  test)
    do_test
    ;;
  deploy)
    do_deploy
    ;;
  all)
    do_all
    ;;
  *)
  echo "Usage: $0 {cache|test|deploy|all}"
    exit 1
    ;;
esac
