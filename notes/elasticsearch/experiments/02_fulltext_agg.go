// # 实验二：全文检索与聚合
//
// 核心认知：
//   - Query Context（must）算相关性分，Filter Context（filter）只判断是否匹配且可缓存
//   - match 查询会先分词，term 不分词；对 text 字段用 term 经常查不到东西
//   - 聚合要求字段是 keyword 或 numeric，text 字段默认不支持聚合
//
// 本实验演示：
//   1. Query vs Filter 性能对比（多次查询，观察 filter 因缓存而加速）
//   2. match vs term 在 text 字段的结果差异（经典陷阱）
//   3. multi_match 跨字段搜索 + boost 权重调整
//   4. terms 聚合 + avg price 子聚合
//
// 对应笔记：03-查询DSL与相关性.md

package experiments

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// ExpQueryVsFilter 对比 Query Context（must）和 Filter Context（filter）的性能差异。
//
// Filter 为什么快：
//  1. 不计算 _score，省去 BM25 计算
//  2. 结果以 Bitset 形式缓存在内存，相同条件第二次查直接读缓存
//
// 实验方法：同一条件分别用 must 和 filter 查 N 次，观察耗时差异（第二次起 filter 应明显更快）
func ExpQueryVsFilter() {
	fmt.Println("=== 实验4: Query vs Filter 性能对比 ===")

	const rounds = 5

	mustQuery := `{
	  "query": {
	    "bool": {
	      "must": [
	        { "term": { "author": "alice" } }
	      ]
	    }
	  }
	}`

	filterQuery := `{
	  "query": {
	    "bool": {
	      "filter": [
	        { "term": { "author": "alice" } }
	      ]
	    }
	  }
	}`

	fmt.Printf("%-10s  %-12s  %-12s\n", "轮次", "must(Query)", "filter(Filter)")
	for i := 0; i < rounds; i++ {
		t1 := time.Now()
		res, err := ES.Search(
			ES.Search.WithIndex(articlesIndex),
			ES.Search.WithBody(strings.NewReader(mustQuery)),
		)
		if err != nil || res.IsError() {
			log.Fatalf("must 查询失败: %v", err)
		}
		res.Body.Close()
		mustDur := time.Since(t1)

		t2 := time.Now()
		res2, err := ES.Search(
			ES.Search.WithIndex(articlesIndex),
			ES.Search.WithBody(strings.NewReader(filterQuery)),
		)
		if err != nil || res2.IsError() {
			log.Fatalf("filter 查询失败: %v", err)
		}
		res2.Body.Close()
		filterDur := time.Since(t2)

		fmt.Printf("  第%d轮     %-12v  %-12v\n", i+1, mustDur, filterDur)
	}

	fmt.Println()
	fmt.Println("结论：filter 首次查询建立 Bitset 缓存，后续命中缓存速度更快")
	fmt.Println("      精确匹配、范围过滤场景下，优先用 filter 而非 must\n")
}

// ExpMatchVsTerm 演示 match 和 term 在 text 字段上的经典陷阱。
//
// 陷阱：
//   - text 字段在索引时会被分词："Elasticsearch 入门指南" → ["elasticsearch", "入门", "指南"]
//   - term 查询不分词，直接查 "Elasticsearch 入门指南" 这个词项 → 找不到
//   - match 查询先分词，拆成各个词项再查 → 能找到
//
// 反例（keyword 字段）：
//   - keyword 不分词，term 精确匹配正常工作
//   - keyword 字段用 match 也行，但 match 会把查询词分词，可能产生意外结果
func ExpMatchVsTerm() {
	fmt.Println("=== 实验5: match vs term 陷阱 ===")

	type queryCase struct {
		name  string
		query string
	}

	cases := []queryCase{
		{
			name: `term 查 text 字段（完整标题，应查不到）`,
			query: `{
			  "query": { "term": { "title": "Elasticsearch 入门指南" } }
			}`,
		},
		{
			name: `term 查 text 字段（分词后的单词，能查到）`,
			query: `{
			  "query": { "term": { "title": "elasticsearch" } }
			}`,
		},
		{
			name: `match 查 text 字段（完整标题，ES 自动分词后查，能查到）`,
			query: `{
			  "query": { "match": { "title": "Elasticsearch 入门指南" } }
			}`,
		},
		{
			name: `term 查 keyword 字段（精确匹配，正确用法）`,
			query: `{
			  "query": { "term": { "author": "alice" } }
			}`,
		},
		{
			name: `term 查 title.keyword（title 的 keyword subfield，精确全文匹配）`,
			query: `{
			  "query": { "term": { "title.keyword": "Elasticsearch 入门指南" } }
			}`,
		},
	}

	for _, c := range cases {
		res, err := ES.Search(
			ES.Search.WithIndex(articlesIndex),
			ES.Search.WithBody(strings.NewReader(c.query)),
		)
		if err != nil || res.IsError() {
			fmt.Printf("  查询失败: %v\n", err)
			continue
		}
		var result map[string]interface{}
		json.NewDecoder(res.Body).Decode(&result)
		res.Body.Close()

		hits := result["hits"].(map[string]interface{})
		total := hits["total"].(map[string]interface{})
		count := total["value"]
		fmt.Printf("  命中 %v 条 ← %s\n", count, c.name)
	}

	fmt.Println()
	fmt.Println("规律：text 字段 → 用 match；keyword 字段 → 用 term")
	fmt.Println("      想对 text 字段做精确匹配：用 .keyword subfield + term\n")
}

