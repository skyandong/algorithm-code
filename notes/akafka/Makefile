GO := /Users/tal/sdk/go1.26.3/bin/go

.PHONY: up down run-01 run-02 run-03 run-04 run-05 run-06 run-07 run-08 run-09 run-10 run-11 topics test test-unit

# 启动 Kafka + Kafka UI
up:
	docker compose -f docker/docker-compose.yml up -d
	@echo "等待 Kafka 就绪..."
	@sleep 8
	@echo "Kafka UI: http://localhost:8080"

# 停止
down:
	docker compose -f docker/docker-compose.yml down -v

# 手动创建 topic（auto.create 已开启，这里是显式创建带分区数）
topics:
	docker exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
		--create --if-not-exists --topic demo-basic --partitions 3 --replication-factor 1
	docker exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
		--create --if-not-exists --topic orders --partitions 3 --replication-factor 1
	docker exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
		--create --if-not-exists --topic payment-requests --partitions 3 --replication-factor 1
	docker exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
		--create --if-not-exists --topic notifications --partitions 3 --replication-factor 1
	docker exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
		--create --if-not-exists --topic orders-dlq --partitions 1 --replication-factor 1
	docker exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
		--create --if-not-exists --topic account-debit --partitions 1 --replication-factor 1
	docker exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
		--create --if-not-exists --topic account-credit --partitions 1 --replication-factor 1
	docker exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
		--create --if-not-exists --topic account-audit --partitions 1 --replication-factor 1
	docker exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
		--create --if-not-exists --topic order-events --partitions 3 --replication-factor 1
	docker exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
		--create --if-not-exists --topic storage-demo --partitions 3 --replication-factor 1
	docker exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
		--create --if-not-exists --topic concurrent-demo --partitions 1 --replication-factor 1
	docker exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
		--create --if-not-exists --topic skew-uniform --partitions 6 --replication-factor 1
	docker exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
		--create --if-not-exists --topic skew-hot --partitions 6 --replication-factor 1
	@echo "Topic 列表："
	@docker exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list

# 列出所有 topic
list-topics:
	docker exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list

# 查看消费者组
list-groups:
	docker exec kafka /opt/kafka/bin/kafka-consumer-groups.sh --bootstrap-server localhost:9092 --list

# 查看某个组的 lag（用法：make lag GROUP=demo-group-1）
lag:
	docker exec kafka /opt/kafka/bin/kafka-consumer-groups.sh --bootstrap-server localhost:9092 \
		--describe --group $(GROUP)

# 全部单测（需要 Kafka：make up + make topics；没起 Kafka 时集成用例自动跳过）
test:
	$(GO) test ./experiments/ -v -timeout 300s

# 只跑纯逻辑用例（-short 会跳过所有 requireKafka 的集成用例）
test-unit:
	$(GO) test ./experiments/ -short -v -timeout 60s

run-01:
	@echo "=== 运行案例1：生产者基础 ==="
	$(GO) run ./experiments/ basic

run-02:
	@echo "=== 运行案例2：消费者组（Ctrl+C 退出）==="
	$(GO) run ./experiments/ group

run-03:
	@echo "=== 运行案例3：Exactly-Once ==="
	$(GO) run ./experiments/ eos

run-04:
	@echo "=== 运行案例4：死信队列（Ctrl+C 退出）==="
	$(GO) run ./experiments/ dlq

run-05:
	@echo "=== 运行案例5：消费积压监控（Ctrl+C 退出）==="
	$(GO) run ./experiments/ lag

run-06:
	@echo "=== 运行案例6：多 Topic 流水线（Ctrl+C 退出）==="
	$(GO) run ./experiments/ pipeline

run-07:
	@echo "=== 运行案例7：顺序性保证 ==="
	$(GO) run ./experiments/ ordering

run-08:
	@echo "=== 运行案例8：存储与读路径 ==="
	$(GO) run ./experiments/ storage

run-09:
	@echo "=== 运行案例9：位移提交竞态 vs 滑动窗口 ==="
	$(GO) run ./experiments/ concurrent

run-10:
	@echo "=== 运行案例10：分区倾斜观测 ==="
	$(GO) run ./experiments/ skew

run-11:
	@echo "=== 运行案例11：集群健康巡检 ==="
	$(GO) run ./experiments/ health
