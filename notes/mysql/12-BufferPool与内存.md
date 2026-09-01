# Buffer Pool 与内存管理

> **核心认知：磁盘是 10 万倍慢于内存的，InnoDB 的内存管理就是"用有限的内存尽量挡住磁盘 IO"。核心组件：Buffer Pool 缓存页 + 改进版 LRU 决定谁留下、Change Buffer 兜住二级索引写、Doublewrite 防页断裂。**

---

## Buffer Pool 基础

缓存磁盘数据页（16KB/页）的内存区域，**读写都先经过它**：

- **读**：先查 BP，命中直接返回（逻辑读）；未命中从磁盘加载（物理读）
- **写**：只改 BP 中的页（变脏页），由后台线程异步刷盘（WAL 保证不丢）

```
参数：innodb_buffer_pool_size（默认 128MB，生产通常设为物理内存的 50%~75%）
     innodb_buffer_pool_instances（多实例减少锁竞争，大内存建议 8 个左右）

状态：SHOW GLOBAL STATUS LIKE 'Innodb_buffer_pool_read%';
     命中率 = 1 - Innodb_buffer_pool_reads / Innodb_buffer_pool_read_requests
     健康值 > 99%，低于 95% 说明内存不足或存在全表扫描污染
```

---

## 改进版 LRU（面试高频）

**朴素 LRU 的问题：** 一次全表扫描 / mysqldump 备份会把几万个页灌进 LRU，把真正的热数据全部挤出去——冷数据"污染"缓存。

**InnoDB 的解法：把 LRU 链表分成两段：**

```
        ┌── new 区（热数据，约 5/8）──┐ ┌── old 区（冷数据，约 3/8）──┐
      ← [ 热 ... 热 ]← midpoint ← [ 冷 ... 冷 ] ← [ 新页从这插入 ]
                                        ↑
                          所有新加载的页都插入 old 区头部，不进 new 区

晋升规则：页在 old 区停留超过 innodb_old_blocks_time（默认 1 秒）
         之后再次被访问 → 才晋升到 new 区头部
```

- 参数：`innodb_old_blocks_pct`（old 区占比，默认 37%）、`innodb_old_blocks_time`（默认 1000ms）
- **防污染原理**：全表扫描的页从 old 区头部进，1 秒内被顺序扫一遍就丢弃——大多活不过 1 秒，永远进不了 new 区；真正的热页会被反复访问，轻松晋升
- 面试标准问法："InnoDB 的 LRU 和教科书 LRU 有什么区别？"——答分区 + midpoint 插入 + 时间阈值三件套

**old 区内的移动：** 访问 old 区的页若未到 1 秒，不动；到了 1 秒后访问才提升。**new 区头部的页**被访问时也不立刻移到头部（有 1/4 new 区长度保护，防止批量扫描 new 区造成频繁链表抖动）。

---

## 脏页刷新

Buffer Pool 中的页和磁盘不一致时是脏页，由 **Page Cleaner** 线程异步刷盘。触发刷脏的四种时机：

| 时机 | 说明 | 业务感知 |
|------|------|---------|
| Redo Log 快写满 | checkpoint 必须推进，强制大量刷脏 | **业务写卡顿**（平时避免） |
| BP 不足 | 淘汰的页是脏页，得先刷盘才能复用 | 查询变慢 |
| 后台周期刷新 | 每秒按脏页比例自适应刷（Redo 增长越快刷越猛） | 无感 |
| 正常关闭 | 全部刷干净 | 无感 |

```
关键参数：innodb_max_dirty_pages_pct（脏页比例上限，默认 90%）
        刷新速率自适应：Redo 产生越快，后台刷得越猛
```

**"Redo Log 写满"和"脏页太多"是生产写卡顿的两大内存侧根因**——对应调大 Redo 容量 / 调低脏页上限 / 扩 BP。

---

## Change Buffer（写缓冲）

**问题：** `INSERT/UPDATE` 要修改的二级索引页不在 BP 里时，读盘 + 修改，代价高。

**解法：** 若该二级索引页不在 BP：
1. 先把变更缓存到 Change Buffer（内存 + 系统表空间持久化）
2. 后台线程或下次读到该页时再 merge

**限制：**
- 只适用于**非唯一**的二级索引（唯一索引必须读页校验唯一性——这也是"唯一索引 vs 普通索引怎么选"的关键：**写多读少用普通索引 + Change Buffer，读多直接唯一索引**）
- 写多读少场景收益最大（写完很久才读，merge 摊平）；写完立刻读反而多一次 merge

```
参数：innodb_change_buffer_max_size（占 BP 的比例，默认 25%）
状态：SHOW ENGINE INNODB STATUS 里的 INSERT BUFFER AND ADAPTIVE HASH INDEX 段
```

---

## 自适应哈希索引（AHI）

InnoDB 自动给**热点页**的等值查询建内存哈希索引：`B+ 树 3~4 次定位 → 哈希 1 次`。

- 全自动，无法手动指定；`SHOW ENGINE INNODB STATUS` 可看到命中率
- 代价：哈希表有锁竞争，高并发等值查询场景反而可能成瓶颈（8.0 分了分区锁缓解）
- 可用 `innodb_adaptive_hash_index = OFF` 关闭排查

---

## Doublewrite Buffer（双写缓冲）

**问题：** 崩溃时一个 16KB 页只写了一半（页断裂/partial write），Redo Log 记录的是"页内某偏移改成某值"——**基于页完好的物理日志无法修复半页**。

**解法：** 刷脏页前，先把页**顺序写**到共享表空间的 Doublewrite 区域（两次写），再写回数据文件：

```
① 脏页批量顺序写 doublewrite buffer（顺序 IO，快）
② 逐页写回数据文件（随机 IO）
③ 崩溃恢复：数据页损坏 → 从 doublewrite 副本还原完整页 → 再用 Redo 重放
```

面试一句话：**Redo 是物理逻辑日志（页内变更），修不了半页；Doublewrite 保证了"页本身完好"这个前提。**

---

## 一条 UPDATE 涉及的内存组件全景

```
UPDATE t SET x=1 WHERE id=10

① BP 中定位 id=10 所在页（未命中则从磁盘加载，进 LRU old 区）
② 写 Undo Log（旧值）
③ BP 中修改该页 → 变脏页
④ 若有二级索引且页不在 BP → 变更进 Change Buffer
⑤ 写 Redo Log（顺序写，WAL）
⑥ 后台：Page Cleaner 异步刷脏页（先过 Doublewrite）
```

---

## 面试一句话总结

- BP 默认 128MB，生产设物理内存 50%~75%；命中率 `1 - 物理读/逻辑读`，健康值 > 99%。
- LRU 分 new/old 两区（默认 37% old），新页插 old 区头部，**活过 1 秒再被访问才晋升**——防全表扫描污染。
- 刷脏四时机：Redo 快写满（业务卡顿）、BP 不足、后台自适应、正常关闭。
- Change Buffer 只作用于**非唯一二级索引**的写，写多读少收益大；唯一索引必须读页校验。
- Doublewrite 解决页断裂：Redo 是页内物理日志，前提是页完整；先顺序写副本再写数据文件。
- AHI 自动给热点页建哈希，等值查询加速，高并发下有锁竞争可关。
