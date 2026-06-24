.PHONY: build test run sync web clean

build:
	CGO_ENABLED=0 go build -o bin/plato ./cmd/plato
	CGO_ENABLED=0 go build -o bin/plato-sync ./cmd/plato-sync

test:
	go test ./...

run: build
	./bin/plato -port 8080 -db ./plato.db -wiki-dir ./data

sync: build
	./bin/plato-sync --dir ./docs --wiki demo --db ./plato.db --wiki-dir ./data

web:
	cd web && npm install && npm run build

clean:
	rm -rf bin plato.db data
