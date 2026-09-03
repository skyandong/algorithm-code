# LeetCode 核心题（S 级）

从 `leetcode/` 中精选的高频核心题。这些题在面试中反复出现，目标是能在 5 分钟内白板盲写、零思考。

## 清单（21 题）

### 链表（5）
| 文件 | 题目 | 手撕要点 |
|---|---|---|
| reverseList | 反转链表 | 递归 + 迭代两种都要会，迭代用三指针 prev/curr/next |
| hasCycle | 环形链表 | 快慢指针，注意起步点 |
| mergeTwoLists | 合并两个有序链表 | dummy 哨兵节点 |
| removeNthFromEnd | 删除倒数第 N 个 | 双指针间距 N，dummy 处理头删 |
| reverseKGroup | K 个一组翻转 | hard，先写 reverseList 区间版再串起来 |

### 二叉树（5）
| 文件 | 题目 | 手撕要点 |
|---|---|---|
| inorderTraversal | 中序遍历 | 递归 + 栈迭代两种写法都要熟 |
| levelOrder | 层序遍历 | BFS 模板，队列长度快照 |
| maxDepth | 最大深度 | DFS / BFS 两种 |
| invertTree | 翻转二叉树 | 递归 swap，1 分钟内写出 |
| lowestCommonAncestor | 最近公共祖先 | 后序 DFS，左/右子树返回值判断 |

### 双指针（2）
| 文件 | 题目 | 手撕要点 |
|---|---|---|
| threeSum | 三数之和 | 排序 + 双指针，**去重是核心考点** |
| trap | 接雨水 | 双指针 O(1) 空间 / 单调栈两种 |

### 滑动窗口（1）
| 文件 | 题目 | 手撕要点 |
|---|---|---|
| lengthoflongestsubstring | 无重复字符的最长子串 | 滑窗模板：右扩左缩，map 记索引 |

### 动态规划（4）
| 文件 | 题目 | 手撕要点 |
|---|---|---|
| climbStairs | 爬楼梯 | DP 入门，注意只看前两项 |
| rob | 打家劫舍 | 状态转移 f(i)=max(f(i-1), f(i-2)+nums[i]) |
| coinChange | 零钱兑换 | 完全背包，初始化 inf |
| longestPalindrome | 最长回文子串 | 中心扩展法比 DP 更易写对 |

### 哈希（1）
| 文件 | 题目 | 手撕要点 |
|---|---|---|
| twosum | 两数之和 | map 一次遍历，查补数 |

### 栈（1）
| 文件 | 题目 | 手撕要点 |
|---|---|---|
| isValid | 有效的括号 | 栈匹配，注意三种括号 |

### 数组（2）
| 文件 | 题目 | 手撕要点 |
|---|---|---|
| maxSubArray | 最大子数组和 | Kadane 算法，DP 思想 |
| merge | 合并区间 | 按起点排序，遍历合并重叠区间 |

## 复习方法

1. **四个母题模板要内化成肌肉记忆**：链表反转、滑窗（右扩左缩）、双指针（排序去重）、BFS（队列层序）。这四个模板覆盖了 21 题里的一大半。
2. **递归 + 迭代双写**：`inorderTraversal`、`reverseList` 这类必须两种写法都能默写，面试官常指定写法。
3. **hard 优先级**：`trap`、`reverseKGroup` 是 hard 里出镜率最高的，先吃透这两道。

## 不在本目录但需重点关注的题

- `leetcode/linked_list/lrucache.go` — LRU 缓存，Go 后端必考，map + 双向链表手写
- `leetcode/heap/findKthLargest.go` / `topKFrequent.go` — 堆 / TopK
- `leetcode/graph_theory/canFinish.go` — 拓扑排序
- `leetcode/graph_theory/numIslands.go` — DFS/BFS 模板
