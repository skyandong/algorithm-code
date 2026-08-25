// # 实验三：写入优化与深分页
//
// 核心认知：
//   - Bulk API 把 N 次 HTTP 请求合并为 1 次，消除网络 RTT 开销，写入性能可提升数十倍
//   - refresh_interval 控制 Segment 生成频率，调大可减少小 Segment，提升写吞吐
//   - from+size 深分页：每个分片都要取 from+size 条，协调节点合并，内存随页数线性增长
//   - search_after 基于游标翻页，不需要跳页，内存恒定
//
// 本实验演示：
//   1. 逐条写 vs Bulk 写入 1000 条文档，耗时对比
//   2. refresh_interval 对写入吞吐的影响（-1 禁用 vs 默认 1s）
//   3. from+size 的限制说明（不真正执行到 10000 页，避免耗时过长）
//   4. search_after 游标翻页完整演示
//
// 对应笔记：04-分片与写入流程.md, 05-性能优化与生产实践.md

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

const bulkIndex = "bulk_test"

type BulkDoc struct {
	DocID   int    `json:"doc_id"`
	Title   string `json:"title"`
	Author  string `json:"author"`
	Score   int    `json:"score"`
	Created string `json:"created_at"`
}

// ExpBulkVsSingle 对比逐条写和 Bulk 写入的耗时。
//
// 逐条写：每条文档 = 1 次 HTTP 请求 + 1 次 Translog 写 + 1 次路由计算
// Bulk：  N 条文档 = 1 次 HTTP 请求，批量解析，批量写 Translog
//
// 建议单批次 5~15 MB，不是越大越好（太大 GC 压力大，容易超时）。
func ExpBulkVsSingle() {
	fmt.Println("=== 实验9: 逐条写 vs Bulk 写入 ===")

	esClient.Indices.Delete([]string{bulkIndex})

	// 创建索引
	mapping := `{
	  "settings": { "number_of_shards": 1, "number_of_replicas": 0 },
	  "mappings": {
	    "properties": {
	      "doc_id":     { "type": "integer" },
	      "title":      { "type": "text" },
	      "author":     { "type": "keyword" },
	      "score":      { "type": "integer" },
	      "created_at": { "type": "date", "format": "yyyy-MM-dd" }
	    }
	  }
	}`
	res, _ := esClient.Indices.Create(bulkIndex, esClient.Indices.Create.WithBody(strings.NewReader(mapping)))
	res.Body.Close()

	authors := []string{"alice", "bob", "carol", "dave", "eve"}
	const total = 500

	// --- 逐条写 ---
	start := time.Now()
	for i := 0; i < total; i++ {
		doc := BulkDoc{
			DocID:   i,
			Title:   fmt.Sprintf("文档-%d", i),
			Author:  authors[i%len(authors)],
			Score:   i % 100,
			Created: "2024-01-01",
		}
		body, _ := json.Marshal(doc)
		res, _ := esClient.Index(bulkIndex, bytes.NewReader(body))
		res.Body.Close()
	}
	singleDur := time.Since(start)

	// 重建索引（DeleteByQuery 是异步的，直接删重建更干净）
	esClient.Indices.Delete([]string{bulkIndex})
	res2, _ := esClient.Indices.Create(bulkIndex, esClient.Indices.Create.WithBody(strings.NewReader(mapping)))
	res2.Body.Close()

	// --- Bulk 写入（每批 100 条）---
	const batchSize = 100
	start = time.Now()
	for batch := 0; batch < total/batchSize; batch++ {
		var buf bytes.Buffer
		for i := batch * batchSize; i < (batch+1)*batchSize; i++ {
			// Bulk 格式：一行 action，一行文档；指定 _id=doc_id 保证唯一
			meta := fmt.Sprintf(`{"index":{"_index":"%s","_id":"%d"}}`, bulkIndex, i)
			doc := BulkDoc{
				DocID:   i,
				Title:   fmt.Sprintf("文档-%d", i),
				Author:  authors[i%len(authors)],
				Score:   i % 100,
				Created: "2024-01-01",
			}
			docBytes, _ := json.Marshal(doc)
			buf.WriteString(meta + "\n")
			buf.Write(docBytes)
			buf.WriteByte('\n')
		}
		res, err := esClient.Bulk(bytes.NewReader(buf.Bytes()))
		if err != nil || res.IsError() {
			log.Fatalf("bulk 写入失败: %v", err)
		}
		res.Body.Close()
	}
	bulkDur := time.Since(start)

	fmt.Printf("逐条写入 %d 条:  %v\n", total, singleDur)
	fmt.Printf("Bulk 写入 %d 条:  %v（每批 %d 条）\n", total, bulkDur, batchSize)
	fmt.Printf("提速: %.1fx  ← 消除了 %d 次 HTTP RTT\n\n",
		float64(singleDur)/float64(bulkDur), total)
}

