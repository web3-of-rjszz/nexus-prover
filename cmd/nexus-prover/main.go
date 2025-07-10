package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"nexus-prover/internal/config"
	"nexus-prover/internal/utils"
	"nexus-prover/internal/worker"
	"nexus-prover/pkg/types"
)

func main() {
	// 检查是否运行进程worker模式
	if len(os.Args) > 1 && os.Args[1] == "--prove" {
		worker.RunProcessWorker()
		return
	}

	// 定义命令行参数
	configPath := flag.String("c", "config.json", "配置文件路径 (默认: config.json)")
	configPathLong := flag.String("config", "config.json", "配置文件路径 (默认: config.json)")
	processIsolation := flag.Bool("ps", false, "启用进程隔离模式（使用官方zkVM生成proof）")
	processIsolationLong := flag.Bool("process-isolation", false, "启用进程隔离模式（使用官方zkVM生成proof）")
	showHelp := flag.Bool("h", false, "显示帮助信息")
	showHelpLong := flag.Bool("help", false, "显示帮助信息")
	showVersion := flag.Bool("v", false, "显示版本信息")
	showVersionLong := flag.Bool("version", false, "显示版本信息")
	flag.Parse()

	// 帮助信息
	if *showHelp || *showHelpLong {
		printHelp()
		return
	}

	// 版本信息
	if *showVersion || *showVersionLong {
		printVersion()
		return
	}

	printVersion()

	// 选择配置文件参数（优先--config）
	cfgFile := "config.json"
	if *configPathLong != "config.json" {
		cfgFile = *configPathLong
	} else if *configPath != "config.json" {
		cfgFile = *configPath
	}

	// 检查配置文件是否存在
	if _, err := os.Stat(cfgFile); os.IsNotExist(err) {
		log.Fatalf("❌ 配置文件不存在: %s", cfgFile)
	}

	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		log.Fatalf("❌ 加载配置文件失败: %v", err)
	}

	// 验证配置
	if len(cfg.NodeIDs) == 0 {
		log.Fatal("配置错误: node_ids 数组不能为空")
	}

	utils.LogWithTime("📋 配置信息:")
	utils.LogWithTime("   配置文件: %s", cfgFile)
	utils.LogWithTime("   节点IDs: %v", cfg.NodeIDs)
	utils.LogWithTime("   用户ID: %s", cfg.UserID)
	utils.LogWithTime("   钱包地址: %s", cfg.WalletAddress)
	utils.LogWithTime("   请求间隔: %d 秒", cfg.RequestDelay)
	utils.LogWithTime("   证明计算worker数量: %d", cfg.ProverWorkers)
	utils.LogWithTime("   节点数量: %d", len(cfg.NodeIDs))
	utils.LogWithTime("   🆕 任务队列调度模式")
	utils.LogWithTime("   🆕 队列容量: %d", cfg.TaskQueueCapacity)
	utils.LogWithTime("   🆕 固定180秒间隔获取任务")
	utils.LogWithTime("   🆕 优先获取已分配任务")
	utils.LogWithTime("   🆕 内存优化: 提交成功后立即释放证明数据")
	utils.LogWithTime("   按 Ctrl+C 优雅停止程序")

	// 显示初始内存使用情况
	utils.LogWithTime("💾 初始进程物理内存: %.2fMB", utils.GetProcMemUsage())

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var acceptingTasks int32 = 1

	// 创建任务队列
	taskQueue := types.NewTaskQueue(cfg.TaskQueueCapacity, 100)
	utils.LogWithTime("📦 任务队列已创建 (容量: %d), 提交失败重试队列容量: %d", cfg.TaskQueueCapacity, 100)

	utils.LogWithTime("🔄 防止任务获取限速, 等待3分钟...")
	utils.SleepWithContext(ctx, time.Duration(3)*time.Minute) // 为防止任务获取限速，让worker等待3分钟

	// 启动任务获取worker
	wg.Add(1)
	go worker.TaskFetcher(ctx, cfg.NodeIDs, pub, taskQueue, cfg.RequestDelay, &wg, &acceptingTasks)

	// 检查是否使用进程隔离模式
	useProcessIsolation := *processIsolation || *processIsolationLong
	if useProcessIsolation {
		// 使用进程隔离模式
		utils.LogWithTime("🔄 启用进程隔离模式")

		// 获取当前可执行文件路径
		execPath, err := os.Executable()
		if err != nil {
			log.Fatalf("无法获取可执行文件路径: %v", err)
		}

		// 创建进程证明器
		prover := worker.NewProcessProver(execPath, 300, 3) // 5分钟超时，最多3次重启

		// 启动进程隔离的证明计算worker池
		for i := 0; i < cfg.ProverWorkers; i++ {
			wg.Add(1)
			utils.LogWithTime("🔧 启动进程隔离证明计算worker-%d", i)
			go func(workerID int) {
				worker.ProcessWorker(ctx, workerID, priv, taskQueue, &wg, prover)
			}(i)
		}
	} else {
		// 使用普通模式
		utils.LogWithTime("🔧 启用普通模式")

		// 启动证明计算worker池
		for i := 0; i < cfg.ProverWorkers; i++ {
			wg.Add(1)
			utils.LogWithTime("🔧 启动证明计算worker-%d", i)
			go func(workerID int) {
				worker.ProverWorker(ctx, workerID, priv, taskQueue, &wg)
			}(i)
		}
	}

	// 启动重试worker：
	wg.Add(1)
	go worker.RetryWorker(ctx, taskQueue, priv, &wg)

	// 启动周期统计goroutine
	utils.LogWithTime("📊 启动周期统计 (间隔: %d秒)", worker.STATS_INTERVAL)
	go worker.PeriodicStats(ctx, taskQueue)

	// 控制useLocal
	useLocal := !useProcessIsolation

	if useLocal {
		utils.LogWithTime("⚡ 当前使用Go本地算法生成proof，仅用于本地校验/性能测试，提交到服务端会422！")
	} else {
		utils.LogWithTime("✅ 当前使用官方zkVM生成proof，可提交到服务端验证。")
	}

	// 设置信号处理
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	utils.LogWithTime("🚀 程序已启动，等待任务...")

	sig := <-c // 等待信号
	utils.LogWithTime("🛑 收到信号 %v，正在优雅关闭...", sig)
	cancel() // 取消上下文，通知所有goroutine停止
	utils.LogWithTime("📢 已发送停止信号给所有goroutine")
	atomic.StoreInt32(&acceptingTasks, 0) // 停止获取新任务
	done := make(chan struct{})           // 等待所有 worker 完成，但设置超时为3分钟
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		utils.LogWithTime("✅ 所有 worker 已优雅关闭")
	case <-time.After(3 * time.Minute):
		utils.LogWithTime("⚠️  等待超时（3分钟），强制退出")
	}
	utils.LogWithTime("👋 程序已退出")

	// 在MainEntry退出前输出统计
	fetched, proved, submitted := worker.GetStats()
	utils.LogWithTime("📊 全局统计 - 获取: %d, 证明: %d, 提交: %d", fetched, proved, submitted)

	// 输出队列统计
	queued, processed, failed := taskQueue.GetStats()
	utils.LogWithTime("📦 队列统计 - 队列中: %d, 已处理: %d, 失败: %d", queued, processed, failed)

	// 显示最终内存使用情况
	utils.LogWithTime("💾 最终进程物理内存: %.2fMB", utils.GetProcMemUsage())
}

