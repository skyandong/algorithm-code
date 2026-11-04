# 内存回收与 OOM

> **核心认知：** Linux 的内存原则是"**空闲的内存是浪费的内存**"——页缓存会吃掉所有空闲内存，需要时再回收。所以 `free` 里 `buff/cache` 高完全正常，`available` 才是可用内存。真正决定你死不死的是**回收能不能跟上分配速度**：跟不上就进直接回收（延迟炸），再跟不上就 OOM。

---

## 1. 内存都花在哪

| 类型 | 例子 | 可回收？ |
|------|------|---------|
| 匿名页（anon） | 堆、栈、`malloc`/`mmap(MAP_ANONYMOUS)` | 只能换到 swap（无 swap 则不可回收） |
| 文件页（file） | 页缓存、可执行文件代码段、mmap 的文件 | ✅ 干净页直接丢，脏页回写后丢 |
| slab | dentry/inode 缓存、内核对象 | 部分可回收 |
| 内核栈 / 页表 | 每个线程一份内核栈、每个进程页表 | ❌ 不可回收 |

**关键区别**：文件页是"缓存"，不是"占用"——它随时可以被丢弃再从磁盘读回。匿名页是真正的"占用"，因为它的内容只存在于内存（或 swap）里。

---

## 2. 回收机制：kswapd 与直接回收

```
空闲内存水位：
   high  ─────────────  kswapd 回收到这里就停
   low   ─────────────  kswapd 被唤醒，开始后台回收
   min   ─────────────  低于这里，分配路径必须自己回收（direct reclaim）
```

- **kswapd**：每个 NUMA 节点一个内核线程，后台异步回收，不阻塞业务。
- **直接回收（direct reclaim）**：分配内存时发现低于 `min` 水位，**调用者自己同步回收**——这时你的 `malloc`/缺页会卡住几十毫秒甚至更久。这是**延迟毛刺的常见根因**，而且监控上往往看不出 CPU 或 IO 异常。
- 低于 `min` 且回收不出来 → 触发 OOM killer。

**LRU 四条链**：`active/inactive × anon/file`。新页进 inactive 尾部，被二次访问提升到 active；回收从 inactive 头部开始。这个"二次机会"机制避免把刚用过的热页踢掉。

内核 5.9+ 还有 **MGLRU**（多代 LRU），在高内存压力下减少扫描开销；容器场景里它能明显改善"回收把热页也扫掉"的问题。

---

## 3. swap：把匿名页换出去

- 作用：让匿名页也能"暂时腾地方"，避免 OOM。
- 代价：换回来的那次访问是 **major fault**（走磁盘），延迟从纳秒变毫秒。
- 容器里通常**禁用 swap**（`memory.swap.max=0`），因为延迟不可控；物理机上一般留一点做缓冲。

判读：

```bash
vmstat 1        # si/so 持续非 0 = 真的在换页，性能已经受影响
```

**`si/so` 长期非零是最明确的"内存不够"信号**，比看使用率百分比准。

---

## 4. cgroup v2 的内存账本

```bash
cat /sys/fs/cgroup/memory.max        # 硬上限（容器内存 limit）
cat /sys/fs/cgroup/memory.high       # 软上限：超过后被"节流式回收"，不杀进程
cat /sys/fs/cgroup/memory.current    # 当前用量
cat /sys/fs/cgroup/memory.stat       # anon / file / slab / sock / kernel_stack 明细
cat /sys/fs/cgroup/memory.events     # high / max / oom / oom_kill 计数
```

| 关键点 | 说明 |
|--------|------|
| `memory.max` 是**硬限** | 触顶且回收不出来 → cgroup OOM |
| `memory.high` 是**软限** | 超过后内核让该 cgroup 的分配变慢、积极回收，**不杀进程**——适合做"预警线"，但配太低会造成持续的性能损失 |
| `memory.current` 含页缓存 | 所以"用量接近上限"不一定是真要 OOM，先看 `memory.stat` 里 file 占多少 |
| `memory.events` 的 `max` | 每次触顶 +1，**持续增长就说明一直在顶着上限跑** |

**容器里最经典的误判**：`memory.current` 逼近 `memory.max`，但 `file` 占了大头（页缓存）——这部分是可回收的，内核会在压力下自动丢。真正危险的是 `anon` 逼近上限。

---

