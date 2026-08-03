package main

import (
	"fmt"
	"os"

	"aes/experiments"
)

func main() {
	experiments.Init()

	section := ""
	if len(os.Args) > 1 {
		section = os.Args[1]
	}

	if section == "" || section == "mapping" {
		fmt.Println("========== 第一节：Mapping 与 CRUD ==========\n")
		experiments.ExpMappingCreate()
		experiments.ExpDocumentCRUD()
		experiments.ExpDynamicMappingTrap()
	}

	if section == "" || section == "fulltext" {
		fmt.Println("========== 第二节：全文检索与聚合 ==========\n")
		experiments.ExpQueryVsFilter()
		experiments.ExpMatchVsTerm()
		experiments.ExpMultiMatch()
		experiments.ExpAggregation()
		experiments.ExpBoolQuery()
	}

	if section == "" || section == "bulk" {
		fmt.Println("========== 第三节：写入优化与深分页 ==========\n")
		experiments.ExpBulkVsSingle()
		experiments.ExpRefreshInterval()
		experiments.ExpFromSizeLimit()
		experiments.ExpSearchAfter()
	}
}
