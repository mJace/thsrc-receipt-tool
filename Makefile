BINARY_NAME=thsrc-decrypt
OUT_DIR=bin

.PHONY: all build clean linux-x86 mac-arm help

all: build

build: linux-x86 mac-arm ## 編譯所有平台執行檔

linux-x86: ## 編譯 Linux x86_64 版本
	@echo "正在編譯 Linux x86_64..."
	GOOS=linux GOARCH=amd64 go build -o $(OUT_DIR)/$(BINARY_NAME)-linux-x86_64 main.go

mac-arm: ## 編譯 macOS ARM (M1/M2/M3) 版本
	@echo "正在編譯 macOS ARM64..."
	GOOS=darwin GOARCH=arm64 go build -o $(OUT_DIR)/$(BINARY_NAME)-darwin-arm64 main.go

clean: ## 清理編譯產出的檔案
	@echo "清理 bin 目錄..."
	rm -rf $(OUT_DIR)

help: ## 顯示幫助訊息
	@echo "使用方法:"
	@echo "  make [target]"
	@echo ""
	@echo "可用指令:"
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

# 預設執行指令
.DEFAULT_GOAL := help