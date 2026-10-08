// Package seed 存放题库。题目按知识点组织，每个知识点一个文件。
// 题目由人工（AI）依据仓库 notes/ 下的笔记编写，标准答案与解析随题入库，
// 批改由服务端完成，运行期不依赖任何外部大模型。
package seed

import (
	"fmt"

	"quiz/internal/model"
)

// DefaultDifficulty 是题目默认难度，1 易 / 2 中 / 3 难。
const DefaultDifficulty = 2

// topic 描述一个知识点：它对应仓库里的哪篇笔记。
type topic struct {
	module string
	slug   string
	name   string
	source string
}

// builder 返回一个出题函数，内部维护序号，避免手写 QID 出错。
func (t topic) builder() func(stem string, options []string, answer int, explanation string) model.Question {
	seq := 0
	return func(stem string, options []string, answer int, explanation string) model.Question {
		seq++
		return model.Question{
			QID:         fmt.Sprintf("%s-%02d", t.slug, seq),
			Module:      t.module,
			TopicSlug:   t.slug,
			TopicName:   t.name,
			Stem:        stem,
			Options:     options,
			Answer:      answer,
			Explanation: explanation,
			Difficulty:  DefaultDifficulty,
			Source:      t.source,
		}
	}
}

// providers 登记已经写好题目的知识点。新增一个知识点时，写好对应文件后在这里追加一行。
var providers = []func() []model.Question{
	golangDataStructure, // 01 slice、string 与 map 底层
	golangInterface,     // 02 interface 与反射
	golangGenerics,      // 03 泛型
	golangMemoryModel,   // 04 并发内存模型与可见性
	golangChannel,       // 05 channel 内部与 nil 语义
	golangSync,          // 06 sync 并发原语
	golangGMP,           // 07 运行时调度器 GMP
	golangGC,            // 08 内存管理与 GC
	golangContext,       // 09 context 与错误处理
	golangPerf,          // 10 性能调优实战
	redisCache,          // redis 05 缓存穿透、击穿与雪崩
	redisLock,           // redis 08 Redis 分布式锁
	// 其余模块的知识点按同样方式补充：写好题目文件后在上面追加一行。
}

// Bank 汇总全部知识点下的题目。
func Bank() []model.Question {
	var all []model.Question
	for _, p := range providers {
		all = append(all, p()...)
	}
	return all
}
