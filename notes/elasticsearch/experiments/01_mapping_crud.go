// # 实验一：Mapping 定义与文档 CRUD
//
// 核心认知：
//   - Mapping 是 ES 最重要的前期决策，字段类型一旦确定基本不能改（改需 reindex）
//   - text 字段被分词，keyword 不分词；既要搜索又要聚合的字段用 fields 多字段
//   - Dynamic Mapping 会自动推断类型，但容易推错，生产建议关闭
//
// 本实验演示：
//   1. 创建带显式 Mapping 的索引（articles）
//   2. 文档的增删改查（CRUD）
//   3. Dynamic Mapping 陷阱：让 ES 自动推断，观察推断结果
//
// 对应笔记：02-分词与Mapping.md

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
)

const articlesIndex = "articles"
const dynamicIndex = "dynamic_trap"

// Article 是实验用的文档结构。
type Article struct {
	Title     string   `json:"title"`
	Content   string   `json:"content"`
	Author    string   `json:"author"`
	Tags      []string `json:"tags"`
	Price     float64  `json:"price"`
	CreatedAt string   `json:"created_at"`
}

// ExpMappingCreate 创建带显式 Mapping 的索引。
//
// 设计要点：
//   - title: text（全文检索）+ keyword subfield（精确匹配/排序）
//   - author/tags: keyword（精确过滤、聚合）
//   - dynamic: strict（拒绝未定义的字段，生产常用，防止 Mapping 爆炸）
func ExpMappingCreate() {
	// 先删索引（保证实验幂等）
	esClient.Indices.Delete([]string{articlesIndex})

	mapping := `{
	  "settings": {
	    "number_of_shards": 1,
	    "number_of_replicas": 0,
	    "refresh_interval": "1s"
	  },
	  "mappings": {
	    "dynamic": "strict",
	    "properties": {
	      "title": {
	        "type": "text",
	        "analyzer": "standard",
	        "fields": {
	          "keyword": { "type": "keyword" }
	        }
	      },
	      "content":    { "type": "text", "analyzer": "standard" },
	      "author":     { "type": "keyword" },
	      "tags":       { "type": "keyword" },
	      "price":      { "type": "float" },
	      "created_at": { "type": "date", "format": "yyyy-MM-dd" }
	    }
	  }
	}`

	res, err := esClient.Indices.Create(articlesIndex, esClient.Indices.Create.WithBody(strings.NewReader(mapping)))
	if err != nil || res.IsError() {
		log.Fatalf("创建索引失败: %v %s", err, readBody(res.Body))
	}
	res.Body.Close()

	fmt.Println("=== 实验1: Mapping 创建 ===")
	fmt.Printf("索引 [%s] 创建成功\n", articlesIndex)
	fmt.Println("  title: text + keyword subfield（搜索用 text，排序/聚合用 title.keyword）")
	fmt.Println("  author/tags: keyword（只做精确匹配，不分词）")
	fmt.Println("  dynamic: strict（未定义字段会报错，防止 Mapping 膨胀）\n")
}

