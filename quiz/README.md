# quiz：笔记知识点答题评测

从本仓库 `notes/` 下的知识点出选择题，每次同一知识点出 5 道，提交后逐题批改并给解析。
题目与解析预先随服务入库（写在 `internal/seed/` 里），批改在服务端完成，
**运行期不依赖任何外部大模型或 API key**。

## 运行

```bash
cd quiz

# 1. 起数据库（MySQL 8.0，端口 3306；附带 adminer 在 8081）
docker compose up -d mysql

# 2. 启动服务（自动建表、导入题库、托管页面）
go run ./cmd/server
# 默认监听 :8090，打开 http://localhost:8090 即可答题
```

可选参数：

```bash
go run ./cmd/server -addr :9090          # 换端口
go run ./cmd/server -seed-only           # 只导入题库后退出
go run ./cmd/server -dsn 'user:pass@tcp(127.0.0.1:3306)/quiz?parseTime=true'
```

浏览器进度按「匿名用户」区分：标识存在 localStorage，换浏览器即新一轮进度。

## 答题规则

- 一次出 5 道新题，全部来自**同一个知识点**，知识点由你在页面上手选；
- 做过的题会被记住（`answered` 表），同一道新题不会第二次出现；
- 剩余新题不足 5 道时**拒绝出题**（宁可不出，不重复也不硬凑）；
- **错题重练**：做错（含没答）的题进错题池，卡片上出现「重练错题」入口；
  错题有多少出多少，答对一次就出池，做错仍留在池里反复练；
- 提交后逐题批改：答对答错都有描述、正确项高亮、附解析与笔记出处；
  结果页可「只看错题」，并显示与上次成绩的对比；
- 未提交的一轮作答会暂存在 localStorage：刷新页面自动恢复，返回列表后可「继续答」；
- 支持键盘答题：`1-4` 或 `A-D` 选选项，`↑↓` / `J K` 切题，`Enter` 提交，`Esc` 返回；
- 页面提供「清空进度」，让题库回到全部没做过的状态（错题池一并清空）。

## 目录结构

```
quiz/
├── docker-compose.yml          # mysql:8.0 + adminer
├── cmd/server/main.go          # 入口：连接、建表、导入题库、起 HTTP
├── internal/
│   ├── model/                  # 题目 / 知识点 / 批改结果等领域结构
│   ├── store/                  # 全部 SQL：建表、导入、抽题、批改、历史
│   ├── seed/                   # 题库（按知识点一个文件）
│   └── api/                    # HTTP 路由与参数校验
└── web/                        # 答题页面（原生 HTML/CSS/JS，无构建）
```

## 怎么加题

1. 在 `internal/seed/` 仿照 `golang_01_data_structure.go` 新建一个文件：
   定义 `topic`（模块、slug、名称、笔记出处），用 `t.builder()` 出题，
   每道题依次是题干、四个选项、正确项下标（0 开始）、解析。
2. 在 `internal/seed/seed.go` 的 `providers` 里追加一行。
3. 重启服务，题目按 QID 幂等导入（同 QID 覆盖内容，不会重复）。

题目口径以 `notes/` 各分册与 `面试一口答.md` 为准；解析要求把错误选项的误区一并讲清。

## 接口一览

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/api/topics` | 知识点列表与进度（含正确数、错题数，`?user=` 区分用户） |
| GET  | `/api/quiz?topic=<slug>&n=5&mode=new` | 抽题；`mode=wrong` 抽错题。新题不够一轮返回 `available=false` |
| POST | `/api/submit` | 提交整卷，返回逐题批改、解析与上次成绩 |
| GET  | `/api/history` | 最近 20 次作答记录 |
| POST | `/api/reset` | 清空该用户的作答进度 |

下发给页面的题目视图**不含答案与解析**，客户端在提交前拿不到正确项。
