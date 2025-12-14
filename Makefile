BINARY := specularium
IMAGE := cwilson613/specularium
GO := /usr/local/go/bin/go

build:
	CGO_ENABLED=0 $(GO) build -o $(BINARY) ./cmd/server

run: build
	./$(BINARY) -addr :3000

test:
	$(GO) test -v ./...

tidy:
	$(GO) mod tidy

clean:
	rm -f $(BINARY) *.db*

docker:
	docker build -t $(IMAGE) .

docker-push: docker
	docker push $(IMAGE)

push: docker-push

# Cross-compilation targets (IoT/edge deployments)
build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -o $(BINARY)-arm64 ./cmd/server

build-armv7:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 $(GO) build -o $(BINARY)-armv7 ./cmd/server

build-all: build build-arm64 build-armv7
	@echo "Built: $(BINARY) $(BINARY)-arm64 $(BINARY)-armv7"
	@file $(BINARY) $(BINARY)-arm64 $(BINARY)-armv7
