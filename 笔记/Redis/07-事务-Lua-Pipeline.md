# 7. 事务 / Lua / Pipeline

## 事务(MULTI / EXEC / DISCARD / WATCH)
- `MULTI` 开启 → 命令入队 → `EXEC` 顺序执行
- **不支持回滚**:命令执行报错(如类型错误)其他命令仍会执行
- **WATCH 乐观锁**:监视 Key,事务执行前若被修改则整个事务放弃(CAS 模式)
- Redis 事务本质是"批量执行",**不满足 ACID 的原子性**(无回滚)

## Lua 脚本
- **真原子操作**:脚本在 Redis 内单线程执行,中间不会插入其他命令
- 经典用途:**分布式锁释放**(`if redis.call('get',key)==val then return redis.call('del',key) end`),保证"判断+删除"原子性
- 可用 `EVAL` / `EVALSHA`(缓存脚本省带宽)

## Pipeline
- 客户端批量发送命令,**减少网络 RTT**
- **不保证原子性**:命令可能被其他客户端的命令穿插

## 三者对比
|  | 原子性 | 网络优化 | 回滚 |
|--|--------|----------|------|
| Pipeline | 否 | 是(省 RTT) | - |
| 事务 MULTI | 顺序执行,不保回滚 | 否 | 无 |
| Lua | 是 | 否 | - |
