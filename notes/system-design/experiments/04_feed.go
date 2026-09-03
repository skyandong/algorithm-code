package main

import (
	"fmt"
	"hash/fnv"
)

// 实验 04：Feed 流推/拉/推拉结合三模式成本模拟
// 固定数据集（幂律分布的粉丝结构）, 分别统计三种模式下的:
//   - 写扩散次数（发帖时的收件箱写入）
//   - 读聚合次数（刷新时的发件箱查询）
// 锚点: 大 V 只占 0.1%, 但纯推模式下贡献 >90% 写扩散;
//       推拉结合把写扩散削减 ~99%, 代价是每次刷新多 K 次实时拉取。

// feedUser: 模拟用户
type feedUser struct {
	id          int
	followers   int // 粉丝数
	posts       int // 日发帖数
	isBigV      bool
}

func RunFeedExperiments() {
	fmt.Println("== 实验 04: 推/拉/推拉结合成本对比 ==")

	// ---- 构造幂律分布的固定数据集 ----
	// 100 万用户: 头部 1000 个大 V(10万~100万粉), 中部 1 万(1千~1万粉), 尾部 ~99 万(<100 粉)
	const totalUsers = 1_000_000
	const bigVCount = 1000
	const midCount = 10000

	users := make([]feedUser, 0, 1024)
	// 大 V: 粉丝 10万~100万, 日发 10 条
	for i := 0; i < bigVCount; i++ {
		users = append(users, feedUser{id: i, followers: 100000 + i*900, posts: 10, isBigV: true})
	}
	// 中部: 粉丝 1千~1万, 日发 3 条
	for i := 0; i < midCount; i++ {
		users = append(users, feedUser{id: bigVCount + i, followers: 1000 + i, posts: 3})
	}
	// 尾部: 粉丝 <100, 日发 0.1 条（用 1/10 的用户发 1 条近似）
	for i := 0; i < totalUsers-bigVCount-midCount; i++ {
		posts := 0
		if i%10 == 0 {
			posts = 1
		}
		users = append(users, feedUser{id: bigVCount + midCount + i, followers: i % 100, posts: posts})
	}

	// 每用户关注数: 尾部用户关注 200 人, 其中 20 个大 V、180 个普通人
	const followTotal = 200
	const followBigV = 20

	// 日刷新次数: 每用户 20 次
	const refreshesPerUser = 20

	var totalPosts, totalFollowersOfPosts int64
	for _, u := range users {
		totalPosts += int64(u.posts)
		totalFollowersOfPosts += int64(u.posts) * int64(u.followers)
	}

	fmt.Printf("数据集: %d 用户, 日发帖 %s, 日总刷新 %s\n",
		totalUsers, human(totalPosts), human(totalUsers*refreshesPerUser))
	fmt.Printf("粉丝分布: 大 V %d 人(0.1%%), 粉丝合计 %s\n\n",
		bigVCount, human(int64(bigVCount)*500000))

	// ---- 模式一: 纯推（写扩散）----
	// 写: 每条帖子 × 粉丝数 次收件箱写入
	// 读: 只读自己的收件箱 (翻页条数次 lookup, 忽略)
	pushWrites := totalFollowersOfPosts
	fmt.Println("--- 模式一: 纯推（写扩散） ---")
	fmt.Printf("  写成本: Σ(发帖×粉丝) = %s 次/天\n", human(pushWrites))
	fmt.Printf("         其中大 V 贡献: %s 次/天 (%.1f%%)\n",
		human(bigVFollowersSum(users)), float64(bigVFollowersSum(users))/float64(pushWrites)*100)
	fmt.Printf("  读成本: ~%s 次/天 (收件箱预聚合, 只翻页)\n", human(totalUsers*refreshesPerUser/10))

	// ---- 模式二: 纯拉（读扩散）----
	// 写: 每条帖子只写自己发件箱
	// 读: 每次刷新聚合所有关注人的发件箱
	pullReads := int64(totalUsers) * refreshesPerUser * followTotal
	fmt.Println("\n--- 模式二: 纯拉（读扩散） ---")
	fmt.Printf("  写成本: %s 次/天 (只写自己发件箱)\n", human(totalPosts))
	fmt.Printf("  读成本: %d 人 × %d 刷 × 关注 %d = %s 次聚合/天\n",
		totalUsers, refreshesPerUser, followTotal, human(pullReads))

	// ---- 模式三: 推拉结合 ----
	// 写: 普通用户照推, 大 V 只写发件箱;
	//     中部用户只推活跃粉丝(30%)——非活跃的等上线再拉(笔记 04 §3 的活跃度分级)
	// 读: 收件箱 + 实时拉关注的大 V 发件箱
	const activeFanRatio = 0.2
	var hybridPushWrites int64
	for _, u := range users {
		if u.isBigV {
			continue // 大 V 不推
		}
		fans := int64(u.followers)
		if u.followers >= 1000 { // 中部用户: 只推活跃粉丝
			fans = int64(float64(fans) * activeFanRatio)
		}
		hybridPushWrites += int64(u.posts) * fans
	}
	hybridPullReads := int64(totalUsers) * refreshesPerUser * followBigV
	fmt.Println("\n--- 模式三: 推拉结合（分级） ---")
	fmt.Printf("  写成本: 普通用户推 + 中部只推 %.0f%%活跃粉丝 + 大 V 不推 = %s 次/天\n",
		activeFanRatio*100, human(hybridPushWrites))
	fmt.Printf("  读成本: 收件箱翻页 + 实时拉 %d 个大 V 发件箱 × %d 刷 × %d 人 = %s 次/天\n",
		followBigV, refreshesPerUser, totalUsers, human(hybridPullReads))

	// ---- 对比汇总 ----
	fmt.Println("\n--- 对比汇总 ---")
	fmt.Printf("  写成本:  纯推 %s → 推拉结合 %s (降 %.1f%%)\n",
		human(pushWrites), human(hybridPushWrites), (1-float64(hybridPushWrites)/float64(pushWrites))*100)
	fmt.Printf("  读成本:  纯拉 %s → 推拉结合 %s (降 %.1f%%)\n",
		human(pullReads), human(hybridPullReads), (1-float64(hybridPullReads)/float64(pullReads))*100)

	// 锚点验证: 推拉结合的写扩散削减 >99%
	cut := (1 - float64(hybridPushWrites)/float64(pushWrites)) * 100
	fmt.Printf("\n锚点: 写扩散削减 %.1f%% (预期 >99%%) → %s\n", cut, mark(cut > 99))
	fmt.Printf("锚点: 大 V(0.1%%用户) 在纯推下贡献 >90%% 写扩散 → %s\n",
		mark(float64(bigVFollowersSum(users))/float64(pushWrites)*100 > 90))
}

func bigVFollowersSum(users []feedUser) int64 {
	var s int64
	for _, u := range users {
		if u.isBigV {
			s += int64(u.posts) * int64(u.followers)
		}
	}
	return s
}

// feedHashOnly: 防止 unused import（fnv 在 03 已用, 这里演示分桶 hash 的确定性）
var _ = func() bool { h := fnv.New64a(); _ = h; return true }()
