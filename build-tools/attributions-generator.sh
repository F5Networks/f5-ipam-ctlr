#!/bin/bash

#
# Run the attributions generator container
#

set -e
set -x

ATTR_GEN_IMG=f5devcentral/attributions-generator:latest

RUN_ARGS=( \
  --rm
  -v $PWD:$PWD
  -e LOCAL_USER_ID=$(id -u)
)

docker run "${RUN_ARGS[@]}" ${ATTR_GEN_IMG}  "$@"