// ExpDocumentCRUD 演示文档的增删改查。
//
// 注意：ES 的"更新"本质是先标记旧文档删除，再写入新文档（Segment 不可变）。
// 用 _update API 可以只传变更字段，但底层仍是全文档替换。
func ExpDocumentCRUD() {
	fmt.Println("=== 实验2: 文档 CRUD ===")

	// --- 写入 ---
	docs := []Article{
		{Title: "Elasticsearch 入门指南", Content: "ES 是基于 Lucene 的分布式搜索引擎", Author: "alice", Tags: []string{"elasticsearch", "search"}, Price: 99.0, CreatedAt: "2024-01-10"},
		{Title: "Golang 并发编程", Content: "goroutine 和 channel 是 Go 并发的核心", Author: "bob", Tags: []string{"golang", "concurrency"}, Price: 129.0, CreatedAt: "2024-02-15"},
		{Title: "MySQL 索引原理", Content: "B+树扇出决定树高，3-4层覆盖亿级数据", Author: "alice", Tags: []string{"mysql", "database"}, Price: 89.0, CreatedAt: "2024-03-20"},
		{Title: "Redis 持久化机制", Content: "RDB 快照和 AOF 追加日志各有权衡", Author: "carol", Tags: []string{"redis", "database"}, Price: 109.0, CreatedAt: "2024-04-05"},
		{Title: "Kafka 消息队列", Content: "Partition 是 Kafka 并行度的核心", Author: "bob", Tags: []string{"kafka", "mq"}, Price: 119.0, CreatedAt: "2024-05-18"},
	}

	for i, doc := range docs {
		id := fmt.Sprintf("%d", i+1)
		body, _ := json.Marshal(doc)
		res, err := esClient.Index(articlesIndex, bytes.NewReader(body),
			esClient.Index.WithDocumentID(id),
			esClient.Index.WithRefresh("true"), // 立即 refresh，实验用；生产不要这样
		)
		if err != nil || res.IsError() {
			log.Fatalf("写入文档 %s 失败: %v", id, err)
		}
		res.Body.Close()
	}
	fmt.Printf("写入 %d 条文档\n", len(docs))

	// --- 查询单条 ---
	res, _ := esClient.Get(articlesIndex, "1")
	defer res.Body.Close()
	var getResult map[string]interface{}
	json.NewDecoder(res.Body).Decode(&getResult)
	src := getResult["_source"].(map[string]interface{})
	fmt.Printf("GET id=1: title=%q author=%q\n", src["title"], src["author"])

	// --- 更新（partial update）---
	// _update 只需传要改的字段，ES 内部合并后生成新文档版本
	updateBody := `{"doc": {"price": 88.0}}`
	res2, _ := esClient.Update(articlesIndex, "1", strings.NewReader(updateBody),
		esClient.Update.WithRefresh("true"),
	)
	res2.Body.Close()

	// 验证更新
	res3, _ := esClient.Get(articlesIndex, "1")
	defer res3.Body.Close()
	var updated map[string]interface{}
	json.NewDecoder(res3.Body).Decode(&updated)
	updatedSrc := updated["_source"].(map[string]interface{})
	fmt.Printf("UPDATE id=1 price → %.1f（版本号: %v）\n", updatedSrc["price"], updated["_version"])

	// --- 删除 ---
	res4, _ := esClient.Delete(articlesIndex, "5", esClient.Delete.WithRefresh("true"))
	res4.Body.Close()
	res5, _ := esClient.Get(articlesIndex, "5")
	defer res5.Body.Close()
	fmt.Printf("DELETE id=5: found=%v（应为 false）\n\n", res5.StatusCode != 404)
}

// ExpDynamicMappingTrap 演示 Dynamic Mapping 的自动推断陷阱。
//
// 陷阱场景：订单号是纯数字字符串（如 "10086"），ES 自动推断为 long，
// 之后想做前缀查询或精确字符串匹配就悲剧了。
func ExpDynamicMappingTrap() {
	fmt.Println("=== 实验3: Dynamic Mapping 陷阱 ===")

	// 删索引，让它自动推断
	esClient.Indices.Delete([]string{dynamicIndex})

	// 写入第一条，ES 自动推断类型
	firstDoc := `{"order_id": "10086", "amount": 99.9, "status": "paid"}`
	res, _ := esClient.Index(dynamicIndex, strings.NewReader(firstDoc),
		esClient.Index.WithRefresh("true"),
	)
	res.Body.Close()

	// 查看推断出的 Mapping
	mappingRes, _ := esClient.Indices.GetMapping(esClient.Indices.GetMapping.WithIndex(dynamicIndex))
	defer mappingRes.Body.Close()
	var mappingResult map[string]interface{}
	json.NewDecoder(mappingRes.Body).Decode(&mappingResult)

	indexMapping := mappingResult[dynamicIndex].(map[string]interface{})
	mappings := indexMapping["mappings"].(map[string]interface{})
	props := mappings["properties"].(map[string]interface{})

	fmt.Println("Dynamic Mapping 自动推断结果：")
	for field, info := range props {
		fieldInfo := info.(map[string]interface{})
		fmt.Printf("  %-12s → type: %v\n", field, fieldInfo["type"])
	}

	fmt.Println()
	fmt.Println("陷阱分析：")
	orderIdMapping := props["order_id"].(map[string]interface{})
	orderIdType := orderIdMapping["type"]
	fmt.Printf("  order_id 被推断为: %v\n", orderIdType)
	if orderIdType == "long" {
		fmt.Println("  ✗ 推断为 long！想用 prefix query 或精确字符串匹配 → 失败")
		fmt.Println("  ✗ 且 Mapping 已固化，后续无法改回 keyword，只能 reindex")
	} else {
		fmt.Printf("  本次 ES 版本推断为 %v（不同版本行为可能不同）\n", orderIdType)
	}
	fmt.Println("  → 生产建议: dynamic=strict，手动定义所有字段类型\n")

	esClient.Indices.Delete([]string{dynamicIndex})
}

func readBody(body io.ReadCloser) string {
	if body == nil {
		return ""
	}
	defer body.Close()
	b, _ := io.ReadAll(body)
	return string(b)
}
