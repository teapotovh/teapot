SERVICE_NAME=kontakted
DOCKER_IMAGE=github.com/teapotovh/$(SERVICE_NAME)

.PHONY: build run docker

build:
	go build -o $(SERVICE_NAME)

run: build
	./$(SERVICE_NAME)

docker: build
	docker build -t $(DOCKER_IMAGE) . --build-arg root=$(ROOT)
