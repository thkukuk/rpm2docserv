RPM2DOCSERV_BIN := bin/rpm2docserv
AUXSERV_BIN := bin/docserv-auxserv

GO ?= go
GO_MD2MAN ?= go-md2man

VERSION := HEAD
USE_VENDOR =
LOCAL_LDFLAGS = -buildmode=pie -ldflags "-X main.rpm2docservVersion=$(VERSION)"

.PHONY: all api build vendor
all: dep bundle build

dep: ## Get the dependencies
	@$(GO) get -v -d ./...

update: ## Get and update the dependencies
	@$(GO) get -v -d -u ./...

tidy: ## Clean up dependencies
	@$(GO) mod tidy

vendor: dep ## Create vendor directory
	@$(GO) mod vendor

bundle: ## Generate embedded files
	$(GO) generate bundle.go

build: ## Build the binary files
	$(GO) build -v -o bin/ $(USE_VENDOR) $(LOCAL_LDFLAGS) ./cmd/...

clean: ## Remove previous builds
	@rm -f $(RPM2DOCSERV_BIN)

help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'


.PHONY: release
release: ## create release package from git
	git clone https://github.com/thkukuk/rpm2docserv
	mv rpm2docserv rpm2docserv-$(VERSION)
	sed -i -e 's|USE_VENDOR =|USE_VENDOR = -mod vendor|g' rpm2docserv-$(VERSION)/Makefile
	make -C rpm2docserv-$(VERSION) vendor
	#cp VERSION rpm2docserv-$(VERSION)
	tar --exclude .git -cJf rpm2docserv-$(VERSION).tar.xz rpm2docserv-$(VERSION)
	rm -rf rpm2docserv-$(VERSION)
