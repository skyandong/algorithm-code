// # 锁机制实验
//
// InnoDB 锁分类：
//
//	行级锁
//	  S Lock（共享锁）：SELECT ... LOCK IN SHARE MODE，多个事务可同时持有
//	  X Lock（排他锁）：SELECT ... FOR UPDATE / INSERT / UPDATE / DELETE，独占
//	  意向锁（IS/IX）：表级，事务加行锁前先加意向锁，让表锁检测更高效
//
//	间隙锁（Gap Lock）：锁索引记录之间的"间隙"，防止幻读
//	临键锁（Next-Key Lock）= 行锁 + 间隙锁，RR 级别默认
//	自增锁（Auto-inc Lock）：表级，保证自增 ID 连续分配
//
// 实验项：
//
//	Exp1：SELECT FOR UPDATE vs LOCK IN SHARE MODE 的互斥关系
//	Exp2：死锁复现与检测
//	Exp3：间隙锁导致插入阻塞

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Stock 库存，用于演示锁
type Stock struct {
	ID       uint   `gorm:"primarykey"`
	ItemName string `gorm:"type:varchar(64);not null;uniqueIndex"`
	Quantity int64  `gorm:"not null"`
}

// RunLockExperiments 运行锁机制实验
func RunLockExperiments(db *gorm.DB) {
	db.AutoMigrate(&Stock{})
	db.Where("1 = 1").Delete(&Stock{})
	db.Create(&Stock{ID: 1, ItemName: "apple", Quantity: 100})
	db.Create(&Stock{ID: 2, ItemName: "banana", Quantity: 200})
	db.Create(&Stock{ID: 10, ItemName: "cherry", Quantity: 50}) // ID 故意跳到 10，留间隙

	fmt.Println("\n=== 实验1: FOR UPDATE(X锁) 互斥 ===")
	expXLock(db)

	fmt.Println("\n=== 实验2: 死锁复现 ===")
	expDeadlock(db)

	fmt.Println("\n=== 实验3: 间隙锁阻塞插入 ===")
	expGapLock(db)

	fmt.Println("\n=== 实验4: 无索引 UPDATE 退化为表锁 ===")
	expNoIndexLock(db)

	fmt.Println("\n=== 实验5: SKIP LOCKED 任务队列抢占 ===")
	expSkipLocked(db)
}

// expXLock 演示 X 锁互斥：第一个事务持有 FOR UPDATE，第二个事务等待
func expXLock(db *gorm.DB) {
	var wg sync.WaitGroup
	tx1Started := make(chan struct{})
	tx1Done := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Transaction(func(tx *gorm.DB) error {
			var s Stock
			tx.Set("gorm:query_option", "FOR UPDATE").First(&s, 1)
			fmt.Printf("  tx1 获得 X 锁，stock.id=1 quantity=%d\n", s.Quantity)

			close(tx1Started) // 告诉 tx2 可以尝试了
			time.Sleep(300 * time.Millisecond)
			fmt.Println("  tx1 即将释放 X 锁...")
			return nil
		})
		close(tx1Done)
	}()

	<-tx1Started
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		fmt.Println("  tx2 尝试获取同一行的 X 锁（会阻塞直到 tx1 提交）...")
		db.Transaction(func(tx *gorm.DB) error {
			var s Stock
			tx.Set("gorm:query_option", "FOR UPDATE").First(&s, 1)
			fmt.Printf("  tx2 等待了 %dms 后获得 X 锁\n", time.Since(start).Milliseconds())
			return nil
		})
	}()

	wg.Wait()
}

// expDeadlock 复现经典死锁：两个事务以相反顺序加锁
//
//	tx1: 锁 id=1 → 等 id=2
//	tx2: 锁 id=2 → 等 id=1
//
// InnoDB 检测到死锁后会回滚其中一个（通常是改动更少的那个），另一个继续执行
func expDeadlock(db *gorm.DB) {
	db.Model(&Stock{}).Where("id = ?", 1).Update("quantity", 100)
	db.Model(&Stock{}).Where("id = ?", 2).Update("quantity", 200)

	var wg sync.WaitGroup
	tx1LockA := make(chan struct{})
	tx2LockB := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := db.Transaction(func(tx *gorm.DB) error {
			var s Stock
			tx.Set("gorm:query_option", "FOR UPDATE").First(&s, 1)
			fmt.Println("  tx1 锁定 id=1，等待 tx2 锁定 id=2...")
			close(tx1LockA)

			<-tx2LockB // 等 tx2 锁住 id=2

			// 尝试锁 id=2，此时 tx2 也在等 id=1 → 死锁
			tx.Set("gorm:query_option", "FOR UPDATE").First(&s, 2)
			fmt.Println("  tx1 锁定 id=2（不应该到这里，除非另一个被回滚）")
			return nil
		})
		if err != nil {
			fmt.Printf("  tx1 被死锁回滚: %v\n", err)
		} else {
			fmt.Println("  tx1 提交成功")
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-tx1LockA // 等 tx1 锁住 id=1
		err := db.Transaction(func(tx *gorm.DB) error {
			var s Stock
			tx.Set("gorm:query_option", "FOR UPDATE").First(&s, 2)
			fmt.Println("  tx2 锁定 id=2，等待 tx1 锁定 id=1...")
			close(tx2LockB)

			// 尝试锁 id=1，此时 tx1 也在等 id=2 → 死锁
			tx.Set("gorm:query_option", "FOR UPDATE").First(&s, 1)
			fmt.Println("  tx2 锁定 id=1（不应该到这里，除非另一个被回滚）")
			return nil
		})
		if err != nil {
			fmt.Printf("  tx2 被死锁回滚: %v\n", err)
		} else {
			fmt.Println("  tx2 提交成功")
		}
	}()

	wg.Wait()
	fmt.Println("  死锁处理完毕（InnoDB 自动检测并回滚代价小的那个事务）")
	fmt.Println("  预防死锁：保证所有事务以相同顺序加锁（如始终按 id 从小到大）")
}