// ExpRefreshInterval 演示 refresh_interval 的作用及批量导入最佳实践。
//
// 每次 Refresh 将内存 Buffer 中的数据写入新 Segment（文件系统缓存），数据才可被搜索。
// 频繁 Refresh（默认 1s）的代价：
//   - 不断生成小 Segment，查询需要合并更多 Segment 的结果
//   - 大量小文件，后台 Merge 压力大
//
// 注意：refresh_interval 影响的是"数据可见延迟"和"Segment 碎片化程度"，
// 不是写入本身的 TCP 吞吐。在写入量不大时，禁用 Refresh 对写入耗时改善不明显，
// 收益主要体现在写入量极大（数十万条/秒）且需要减少 Segment 碎片的场景。
//
// 批量导入时标准做法：
//  1. 设 refresh_interval=-1（禁用自动 Refresh，减少 Segment 碎片）
//  2. 设 number_of_replicas=0（关副本，省去副本同步开销）
//  3. 导完后恢复设置，手动触发 Refresh 使数据可见
func ExpRefreshInterval() {
	fmt.Println("=== 实验10: refresh_interval 说明 ===")
	fmt.Println("原理：每次 Refresh 把内存 Buffer 写入新 Segment，数据才可被搜索（NRT = 近实时）")
	fmt.Println()

	// 演示手动 refresh：写入后立即查，数据不可见；手动 refresh 后可见
	testDoc := `{"doc_id":99999,"title":"refresh-test","author":"tester","score":1,"created_at":"2024-06-01"}`
	meta := fmt.Sprintf(`{"index":{"_index":"%s","_id":"refresh-test"}}`, bulkIndex)
	payload := meta + "\n" + testDoc + "\n"

	// 先关掉自动 refresh
	res, _ := esClient.Indices.PutSettings(
		strings.NewReader(`{"index":{"refresh_interval":"-1"}}`),
		esClient.Indices.PutSettings.WithIndex(bulkIndex),
	)
	res.Body.Close()

	// 写入文档（不触发 refresh）
	esClient.Bulk(strings.NewReader(payload))

	// 立即查，应查不到
	searchRes, _ := esClient.Search(
		esClient.Search.WithIndex(bulkIndex),
		esClient.Search.WithBody(strings.NewReader(`{"query":{"term":{"author":"tester"}}}`)),
	)
	var r map[string]interface{}
	json.NewDecoder(searchRes.Body).Decode(&r)
	searchRes.Body.Close()
	count1 := r["hits"].(map[string]interface{})["total"].(map[string]interface{})["value"]
	fmt.Printf("写入后立即查（refresh_interval=-1）: 命中 %v 条（数据还在 Buffer，不可见）\n", count1)

	// 手动触发 refresh
	esClient.Indices.Refresh(esClient.Indices.Refresh.WithIndex(bulkIndex))

	// 再查，能查到
	searchRes2, _ := esClient.Search(
		esClient.Search.WithIndex(bulkIndex),
		esClient.Search.WithBody(strings.NewReader(`{"query":{"term":{"author":"tester"}}}`)),
	)
	var r2 map[string]interface{}
	json.NewDecoder(searchRes2.Body).Decode(&r2)
	searchRes2.Body.Close()
	count2 := r2["hits"].(map[string]interface{})["total"].(map[string]interface{})["value"]
	fmt.Printf("手动 POST /_refresh 后再查:           命中 %v 条（Segment 生成，数据可见）\n", count2)

	// 恢复默认
	res2, _ := esClient.Indices.PutSettings(
		strings.NewReader(`{"index":{"refresh_interval":"1s"}}`),
		esClient.Indices.PutSettings.WithIndex(bulkIndex),
	)
	res2.Body.Close()

	fmt.Println()
	fmt.Println("批量导入最佳实践：")
	fmt.Println("  1. PUT /{index}/_settings  { refresh_interval: -1, number_of_replicas: 0 }")
	fmt.Println("  2. Bulk 写入所有数据")
	fmt.Println("  3. POST /{index}/_refresh  （使数据可见）")
	fmt.Println("  4. PUT /{index}/_settings  { refresh_interval: 1s, number_of_replicas: 1 }")
	fmt.Println()
}

