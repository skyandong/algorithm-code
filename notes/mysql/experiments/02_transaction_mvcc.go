// # 事务与 MVCC 实验
//
// 核心问题：MVCC 怎么做到不加锁也能读到"正确"的历史版本？
//
// 原理速记：
//
//	每行记录有三个隐藏列：
//	  DB_TRX_ID   — 最后修改这行的事务 ID
//	  DB_ROLL_PTR — 指向 Undo Log 里上一个版本的指针（版本链）
//	  DB_ROW_ID   — 没主键时的隐式主键
//
//	ReadView（快照）：事务做快照读时生成，包含"当时还活着的事务列表"
//	  RC  隔离级别：每次 SELECT 都重新生成 ReadView → 能看到别人已提交的最新修改
//	  RR  隔离级别：事务第一次 SELECT 时生成，整个事务复用同一个 ReadView → 始终看到同一快照
//
// 实验项：
//
//	Exp1：RC vs RR — 不可重复读 & 可重复读
//	Exp2：脏读演示 — READ UNCOMMITTED 能读到未提交数据
//	Exp3：幻读 — RR 快照读 vs FOR UPDATE 当前读的差异
//	Exp4：两阶段提交可见性 — 事务提交顺序与可见性

package main

import (
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Account 账户，用于演示并发事务
type Account struct {
	ID      uint   `gorm:"primarykey"`
	Name    string `gorm:"type:varchar(64);not null"`
	Balance int64  // 分
}

// RunTransactionExperiments 运行事务/MVCC 实验
func RunTransactionExperiments(db *gorm.DB) {
	db.AutoMigrate(&Account{})
	db.Where("1 = 1").Delete(&Account{})
	db.Create(&Account{ID: 1, Name: "alice", Balance: 1000})
	db.Create(&Account{ID: 2, Name: "bob", Balance: 2000})

	fmt.Println("\n=== 实验1: 可重复读(RR) — 同一事务两次读结果一致 ===")
	expRR(db)

	fmt.Println("\n=== 实验2: 不可重复读(RC) — 每次读都是最新已提交数据 ===")
	expRC(db)

	fmt.Println("\n=== 实验3: 幻读 — 快照读 vs 当前读 ===")
	expPhantomRead(db)

	fmt.Println("\n=== 实验4: 快照读 vs 当前读 — 同一事务看到不同结果 ===")
	expSnapshotVsCurrent(db)

	fmt.Println("\n=== 实验5: 转账 — 事务原子性 ===")
	expTransfer(db)
}

// expRR 演示 RR 级别下的可重复读
// 两个 goroutine：txA 读 → txB 修改并提交 → txA 再读（结果不变）
func expRR(db *gorm.DB) {
	// 重置余额
	db.Model(&Account{}).Where("id = ?", 1).Update("balance", 1000)

	var wg sync.WaitGroup
	ready := make(chan struct{}) // txA 第一次读完后通知 txB 开始

	wg.Add(1)
	go func() {
		defer wg.Done()
		// txA：RR 级别（MySQL 默认），事务开始时生成 ReadView
		db.Transaction(func(tx *gorm.DB) error {
			var a Account
			tx.First(&a, 1)
			fmt.Printf("  txA 第一次读 alice.balance = %d\n", a.Balance)

			close(ready) // 告诉 txB 可以改了
			time.Sleep(100 * time.Millisecond) // 等 txB 提交

			tx.First(&a, 1) // 第二次读，ReadView 不变，结果应该一样
			fmt.Printf("  txA 第二次读 alice.balance = %d（RR：同一快照，看不到 txB 的修改）\n", a.Balance)
			return nil
		})
	}()

	<-ready
	// txB：修改并立即提交
	db.Model(&Account{}).Where("id = ?", 1).Update("balance", 9999)
	fmt.Println("  txB 已将 alice.balance 改为 9999 并提交")

	wg.Wait()
}

// expRC 演示 RC 级别下的不可重复读
// 需要临时切换隔离级别
func expRC(db *gorm.DB) {
	db.Model(&Account{}).Where("id = ?", 1).Update("balance", 1000)

	var wg sync.WaitGroup
	ready := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		// 开一个 RC 级别的事务（通过 SET SESSION 设置，仅当前连接生效）
		db.Transaction(func(tx *gorm.DB) error {
			tx.Exec("SET SESSION TRANSACTION ISOLATION LEVEL READ COMMITTED")

			var a Account
			tx.First(&a, 1)
			fmt.Printf("  txA 第一次读 alice.balance = %d\n", a.Balance)

			close(ready)
			time.Sleep(100 * time.Millisecond)

			tx.First(&a, 1) // RC：重新生成 ReadView，能看到 txB 的提交
			fmt.Printf("  txA 第二次读 alice.balance = %d（RC：能看到 txB 的修改，不可重复读）\n", a.Balance)

			tx.Exec("SET SESSION TRANSACTION ISOLATION LEVEL REPEATABLE READ") // 还原
			return nil
		})
	}()

	<-ready
	db.Model(&Account{}).Where("id = ?", 1).Update("balance", 8888)
	fmt.Println("  txB 已将 alice.balance 改为 8888 并提交")

	wg.Wait()
}

