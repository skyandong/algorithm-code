package main

import (
	"fmt"
	"log"

	"github.com/elastic/go-elasticsearch/v8"
)

var esClient *elasticsearch.Client

func main() {
	var err error
	esClient, err = elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{"http://localhost:9200"},
	})
	if err != nil {
		log.Fatalf("创建 ES 客户端失败: %v", err)
	}

	res, err := esClient.Ping()
	if err != nil || res.IsError() {
		log.Fatalf("连不上 ES，请先执行: docker compose up -d\n错误: %v", err)
	}
	res.Body.Close()

	fmt.Println("========== 第一节：Mapping 与 CRUD ==========\n")
	ExpMappingCreate()
	ExpDocumentCRUD()
	ExpDynamicMappingTrap()

	fmt.Println("========== 第二节：全文检索与聚合 ==========\n")
	ExpQueryVsFilter()
	ExpMatchVsTerm()
	ExpMultiMatch()
	ExpAggregation()
	ExpBoolQuery()

	fmt.Println("========== 第三节：写入优化与深分页 ==========\n")
	ExpBulkVsSingle()
	ExpRefreshInterval()
	ExpFromSizeLimit()
	ExpSearchAfter()
}
