.PHONY: help test serve-docs build-docs fetch-contributors

help:
	@echo "Available targets:"
	@echo "  test              - Run tests"
	@echo "  help              - Show this help message"

test:
	@echo "Running tests..."
	@go test ./...

serve-docs: ## Serve docs
	@command -v mkdocs >/dev/null 2>&1 || { \
	  echo "mkdocs not found - please install it (pip install mkdocs-material)"; exit 1; } ; \
	mkdocs serve
	
build-docs: ## Build docs
	@command -v mkdocs >/dev/null 2>&1 || { \
	  echo "mkdocs not found - please install it (pip install mkdocs-material)"; exit 1; } ; \
	mkdocs build

fetch-contributors: ## Fetch contributors
	python3 docs/scripts/fetch_contributors.py
