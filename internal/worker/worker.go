package worker

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nexus-prover/internal/api"
	"nexus-prover/internal/config"
	"nexus-prover/internal/utils"
	"nexus-prover/pkg/prover"
	"nexus-prover/pkg/types"
)

// 全局统计结构体
var totalFetched int64
var totalProved int64
var totalSubmitted int64

// 统计间隔时间（秒）
const STATS_INTERVAL = 60

func incFetched()   { atomic.AddInt64(&totalFetched, 1) }
func incProved()    { atomic.AddInt64(&totalProved, 1) }
func incSubmitted() { atomic.AddInt64(&totalSubmitted, 1) }

// GetStats 获取当前统计数据的副本
func GetStats() (int64, int64, int64) {
	return atomic.LoadInt64(&totalFetched),
		atomic.LoadInt64(&totalProved),
		atomic.LoadInt64(&totalSubmitted)
}

// TaskFetcher 任务获取worker - 负责从API获取任务并放入队列
func TaskFetcher(ctx context.Context, nodeIDs []string, pub ed25519.PublicKey, taskQueue *types.TaskQueue, requestDelay int, wg *sync.WaitGroup, acceptingTasks *int32) {
	defer wg.Done()
	utils.LogWithTime("[fetcher] 开始任务获取，节点数: %d", len(nodeIDs))

	// 为每个节点维护独立的状态
	states := make([]*types.TaskFetchState, len(nodeIDs))
	for i := range nodeIDs {
		states[i] = types.NewTaskFetchState()
	}

	apiClient := api.NewClient()

	for {
		shouldExit := atomic.LoadInt32(acceptingTasks) == 0
		if shouldExit {
			utils.LogWithTime("[fetcher] 停止获取新任务，准备退出")
			break
		}

		select {
		case <-ctx.Done():
			utils.LogWithTime("[fetcher] Shutting down...")
			return
		default:
			for i, nodeID := range nodeIDs {
				state := states[i]
				if state.ShouldPrintLog() {
					state.SetPrintLogTime()
				}
				if !state.ShouldFetch() {
					continue
				}
				tasks, err := apiClient.FetchTaskBatch(nodeID, pub, config.BATCH_SIZE, state)
				if err != nil {
					if utils.IsRateLimitError(err) {
						utils.LogWithTime("[fetcher@%s] ⏳ 速率限制，等待下次固定间隔获取", nodeID)
					} else if strings.Contains(err.Error(), "no task available") ||
						strings.Contains(err.Error(), "404") {
						utils.LogWithTime("[fetcher@%s] 💤 无任务可用，等待下次固定间隔获取", nodeID)
					} else {
						utils.LogWithTime("[fetcher@%s] ⚠️ 获取任务失败: %v，等待下次固定间隔获取", nodeID, err)
					}
					continue
				}
				if len(tasks) == 0 {
					continue
				}
				state.SetLastFetchTime()

				added := 0
				for _, task := range tasks {
					incFetched()
					internalTask := &types.Task{
						TaskID:       task.TaskId,
						ProgramID:    task.ProgramId,
						PublicInputs: task.PublicInputs,
						NodeID:       nodeID,
						CreatedAt:    time.Now(),
					}
					if taskQueue.AddTask(internalTask) {
						added++
					} else {
						utils.LogWithTime("[fetcher@%s] ⚠️ 队列已满，任务 %s 丢弃", nodeID, task.TaskId)
					}
				}
				if added > 0 {
					utils.LogWithTime("[fetcher@%s] 📥 成功获取并添加 %d 个任务到队列", nodeID, added)
				}
			}
			// 每轮遍历所有节点后等待requestDelay秒, 在配置文件中设置为0
			if !utils.SleepWithContext(ctx, time.Duration(requestDelay)*time.Second) {
				return
			}
		}
	}
}

