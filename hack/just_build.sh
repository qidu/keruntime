#!/usr/bin/env bash

KUBEEDGE_ROOT=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/..
echo "building the cloudcore..."
make -C "${KUBEEDGE_ROOT}" WHAT="cloudcore"
echo "building the edgecore..."
make -C "${KUBEEDGE_ROOT}" WHAT="edgecore"
