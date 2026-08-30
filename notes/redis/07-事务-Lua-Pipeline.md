# 事务 / Lua / Pipeline

> **核心认知：三者经常被混淆，一句话锚定：
> Pipeline 省网络，不保证原子；
> MULTI/EXEC 保顺序，不保回滚；
> Lua 真原子，不省网络。
> Redis 没有 MySQL 那样的 ACID 事务——"要不要原子性"决定了你该用哪个工具。**

---

## 三者对比总表（先背这张）

| | 原子性 | 网络优化 | 回滚 | 适用场景 |
|--|--------|---------|------|---------|
| Pipeline | ✗（命令可被其他客户端穿插） | ✓（N 次 RTT → 1 次） | 无 | 批量读写，不在乎穿插 |
| MULTI/EXEC | 半（顺序不被穿插） | ✗ | ✗ | 需要隔离但能接受部分失败 |
| Lua | ✓（真原子） | ✗ | ✗ | 判断+操作必须不可分割 |

---

## MULTI / EXEC / DISCARD / WATCH

### 执行流程

```
MULTI        开启事务，后续命令只入队不执行（返回 QUEUED）
命令 1..N    全部入队
EXEC         一次性顺序执行所有入队命令，按序返回结果
DISCARD      清空队列，放弃事务
```

### 两种错误，两种命运（高频考点）

```
1. 入队时错误（命令名拼错等语法错误）
   → 整个事务被拒绝，一条都不执行

2. 执行时错误（如对 String 执行 LPUSH，类型错误）
   → 只有出错的这一条失败，其余命令照常执行
   ← 这就是"不支持回滚"
```

**结论的精确表述**：Redis 事务满足**隔离性**（EXEC 执行期间不被其他客户端命令插入），不满足**原子性**（部分失败不回滚）。

实验验证：事务里放一条 `LPUSH` 打 String（必错）和一条正常 `SET`，EXEC 后 SET 照样生效——错误命令没有拖垮事务。

**为什么设计成不回滚**（官方理由）：回滚机制复杂且拖性能，而执行时错误只会由编程错误（类型用错）引起——属于 bug，上生产前就该被测出来，回滚也救不了逻辑错误。

### WATCH：乐观锁（CAS）

```
WATCH key          监视 key（快照当前值）
MULTI ... EXEC     若 key 在 WATCH 后被任何客户端修改
                   → EXEC 返回 nil，整个事务放弃
```

本质是 CAS：先拍快照，提交前比对，变了就放弃重试。适合"读 → 计算 → 写回"的并发安全场景（如余额更新）。重试循环由客户端自己实现。

实验验证：WATCH 后人为改 key，EXEC 的 SET 未生效，key 保持外部修改后的值。

---

## Lua 脚本

### 为什么是真原子

Redis 单线程执行 Lua 脚本，**执行期间不插入任何其他命令**——比 MULTI 更强的原子（MULTI 只是排队顺序执行，Lua 是整体不可分割）。

### EVAL vs EVALSHA

```
EVAL script numkeys key... arg...    每次发完整脚本 → 带宽浪费
SCRIPT LOAD script → 返回 SHA1       脚本缓存到服务端
EVALSHA sha1 numkeys key... arg...   只发 40 字节 SHA1 → 省带宽
```

脚本越长、调用越频繁，EVALSHA 收益越大。生产代码应 ScriptLoad 一次 + EvalSha 复用（注意处理 NOSCRIPT 错误降级回 EVAL）。

### 经典用途：分布式锁的原子释放

```lua
if redis.call('get', KEYS[1]) == ARGV[1] then
    return redis.call('del', KEYS[1])
else
    return 0
end
```

"验证 token 是自己的 + 删除锁"必须是不可分割的整体，详见《08-分布式锁》。

### Lua 的代价

- 脚本执行期间独占主线程——**脚本慢 = 全库慢**
- `lua-time-limit`（默认 5000ms）超时后 Redis 对其他命令返回 BUSY 错误（默认不主动杀脚本，可 `SCRIPT KILL` 终止未写入的脚本）
- 生产规则：Lua 必须短小，禁止循环和复杂计算

---

## Pipeline

### 原理

```
逐条：     write → flush → read | write → flush → read | ...   N 次 RTT
pipeline： write×N → flush → read×N                           1 次 RTT
```

纯客户端行为：攒一批命令一次发出、一次读回全部响应。服务端按序执行、按序回包。

### Pipeline ≠ MULTI（本质区别）

```
Pipeline：客户端打包发送。命令到达服务端后，与其他客户端的命令可能交替执行
MULTI：   服务端保证命令队列连续执行，不被穿插
```

实验：1000 次 RPUSH，逐条 vs Pipeline 提速一个数量级（RTT 越大提升越明显）。

### 使用注意

- 单批不宜过大（建议 ≤ 1000 条）：攒太多会撑爆服务端输入缓冲，响应也一次性回传造成大包
- go-redis 中 `Pipelined()` 自动收集全部命令结果；`Pipeline()` + 手动 `Exec()` 更灵活

---

## 决策树

```
只是批量读写，穿插无所谓？        → Pipeline
需要命令连续执行、能接受部分失败？  → MULTI/EXEC
"先判断再操作"必须不可分割？       → Lua（EVALSHA）
读改写并发安全（乐观锁）？         → WATCH + MULTI
需要严格的 ACID？                  → 别用 Redis，去用数据库
```

---

## 对应实验

对应代码：[experiments/07_transaction_lua_pipeline.go](experiments/07_transaction_lua_pipeline.go)

```bash
go run ./experiments/   # 第七节输出
```

重点观察：事务中 LPUSH 报错但 SET 仍生效（不回滚）；WATCH 被并发修改后事务放弃；Lua 错误 token 释放锁被拒（原子判断）；pipeline vs 逐条的耗时差。