func printHelp() {
	fmt.Println("Nexus Prover CLI (进程隔离/普通模式)")
	fmt.Println("")
	fmt.Println("用法:")
	fmt.Println("  ./nexus-prover [-c 配置文件] [-ps]")
	fmt.Println("")
	fmt.Println("参数:")
	fmt.Println("  -c, --config <文件>        # 指定配置文件 (默认: config.json)")
	fmt.Println("  -ps, --process-isolation   # 启用进程隔离模式, 不加-ps参数则默认使用普通模式")
	fmt.Println("  -h, --help                 # 显示帮助信息")
	fmt.Println("  -v, --version              # 显示版本信息")
	fmt.Println("")
	fmt.Println("示例:")
	fmt.Println("  ./nexus-prover             # 普通模式(生成证明速度更快，内存占用固定非常低，可以无限跑)")
	fmt.Println("  ./nexus-prover -ps         # 进程隔离模式(怕女巫的推荐使用官方zkVM生成proof)")
	fmt.Println("  ./nexus-prover -c myconfig.json -ps")
	fmt.Println("配置文件格式:")
	fmt.Println("  {")
	fmt.Println("    \"node_ids\": [\"节点ID1\", \"节点ID2\"],")
	fmt.Println("    \"user_id\": \"用户ID\",                # 可以不填")
	fmt.Println("    \"wallet_address\": \"钱包地址\",       # 可以不填")
	fmt.Println("    \"request_delay\": 0,")
	fmt.Println("    \"prover_workers\": 9,")
	fmt.Println("    \"task_queue_capacity\": 1000")
	fmt.Println("  }")
	fmt.Println("")
}

func printVersion() {
	fmt.Println("Nexus Prover CLI v1.0.4 (进程隔离/普通模式)")
}
