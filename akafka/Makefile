GO := /Users/tal/sdk/go1.26.3/bin/go

.PHONY: up down run-01 run-02 run-03 run-04 run-05 run-06 topics

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

run-01:
	@echo "=== 运行案例1：生产者基础 ==="
	cd 01-producer-basic && $(GO) run main.go

run-02:
	@echo "=== 运行案例2：消费者组（Ctrl+C 退出）==="
	cd 02-consumer-group && $(GO) run main.go

run-03:
	@echo "=== 运行案例3：Exactly-Once ==="
	cd 03-exactly-once && $(GO) run main.go

run-04:
	@echo "=== 运行案例4：死信队列（Ctrl+C 退出）==="
	cd 04-dead-letter && $(GO) run main.go

run-05:
	@echo "=== 运行案例5：消费积压监控（Ctrl+C 退出）==="
	cd 05-consumer-lag && $(GO) run main.go

run-06:
	@echo "=== 运行案例6：多 Topic 流水线（Ctrl+C 退出）==="
	cd 06-pipeline && $(GO) run main.go
