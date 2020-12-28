FROM debian:10.7-slim as base

ENV DEBIAN_FRONTEND noninteractive

WORKDIR /app

RUN apt-get update \
 && apt-get install -y \
    ca-certificates \
    graphviz=2.40.1-6 \
 && rm -rf /var/lib/apt/lists/*