// ProverWorker 证明计算worker - 从队列获取任务进行计算和提交
func ProverWorker(ctx context.Context, id int, priv ed25519.PrivateKey, taskQueue *types.TaskQueue, waitSecond int, wg *sync.WaitGroup) {
	defer wg.Done()
	utils.LogWithTime("[prover-%d] 开始证明计算", id)
	// 默认10s
	if waitSecond == 0 {
		waitSecond = 10
	}
	apiClient := api.NewClient()

	for {
		select {
		case <-ctx.Done():
			utils.LogWithTime("[prover-%d] Shutting down...", id)
			return
		default:
			// 从队列获取任务
			task, ok := taskQueue.GetTask()
			if !ok {
				// 队列为空，等待一段时间
				time.Sleep(1 * time.Second)
				continue
			}

			// 打印 PublicInputs 长度
			utils.LogWithTime("[prover-%d] 任务 %s PublicInputs 长度: %d 字节", id, task.TaskID, len(task.PublicInputs))

			// 计算证明
			proof, err := prover.Prove(task, true) // 使用go端本地算法
			if err != nil {
				utils.LogWithTime("[prover-%d] ❌ 任务 %s 证明计算失败: %v", id, task.TaskID, err)
				taskQueue.MarkFailed()
				continue
			}

			// 打印 Proof 长度
			utils.LogWithTime("[prover-%d] 任务 %s Proof 长度: %d 字节", id, task.TaskID, len(proof))

			incProved()
			taskQueue.MarkProcessed()

			// 提交证明
			utils.SleepWithContext(ctx, time.Duration(waitSecond)*time.Second) // 计算太快了，提交证明前等待8秒，避免提交过快
			err = apiClient.SubmitProof(task, proof, priv)
			if err != nil {
				if strings.Contains(err.Error(), "NotFoundError") &&
					strings.Contains(err.Error(), "Task not found") &&
					strings.Contains(err.Error(), "httpCode\":404") {
					utils.LogWithTime("❌ 任务 %s 提交失败(404 NotFound)，直接丢弃: %v", task.TaskID, err)
					// 404错误直接丢弃，清理并释放证明数据
					utils.ClearProofData(proof)
					proof = nil
				} else {
					taskQueue.AddRetry(&types.RetryProof{Task: task, Proof: proof, RetryCount: 1})
				}
			} else {
				utils.LogWithTime("[prover-%d] ✅ 任务 %s 证明提交成功", id, task.TaskID)
				incSubmitted() // 增加提交成功计数器
				// 提交成功后立即清理并释放证明数据
				utils.ClearProofData(proof)
				proof = nil
			}
		}
	}
}

// RetryWorker 重试worker - 负责从重试队列获取任务并重新提交
func RetryWorker(ctx context.Context, taskQueue *types.TaskQueue, priv ed25519.PrivateKey, wg *sync.WaitGroup) {
	defer wg.Done()
	utils.LogWithTime("🔁 启动提交重试worker")

	apiClient := api.NewClient()

	for {
		select {
		case <-ctx.Done():
			utils.LogWithTime("🔁 提交重试worker退出")
			return
		default:
			rp, ok := taskQueue.TryGetRetry()
			if !ok {
				time.Sleep(2 * time.Second)
				continue
			}
			err := apiClient.SubmitProof(rp.Task, rp.Proof, priv)
			if err != nil {
				if rp.RetryCount < 3 {
					utils.LogWithTime("🔁 重试提交失败，任务ID: %s，第%d次，放回队列: %v", rp.Task.TaskID, rp.RetryCount, err)
					rp.RetryCount++
					taskQueue.AddRetry(rp)
				} else {
					utils.LogWithTime("❌ 任务ID: %s 提交重试已达3次，丢弃此任务，最后错误: %v", rp.Task.TaskID, err)
					// 重试失败后清理并释放证明数据
					utils.ClearProofData(rp.Proof)
					rp.Proof = nil
				}
			} else {
				utils.LogWithTime("🔁 重试提交成功，任务ID: %s", rp.Task.TaskID)
				incSubmitted() // 增加提交成功计数器
				// 重试提交成功后清理并释放证明数据
				utils.ClearProofData(rp.Proof)
				rp.Proof = nil
			}
		}
	}
}

// PeriodicStats 周期统计输出函数
func PeriodicStats(ctx context.Context, taskQueue *types.TaskQueue) {
	ticker := time.NewTicker(STATS_INTERVAL * time.Second)
	defer ticker.Stop()

	lastFetched, lastProved, lastSubmitted := int64(0), int64(0), int64(0)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentFetched, currentProved, currentSubmitted := GetStats()
			queued, processed, failed := taskQueue.GetStats()

			// 计算增量
			fetchedDelta := currentFetched - lastFetched
			provedDelta := currentProved - lastProved
			submittedDelta := currentSubmitted - lastSubmitted

			// 计算速率（每分钟）
			fetchedRate := float64(fetchedDelta) / float64(STATS_INTERVAL) * 60
			provedRate := float64(provedDelta) / float64(STATS_INTERVAL) * 60
			submittedRate := float64(submittedDelta) / float64(STATS_INTERVAL) * 60

			// 计算成功率
			var successInfo string
			if currentFetched > 0 {
				proveSuccessRate := float64(currentProved) / float64(currentFetched) * 100
				submitSuccessRate := float64(currentSubmitted) / float64(currentProved) * 100
				successInfo = fmt.Sprintf(" | 成功率: 证明%.1f%%, 提交%.1f%%", proveSuccessRate, submitSuccessRate)
			}

			// 获取进程真实物理内存
			memMB := utils.GetProcMemUsage()
			memoryInfo := fmt.Sprintf(" | 进程物理内存: %.2fMB", memMB)

			utils.LogWithTime("📊 周期统计(%ds): 获取%d(+%d,%.1f/min) | 证明%d(+%d,%.1f/min) | 提交%d(+%d,%.1f/min) | 队列:%d 已处理:%d 失败:%d%s%s",
				STATS_INTERVAL,
				currentFetched, fetchedDelta, fetchedRate,
				currentProved, provedDelta, provedRate,
				currentSubmitted, submittedDelta, submittedRate,
				queued, processed, failed,
				successInfo, memoryInfo)

			// 更新上次统计值
			lastFetched, lastProved, lastSubmitted = currentFetched, currentProved, currentSubmitted
		}
	}
}
