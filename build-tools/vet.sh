#!/bin/bash

set -x
set -e

go tool vet -all -shadow ./pkg ./test
go tool vet -all -shadow main.go

exit $?