// Task 任务表，用于演示 SKIP LOCKED
type Task struct {
	ID     uint   `gorm:"primarykey"`
	Name   string `gorm:"type:varchar(64)"`
	Status string `gorm:"type:varchar(16);default:'pending'"` // pending / done
}

// expNoIndexLock 演示 WHERE 条件无索引时 UPDATE 退化为表锁
// item_name 有 uniqueIndex，这里用 quantity（无索引）演示
func expNoIndexLock(db *gorm.DB) {
	db.Model(&Stock{}).Where("id = ?", 1).Update("quantity", 100)

	var wg sync.WaitGroup
	tx1Ready := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Transaction(func(tx *gorm.DB) error {
			// quantity 无索引，全表扫描加 X Lock，退化为表锁
			tx.Exec("UPDATE stocks SET quantity = quantity - 1 WHERE quantity > 50")
			fmt.Println("  tx1: UPDATE WHERE quantity>50（无索引），持有全表 X Lock")
			close(tx1Ready)
			time.Sleep(300 * time.Millisecond)
			fmt.Println("  tx1 提交，释放全表锁")
			return nil
		})
	}()

	<-tx1Ready
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		fmt.Println("  tx2: 尝试更新 id=2（完全不同的行，但也被阻塞）")
		db.Transaction(func(tx *gorm.DB) error {
			tx.Model(&Stock{}).Where("id = ?", 2).Update("quantity", 999)
			fmt.Printf("  tx2: 等待了 %dms 才获得锁（无索引导致全表锁，无辜受害）\n",
				time.Since(start).Milliseconds())
			return nil
		})
	}()

	wg.Wait()
}

// expSkipLocked 演示多 worker 用 SKIP LOCKED 无阻塞抢任务
func expSkipLocked(db *gorm.DB) {
	db.AutoMigrate(&Task{})
	db.Where("1 = 1").Delete(&Task{})
	for i := 1; i <= 5; i++ {
		db.Create(&Task{ID: uint(i), Name: fmt.Sprintf("task-%d", i), Status: "pending"})
	}

	var wg sync.WaitGroup
	for workerID := 1; workerID <= 3; workerID++ {
		wg.Add(1)
		id := workerID
		go func() {
			defer wg.Done()
			db.Transaction(func(tx *gorm.DB) error {
				var task Task
				// SKIP LOCKED：跳过其他 worker 已锁定的行，立即拿到未锁定的任务
				err := tx.Raw(`SELECT * FROM tasks WHERE status='pending' ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`).
					Scan(&task).Error
				if err != nil || task.ID == 0 {
					fmt.Printf("  worker%d: 没有可用任务\n", id)
					return nil
				}
				tx.Model(&Task{}).Where("id = ?", task.ID).Update("status", "done")
				fmt.Printf("  worker%d: 抢到 %s，无需等待其他 worker\n", id, task.Name)
				time.Sleep(50 * time.Millisecond)
				return nil
			})
		}()
	}
	wg.Wait()
}

// expGapLock 演示间隙锁阻塞插入
//
// stocks 表中有 id=1,2,10。RR 级别下 FOR UPDATE 查询 id > 5 的行，
// 会对 (2, 10] 这个范围加临键锁（Next-Key Lock），导致 id=3~9 的插入被阻塞。
func expGapLock(db *gorm.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	tx1Ready := make(chan struct{})
	insertDone := make(chan error, 1)

	go func() {
		db.Transaction(func(tx *gorm.DB) error {
			var stocks []Stock
			// FOR UPDATE 查询 id > 2，对 (2, 10] 加 Next-Key Lock，(10, +∞) 加 Gap Lock
			tx.Set("gorm:query_option", "FOR UPDATE").Where("id > ?", 2).Find(&stocks)
			fmt.Printf("  tx1 FOR UPDATE id>2，持有间隙锁 (2, 10]\n")
			close(tx1Ready)

			time.Sleep(400 * time.Millisecond) // 持锁等待，让插入超时
			fmt.Println("  tx1 释放间隙锁")
			return nil
		})
	}()

	<-tx1Ready
	go func() {
		// 尝试插入 id=5，落在间隙 (2, 10) 内，会被阻塞
		err := db.WithContext(ctx).Create(&Stock{ID: 5, ItemName: "durian", Quantity: 30}).Error
		insertDone <- err
	}()

	err := <-insertDone
	if err != nil {
		fmt.Printf("  插入 id=5 被间隙锁阻塞，超时: %v\n", err)
		fmt.Println("  结论：RR 级别 FOR UPDATE 会对范围加间隙锁，防止幻读，但也会阻塞合法插入")
	} else {
		fmt.Println("  插入成功（间隙锁释放后）")
	}
}
