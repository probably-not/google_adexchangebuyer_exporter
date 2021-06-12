GO := go
DOCKER := docker
TAG?=$(shell git rev-parse HEAD)
REGISTRY?=probablynot
IMAGE=google-adexchangebuyer-exporter

all: build

build:
	@echo ">> building using docker"
	@$(DOCKER) build -t ${REGISTRY}/${IMAGE}:${TAG} -f Dockerfile .
	@$(DOCKER) tag ${REGISTRY}/${IMAGE}:${TAG} ${REGISTRY}/${IMAGE}:latest

push:
	docker push ${REGISTRY}/${IMAGE}:${TAG}
	docker push ${REGISTRY}/${IMAGE}:latest

.PHONY: all build push
