#!/bin/sh

VERSION=$(cat ./VERSION)

cat > version.go <<EOF
package lightstep
var TracerVersionValue = "$VERSION"
EOF
