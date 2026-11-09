.PHONY: ui build test vet docker run-docker app install-app clean

ui:
	cd web && npm ci && npm run build

build: ui
	go build -trimpath -ldflags="-s -w" -o bin/findDevClean ./cmd/findDevClean

test:
	go test ./...

vet:
	go vet ./...

docker:
	docker build -t finddevclean:latest .

run-docker:
	docker run --rm -p 127.0.0.1:7777:7777 \
		--user "$$(id -u):$$(id -g)" \
		-v "$$HOME:/home/nonroot" \
		finddevclean:latest

app:
	./scripts/build-mac-app.sh

install-app:
	./scripts/build-mac-app.sh --install

clean:
	rm -rf bin dist web/dist/* && touch web/dist/.gitkeep
