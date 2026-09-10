GO := /Users/tal/sdk/go1.26.3/bin/go

.PHONY: test test-unit

# 全量测试（约 4s）
test:
	$(GO) test ./experiments/ -v -timeout 300s

# 跳过慢用例（含 sleep / 大内存分配 / 子进程崩溃验证，约 1s）
test-unit:
	$(GO) test ./experiments/ -short -v -timeout 60s
