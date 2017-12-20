#!/bin/bash
go test -tags=integration -run=IT -v -timeout 15s ./... "$@"
