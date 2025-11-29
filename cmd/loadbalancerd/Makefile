SERVICE_NAME=loadbalancerd
DOCKER_IMAGE=ghcr.io/teapotovh/$(SERVICE_NAME)

.PHONY: build run docker

build:
	go build -o $(SERVICE_NAME)

run: build
	./$(SERVICE_NAME)

docker:
	docker build -t $(DOCKER_IMAGE) --build-context root=$(ROOT) .
