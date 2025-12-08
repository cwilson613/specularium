BINARY := specularium
IMAGE := cwilson613/specularium
GO := /usr/local/go/bin/go

build:
	CGO_ENABLED=1 $(GO) build -o $(BINARY) ./cmd/server

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