// ExpFromSizeLimit 演示 from+size 深分页的问题，不真正执行深翻页（避免耗时）。
//
// 为什么深分页会 OOM：
//   from=10000, size=10 时，每个分片需要返回 10010 条，
//   协调节点合并 N*10010 条后排序，再截取 10 条。
//   分片越多，内存消耗越大，容易 OOM。
//
// ES 默认限制 max_result_window=10000，超出会报错。
func ExpFromSizeLimit() {
	fmt.Println("=== 实验11: from+size 深分页限制 ===")

	// 正常翻页
	page1 := `{"from": 0, "size": 3, "_source": ["doc_id", "author", "score"], "sort": [{"score": "desc"}, {"doc_id": "asc"}]}`
	res, _ := esClient.Search(
		esClient.Search.WithIndex(bulkIndex),
		esClient.Search.WithBody(strings.NewReader(page1)),
	)
	var r1 map[string]interface{}
	json.NewDecoder(res.Body).Decode(&r1)
	res.Body.Close()

	hits := r1["hits"].(map[string]interface{})["hits"].([]interface{})
	fmt.Println("第1页（from=0, size=3）:")
	for _, h := range hits {
		hit := h.(map[string]interface{})
		src := hit["_source"].(map[string]interface{})
		fmt.Printf("  doc_id=%-4v  author=%-8s  score=%v\n", src["doc_id"], src["author"], src["score"])
	}

	// 尝试超出限制（from=10001）
	deepQuery := `{"from": 10001, "size": 10}`
	res2, _ := esClient.Search(
		esClient.Search.WithIndex(bulkIndex),
		esClient.Search.WithBody(strings.NewReader(deepQuery)),
	)
	defer res2.Body.Close()
	fmt.Printf("\nfrom=10001 查询 HTTP 状态码: %d\n", res2.StatusCode)
	fmt.Println("  → 400 错误：Result window is too large (max_result_window=10000)")
	fmt.Println("  → 解决方案：用 search_after 代替 from+size\n")
}

// ExpSearchAfter 演示基于游标的 search_after 翻页。
//
// search_after 原理：
//   - 每页查询传入上一页最后一条记录的排序值
//   - ES 从该排序值之后继续取，无需跳过前 N 条
//   - 内存消耗恒定（只需当前页 + 游标值），不随页数增长
//
// 注意：排序字段必须包含唯一值（如 doc_id）作为 tiebreaker，
//       否则相同排序值的文档可能被漏掉或重复。
func ExpSearchAfter() {
	fmt.Println("=== 实验12: search_after 游标翻页 ===")

	const pageSize = 3
	var searchAfterValues []interface{}
	page := 1

	fmt.Printf("翻页演示（pageSize=%d，排序：score DESC + doc_id ASC）:\n", pageSize)

	for page <= 4 { // 演示翻 4 页
		var queryBuf bytes.Buffer
		baseQuery := map[string]interface{}{
			"size":    pageSize,
			"_source": []string{"doc_id", "author", "score"},
			"sort": []map[string]interface{}{
				{"score": "desc"},
				{"doc_id": "asc"}, // tiebreaker，doc_id 与 _id 一致，全局唯一
			},
		}
		if len(searchAfterValues) > 0 {
			baseQuery["search_after"] = searchAfterValues
		}
		json.NewEncoder(&queryBuf).Encode(baseQuery)

		res, err := esClient.Search(
			esClient.Search.WithIndex(bulkIndex),
			esClient.Search.WithBody(bytes.NewReader(queryBuf.Bytes())),
		)
		if err != nil || res.IsError() {
			log.Fatalf("search_after 查询失败: %v", err)
		}

		var result map[string]interface{}
		json.NewDecoder(res.Body).Decode(&result)
		res.Body.Close()

		hits := result["hits"].(map[string]interface{})["hits"].([]interface{})
		if len(hits) == 0 {
			fmt.Println("  无更多数据")
			break
		}

		fmt.Printf("  第%d页:\n", page)
		for _, h := range hits {
			hit := h.(map[string]interface{})
			src := hit["_source"].(map[string]interface{})
			fmt.Printf("    doc_id=%-4v  author=%-8s  score=%v\n", src["doc_id"], src["author"], src["score"])
		}

		// 取最后一条的排序值作为下一页游标
		lastHit := hits[len(hits)-1].(map[string]interface{})
		searchAfterValues = lastHit["sort"].([]interface{})
		fmt.Printf("  游标（search_after）: %v\n\n", searchAfterValues)

		page++
	}

	fmt.Println("结论：")
	fmt.Println("  search_after 每次只取当前页数据，内存恒定，不随页数增长")
	fmt.Println("  不支持跳页（只能顺序翻页），适合瀑布流/无限滚动场景")
	fmt.Println("  排序字段必须含唯一值作为 tiebreaker（这里用 doc_id）\n")
}
