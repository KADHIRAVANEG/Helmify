BINARY := helmify
PKG := ./cmd/helmify
OUT := bin/$(BINARY)

.PHONY: build run test lint fmt vet clean install examples

build:
	go build -o $(OUT) $(PKG)

run: build
	$(OUT)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

lint: vet
	@echo "go vet passed. Run 'make examples' to generate + helm-lint the example charts."

clean:
	rm -rf bin/ examples/*/output

# Regenerate every example chart and lint each one - useful as a quick
# end-to-end smoke test after changing generator/parser code.
examples: build
	rm -rf examples/yaml-input/output examples/compose-input/output examples/dockerfile-input/output
	$(OUT) generate --input examples/yaml-input/manifests --output examples/yaml-input/output --name webapp --secure --lint
	$(OUT) generate --input examples/compose-input/docker-compose.yml --output examples/compose-input/output --secure --lint
	$(OUT) generate --input examples/dockerfile-input/Dockerfile --output examples/dockerfile-input/output --secure --lint

install: build
	cp $(OUT) $(GOPATH)/bin/$(BINARY) 2>/dev/null || cp $(OUT) $(HOME)/go/bin/$(BINARY)
	@echo "Installed to $$(command -v $(BINARY) || echo '$$GOPATH/bin - make sure it is on your PATH')"
