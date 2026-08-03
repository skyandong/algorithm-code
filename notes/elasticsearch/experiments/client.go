package experiments

import (
	"log"

	"github.com/elastic/go-elasticsearch/v8"
)

var ES *elasticsearch.Client

func Init() {
	var err error
	ES, err = elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{"http://localhost:9200"},
	})
	if err != nil {
		log.Fatalf("创建 ES 客户端失败: %v", err)
	}

	res, err := ES.Ping()
	if err != nil || res.IsError() {
		log.Fatalf("连不上 ES，请先执行: docker compose up -d\n错误: %v", err)
	}
	res.Body.Close()
}
