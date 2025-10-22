SERVICE_NAME=service-a
DOCKER_IMAGE=myorg/$(SERVICE_NAME)

.PHONY: build run docker

build:
	go build -o kontakte main.go

run: build
	./kontakte

docker: build
	docker build -t $(DOCKER_IMAGE) .
