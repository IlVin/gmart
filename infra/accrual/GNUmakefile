# Переменные проекта
WORKDIR=.
BINARY_NAME=${WORKDIR}/accrual_linux_amd64
DOCKER_IMG=accrual:latest

build:
	@echo "Building scratch Docker image..."
	docker build -f Dockerfile -t $(DOCKER_IMG) .
