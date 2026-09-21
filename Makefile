APP     := sshpfm
BIN     := build/bin
APP_DIR := $(BIN)/$(APP).app
VERSION := $(shell sed -n 's/.*"productVersion": "\(.*\)".*/\1/p' wails.json)

.PHONY: help dev build build-mac build-mac-arm64 build-mac-amd64 \
        build-windows build-linux dmg appicon test check clean

help:
	@echo "$(APP) $(VERSION)"
	@echo "  make dev            live development build (Vite + hot reload)"
	@echo "  make build          macOS universal .app, ad-hoc signed  [default]"
	@echo "  make build-mac-arm64 / build-mac-amd64"
	@echo "  make build-windows  windows/amd64 .exe + NSIS installer"
	@echo "  make build-linux    linux/amd64 binary"
	@echo "  make dmg            package the macOS .app as a .dmg"
	@echo "  make appicon        regenerate build/appicon.png from brand/"
	@echo "  make check          go vet + go test + frontend type-check"
	@echo "  make clean          remove build output and frontend/dist"

dev:
	wails dev

build: build-mac

# Universal so the same bundle runs on Apple Silicon and Intel.
build-mac:
	wails build -platform darwin/universal -clean
	$(MAKE) sign-mac

build-mac-arm64:
	wails build -platform darwin/arm64 -clean
	$(MAKE) sign-mac

build-mac-amd64:
	wails build -platform darwin/amd64 -clean
	$(MAKE) sign-mac

# Ad-hoc signature. It is not a Developer ID and will not pass Gatekeeper
# on another machine, but it gives the bundle a stable identity so macOS
# keeps its keychain and network grants across rebuilds.
.PHONY: sign-mac
sign-mac:
	@codesign --force --deep --sign - $(APP_DIR)
	@codesign --verify --verbose=1 $(APP_DIR)
	@echo "built $(APP_DIR)"

build-windows:
	wails build -platform windows/amd64 -clean -nsis

build-linux:
	wails build -platform linux/amd64 -clean -tags webkit2_41

dmg: build-mac
	@rm -f $(BIN)/$(APP)-$(VERSION).dmg
	@rm -rf $(BIN)/dmg-staging
	@mkdir -p $(BIN)/dmg-staging
	@cp -R $(APP_DIR) $(BIN)/dmg-staging/
	@ln -s /Applications $(BIN)/dmg-staging/Applications
	hdiutil create -volname "$(APP)" -srcfolder $(BIN)/dmg-staging \
		-ov -format UDZO $(BIN)/$(APP)-$(VERSION).dmg
	@rm -rf $(BIN)/dmg-staging
	@echo "built $(BIN)/$(APP)-$(VERSION).dmg"

appicon:
	go run ./tools/appicon brand/logo-icon.png build/appicon.png

check:
	go vet ./...
	go test ./...
	cd frontend && npm run build

clean:
	rm -rf $(BIN) frontend/dist
