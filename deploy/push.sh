#!/bin/sh

set -e

BASEDIR=$(dirname "$0")
OFISU_CONTAINER_BASE="us.gcr.io/kizuna-188702/github.com/austinpray/ofisu"
OFISU_CONTAINER_TAG="$(git branch --show-current)-$(git rev-parse HEAD)"
OFISU_CONTAINER_NAME="$OFISU_CONTAINER_BASE:$OFISU_CONTAINER_TAG"
export OFISU_CONTAINER_NAME
echo "BUILDING $OFISU_CONTAINER_NAME"
docker-compose build

cd "$BASEDIR"
kustomize edit set image "$OFISU_CONTAINER_NAME"

docker push "$OFISU_CONTAINER_NAME"
