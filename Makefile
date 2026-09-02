# VLPM — compilation et distribution multi-plateformes.
#
# « make » compile pour la machine courante.
# « make dist » produit les exécutables des trois systèmes dans dist/.

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/assopmf/vlpm/internal/api.Version=$(VERSION)
# CGO_ENABLED=0 : pilote SQLite en Go pur, donc binaire statique et
# cross-compilation sans chaîne d'outils C.
GO := CGO_ENABLED=0 go

.PHONY: aide
aide:
	@echo "Cibles disponibles :"
	@echo "  make lancer      Compile et démarre le serveur sur le port 8080"
	@echo "  make dev         Démarre avec l'interface rechargée depuis le disque"
	@echo "  make test        Lance les tests"
	@echo "  make verifier    Format, vet et tests"
	@echo "  make build       Compile pour la machine courante"
	@echo "  make dist        Compile pour Linux, macOS et Windows dans dist/"
	@echo "  make propre      Supprime les fichiers produits"

.PHONY: build
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o vlpm ./cmd/vlpm

.PHONY: lancer
lancer: build
	./vlpm

.PHONY: dev
dev:
	$(GO) run ./cmd/vlpm --dev --data ./data

.PHONY: test
test:
	go test ./...

.PHONY: verifier
verifier:
	gofmt -l . | tee /dev/stderr | (! read)
	go vet ./...
	go test ./...

# Plateformes couvertes. amd64 et arm64 pour Linux (VPS x86 et Raspberry Pi),
# arm64 et amd64 pour macOS (Apple Silicon et Intel), amd64 pour Windows.
PLATEFORMES := linux/amd64 linux/arm64 linux/arm darwin/arm64 darwin/amd64 windows/amd64

.PHONY: dist
dist: propre-dist
	@mkdir -p dist
	@for p in $(PLATEFORMES); do \
		os=$${p%/*}; arch=$${p#*/}; \
		nom=vlpm-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then nom=$$nom.exe; fi; \
		echo "  compilation $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags "$(LDFLAGS)" \
			-o dist/$$nom ./cmd/vlpm || exit 1; \
	done
	@cd dist && shasum -a 256 * > SHA256SUMS 2>/dev/null || sha256sum * > SHA256SUMS
	@echo ""
	@echo "Exécutables produits dans dist/ (version $(VERSION)) :"
	@ls -lh dist/ | tail -n +2 | awk '{printf "  %-28s %s\n", $$9, $$5}'

.PHONY: propre-dist
propre-dist:
	@rm -rf dist

.PHONY: propre
propre: propre-dist
	rm -f vlpm vlpm.exe