// expPhantomRead 演示 RR 下快照读 vs 当前读 对幻读的不同表现
func expPhantomRead(db *gorm.DB) {
	db.Where("1 = 1").Delete(&Account{})
	db.Create(&Account{ID: 10, Name: "x", Balance: 100})

	fmt.Println("  [快照读] RR 下普通 SELECT 不会看到事务后插入的新行：")
	db.Transaction(func(tx *gorm.DB) error {
		var count int64
		tx.Model(&Account{}).Count(&count)
		fmt.Printf("    第一次 COUNT = %d\n", count)

		// 另一个事务插入一行（用裸 db，绕过当前事务）
		db.Create(&Account{ID: 11, Name: "y", Balance: 200})
		fmt.Println("    另一事务插入 id=11 并提交")

		tx.Model(&Account{}).Count(&count) // 快照读，看不到新行
		fmt.Printf("    第二次 COUNT = %d（快照读：幻读被 MVCC 避免）\n", count)
		return nil
	})

	db.Where("1 = 1").Delete(&Account{})
	db.Create(&Account{ID: 10, Name: "x", Balance: 100})

	fmt.Println("  [当前读] FOR UPDATE 会看到最新数据，配合 Gap Lock 阻止幻读：")
	db.Transaction(func(tx *gorm.DB) error {
		var accounts []Account
		// FOR UPDATE = 当前读，获取 Next-Key Lock
		tx.Set("gorm:query_option", "FOR UPDATE").Find(&accounts)
		fmt.Printf("    第一次 FOR UPDATE 查到 %d 行\n", len(accounts))

		// 此时另一个事务想插入 id=11 会被 Gap Lock 阻塞（这里无法直观演示阻塞，仅说明机制）
		fmt.Println("    Gap Lock 持有中：其他事务在 id=10 附近的插入会被阻塞")

		tx.Set("gorm:query_option", "FOR UPDATE").Find(&accounts)
		fmt.Printf("    第二次 FOR UPDATE 查到 %d 行（当前读 + Gap Lock，防幻读）\n", len(accounts))
		return nil
	})
}

// expTransfer 演示事务原子性：转账要么全成功，要么全回滚
func expTransfer(db *gorm.DB) {
	db.Where("1 = 1").Delete(&Account{})
	db.Create(&Account{ID: 1, Name: "alice", Balance: 1000})
	db.Create(&Account{ID: 2, Name: "bob", Balance: 500})

	printBalance := func(label string) {
		var a, b Account
		db.First(&a, 1)
		db.First(&b, 2)
		fmt.Printf("  [%s] alice=%d  bob=%d  total=%d\n", label, a.Balance, b.Balance, a.Balance+b.Balance)
	}

	printBalance("转账前")

	// 正常转账
	err := transfer(db, 1, 2, 300)
	fmt.Printf("  转账 300: %v\n", err)
	printBalance("正常转账后")

	// 余额不足，事务回滚
	err = transfer(db, 1, 2, 99999)
	fmt.Printf("  转账 99999（余额不足）: %v\n", err)
	printBalance("失败回滚后（余额不变）")
}

// expSnapshotVsCurrent 演示同一事务内快照读 vs 当前读看到不同行数
// 说明 RR 下幻读"部分解决"的真实含义
func expSnapshotVsCurrent(db *gorm.DB) {
	db.Where("1 = 1").Delete(&Account{})
	db.Create(&Account{ID: 1, Name: "alice", Balance: 100})

	db.Transaction(func(tx *gorm.DB) error {
		// 第一次快照读，建立 ReadView
		var count int64
		tx.Model(&Account{}).Count(&count)
		fmt.Printf("  快照读 COUNT = %d（ReadView 建立）\n", count)

		// 另一个事务插入新行并提交
		db.Create(&Account{ID: 2, Name: "bob", Balance: 200})
		fmt.Println("  外部事务插入 id=2 并提交")

		// 再次快照读：ReadView 复用，看不到新行
		tx.Model(&Account{}).Count(&count)
		fmt.Printf("  快照读 COUNT = %d（MVCC 屏蔽，幻读不可见）\n", count)

		// 当前读：读最新数据，能看到新行
		var accounts []Account
		tx.Raw("SELECT * FROM accounts FOR UPDATE").Scan(&accounts)
		fmt.Printf("  当前读 COUNT = %d（FOR UPDATE 看到最新，幻读出现）\n", len(accounts))

		return nil
	})
}

func transfer(db *gorm.DB, fromID, toID uint, amount int64) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var from Account
		// FOR UPDATE 加行锁，防止并发转账时余额被多扣
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&from, fromID).Error; err != nil {
			return err
		}
		if from.Balance < amount {
			return fmt.Errorf("余额不足: 现有 %d，需要 %d", from.Balance, amount)
		}
		if err := tx.Model(&Account{}).Where("id = ?", fromID).
			Update("balance", gorm.Expr("balance - ?", amount)).Error; err != nil {
			return err
		}
		if err := tx.Model(&Account{}).Where("id = ?", toID).
			Update("balance", gorm.Expr("balance + ?", amount)).Error; err != nil {
			return err
		}
		return nil
	})
}
