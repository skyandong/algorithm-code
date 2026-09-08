GO := /Users/tal/sdk/go1.26.3/bin/go

.PHONY: test test-unit run-all

# 全量测试：纯逻辑用例 + 全部实验的 demo 冒烟（约 12s）
test:
	$(GO) test ./experiments/ -v -timeout 300s

# 只跑纯逻辑用例（-short 跳过 demo 冒烟，约 1s）
test-unit:
	$(GO) test ./experiments/ -short -v -timeout 60s

# 跑全部实验（等价 go run ./experiments/ all）
run-all:
	$(GO) run ./experiments/ all