// ExpMultiMatch 演示跨字段搜索和 boost 权重调整。
//
// multi_match 的 best_fields 模式：取得分最高的那个字段的分作为最终分。
// 用 ^ 符号提升字段权重：title^2 表示 title 字段的命中权重是 content 的 2 倍。
func ExpMultiMatch() {
	fmt.Println("=== 实验6: multi_match 跨字段搜索 ===")

	query := `{
	  "query": {
	    "multi_match": {
	      "query": "Elasticsearch",
	      "fields": ["title^2", "content"],
	      "type": "best_fields"
	    }
	  },
	  "_source": ["title", "author"],
	  "size": 3
	}`

	res, err := ES.Search(
		ES.Search.WithIndex(articlesIndex),
		ES.Search.WithBody(strings.NewReader(query)),
	)
	if err != nil || res.IsError() {
		log.Fatalf("multi_match 查询失败: %v", err)
	}
	defer res.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(res.Body).Decode(&result)

	hits := result["hits"].(map[string]interface{})["hits"].([]interface{})
	fmt.Println("搜索 'Elasticsearch'，fields=[title^2, content]：")
	for _, h := range hits {
		hit := h.(map[string]interface{})
		src := hit["_source"].(map[string]interface{})
		fmt.Printf("  _score=%.4f  title=%q\n", hit["_score"], src["title"])
	}
	fmt.Println("  title 权重是 content 的 2 倍，title 命中的文档排名更靠前\n")
}

// ExpAggregation 演示 terms 聚合 + metric 子聚合。
//
// 类比 SQL：
//   SELECT author, COUNT(*), AVG(price) FROM articles GROUP BY author ORDER BY COUNT(*) DESC
//
// ES 聚合要求字段必须是 keyword 或 numeric，text 字段默认不能聚合。
func ExpAggregation() {
	fmt.Println("=== 实验7: 聚合（terms + avg）===")

	query := `{
	  "size": 0,
	  "aggs": {
	    "by_author": {
	      "terms": {
	        "field": "author",
	        "size": 10
	      },
	      "aggs": {
	        "avg_price": {
	          "avg": { "field": "price" }
	        },
	        "max_price": {
	          "max": { "field": "price" }
	        }
	      }
	    }
	  }
	}`

	res, err := ES.Search(
		ES.Search.WithIndex(articlesIndex),
		ES.Search.WithBody(strings.NewReader(query)),
	)
	if err != nil || res.IsError() {
		log.Fatalf("聚合查询失败: %v", err)
	}
	defer res.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(res.Body).Decode(&result)

	aggs := result["aggregations"].(map[string]interface{})
	byAuthor := aggs["by_author"].(map[string]interface{})
	buckets := byAuthor["buckets"].([]interface{})

	fmt.Printf("  %-10s  %-8s  %-10s  %-10s\n", "author", "文章数", "均价", "最高价")
	for _, b := range buckets {
		bucket := b.(map[string]interface{})
		author := bucket["key"].(string)
		count := bucket["doc_count"].(float64)
		avgPrice := bucket["avg_price"].(map[string]interface{})["value"].(float64)
		maxPrice := bucket["max_price"].(map[string]interface{})["value"].(float64)
		fmt.Printf("  %-10s  %-8.0f  %-10.1f  %-10.1f\n", author, count, avgPrice, maxPrice)
	}

	fmt.Println()
	fmt.Println("等价 SQL: SELECT author, COUNT(*), AVG(price), MAX(price) FROM articles GROUP BY author")
	fmt.Println("          聚合字段必须是 keyword 或 numeric，text 字段不支持\n")
}

// ExpBoolQuery 演示 bool 查询的四个子句组合。
func ExpBoolQuery() {
	fmt.Println("=== 实验8: bool 查询组合 ===")

	// must + filter + should
	// 找：(标题含 "Golang" 或 "MySQL") 且 作者是 alice 或 bob 且 价格 80~130
	query := `{
	  "query": {
	    "bool": {
	      "should": [
	        { "match": { "title": "Golang" } },
	        { "match": { "title": "MySQL" } }
	      ],
	      "minimum_should_match": 1,
	      "filter": [
	        { "terms": { "author": ["alice", "bob"] } },
	        { "range": { "price": { "gte": 80, "lte": 130 } } }
	      ]
	    }
	  },
	  "_source": ["title", "author", "price"],
	  "sort": [{ "_score": "desc" }]
	}`

	res, err := ES.Search(
		ES.Search.WithIndex(articlesIndex),
		ES.Search.WithBody(strings.NewReader(query)),
	)
	if err != nil || res.IsError() {
		body := new(bytes.Buffer)
		body.ReadFrom(res.Body)
		log.Fatalf("bool 查询失败: %v %s", err, body.String())
	}
	defer res.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(res.Body).Decode(&result)

	hits := result["hits"].(map[string]interface{})["hits"].([]interface{})
	fmt.Println("条件：(标题含 Golang 或 MySQL) + 作者 in [alice,bob] + 价格 80~130")
	for _, h := range hits {
		hit := h.(map[string]interface{})
		src := hit["_source"].(map[string]interface{})
		fmt.Printf("  _score=%.4f  title=%-25q  author=%-8s  price=%.1f\n",
			hit["_score"], src["title"], src["author"], src["price"])
	}

	fmt.Println()
	fmt.Println("要点：should + minimum_should_match 实现 OR 逻辑；filter 不影响评分且可缓存\n")
}
