GO := go
DOCKER := docker
TAG?=$(shell git rev-parse HEAD)
REGISTRY?=probablynot
IMAGE=google-adexchangebuyer-exporter

all: build

build:
	@echo ">> building using docker"
	@$(DOCKER) build -t ${REGISTRY}/${IMAGE}:${TAG} -f Dockerfile .

push:
	docker push ${REGISTRY}/${IMAGE}:${TAG}

.PHONY: all build push