## 5. OOM killer 怎么选受害者

```
全局 OOM：oom_badness = (rss + swap + page_tables) × oom_score_adj 系数 → 选分数最高的
cgroup OOM：在该 cgroup 内比较，同样选占用最大的
```

- `/proc/<pid>/oom_score`：当前得分（0~1000）。
- `/proc/<pid>/oom_score_adj`：手动调整（-1000 表示永不被杀，1000 表示优先被杀）。
- 杀完在 `dmesg` 留下 `Out of memory: Killed process 12345 (myapp) total-vm:... rss:...`——**这是确认"是不是被 OOM 杀的"唯一可靠证据**。

**Java/Go 这类"自己管内存"的进程容易被选中**：RSS 大、回收慢，内核按 badness 排序时天然靠前。所以给这类进程设合理的 `oom_score_adj` 保护（如 -500）是常见做法。

---

## 6. Go 服务的内存谜题

| 现象 | 原因 |
|------|------|
| GC 后 RSS 不降 | Go 1.12~1.15 用 `MADV_FREE`（延迟归还）；1.16 起默认 `MADV_DONTNEED` |
| 容器里被 OOM，但 heap 不高 | 页缓存 + Go 未归还的 `HeapIdle` + 栈/线程开销都在 RSS 里 |
| `GOGC` 调大后反而更稳 | 分配频繁的小对象服务，GC 太频繁会吃 CPU 并加剧延迟；调大 GOGC 换内存降 CPU |
| 有 `GOMEMLIMIT` 还需要 GOGC 吗 | 要。`GOMEMLIMIT` 是软限（到顶后 GC 变积极），GC 频率仍由 GOGC 控制 |
| 为什么要低于容器 limit 设 GOMEMLIMIT | 因为 limit 是硬限，触顶直接 cgroup OOM；留 10%~20% 给栈、页缓存、非 Go 内存 |

**GOMEMLIMIT 的正确用法**：设成容器 limit 的 80% 左右，而不是等于 limit——Go 管不到 C 库、线程栈、页缓存等内存。

---

## 7. 排查顺序（内存高）

```
① 是不是 cgroup 硬限：看 memory.current / memory.max / memory.events(max 是否增长)
② 是不是 anon 高：memory.stat 里 anon 占比；file 高则是页缓存，可回收
③ 进程级：按 PSS 排序找真凶（不是 RSS，RSS 把共享页重复计）
④ Go 进程：pprof heap（inuse_space 看活对象，alloc_space 看分配总量）
⑤ 是否泄漏：长时间观察 HeapAlloc 是否单调上升；对比 GC 后的基线
⑥ 是否 GC 参数不当：GOGC / GOMEMLIMIT / 对象分配率（allocs/op）
⑦ 是否缺页 IO：major fault 增长、si/so 非零、direct reclaim（内核 log 里的 `order-N` 分配失败）
```

**别一上来就怀疑泄漏**：Go 服务"内存慢慢涨"多数是页缓存 + HeapIdle 未归还，而不是泄漏。真正的泄漏会表现为**GC 之后 HeapAlloc 的基线持续抬高**。

---

## 8. 记忆锚点（面试 30 秒版）

> "Linux 把空闲内存全变成页缓存，所以要看的不是使用率而是 available。匿名页不可丢弃（只能 swap），文件页随时可回收；回收靠 kswapd 后台做，跟不上就进直接回收、分配路径自己同步回收，造成延迟毛刺，再不行就 OOM。容器里看 memory.current/max 和 memory.events，anon 逼近上限才真危险，file 占大头只是页缓存。OOM 杀的是 badness 最高的，也就是 RSS 最大的进程，证据只在 dmesg 里。"

---

## 实验

`go run ./experiments/ memory` 观察 Go 与内核之间的账：

- 分配一批对象后 `runtime.GC()` + `debug.FreeOSMemory()`，对比 `HeapAlloc / HeapIdle / HeapReleased`
- 读 Linux 上的 `/proc/meminfo` 与 cgroup `memory.stat`（其它平台打印等价说明），看 anon/file 的分布
- 结论：HeapReleased 增加 ≠ RSS 立刻下降，两者之间有页缓存与 madvise 语义的差

对应代码：[experiments/06_memory.go](experiments/06_memory.go) / [experiments/06_memory_linux.go](experiments/06_memory_linux.go)
