package types

import (
	"sync"
	"sync/atomic"
	"time"
)

// Task 任务结构体
type Task struct {
	TaskID       string
	ProgramID    string
	PublicInputs []byte
	NodeID       string
	CreatedAt    time.Time
}

// RetryProof 提交重试结构体
type RetryProof struct {
	Task       *Task
	Proof      []byte
	RetryCount int
}

// LightRetryProof 轻量级重试结构体 - 只存储必要信息
type LightRetryProof struct {
	TaskID     string
	ProgramID  string
	NodeID     string
	Proof      []byte
	RetryCount int
}

// TaskQueue 任务队列结构体
type TaskQueue struct {
	tasks chan *Task
	mu    sync.RWMutex
	stats struct {
		queued    int64
		processed int64
		failed    int64
	}
	// 新增重试队列
	retryQueue chan *RetryProof
}

// NewTaskQueue 创建新的任务队列
func NewTaskQueue(capacity int, retryCapacity int) *TaskQueue {
	return &TaskQueue{
		tasks:      make(chan *Task, capacity),
		retryQueue: make(chan *RetryProof, retryCapacity),
	}
}

// AddTask 添加任务到队列
func (tq *TaskQueue) AddTask(task *Task) bool {
	select {
	case tq.tasks <- task:
		atomic.AddInt64(&tq.stats.queued, 1)
		return true
	default:
		return false // 队列已满
	}
}

// GetTask 从队列获取任务
func (tq *TaskQueue) GetTask() (*Task, bool) {
	select {
	case task := <-tq.tasks:
		return task, true
	default:
		return nil, false // 队列为空
	}
}

// AddRetry 添加重试任务
func (tq *TaskQueue) AddRetry(rp *RetryProof) {
	tq.retryQueue <- rp
}

// GetRetry 获取重试任务（阻塞）
func (tq *TaskQueue) GetRetry() *RetryProof {
	return <-tq.retryQueue
}

// TryGetRetry 获取重试任务（非阻塞）
func (tq *TaskQueue) TryGetRetry() (*RetryProof, bool) {
	select {
	case rp := <-tq.retryQueue:
		return rp, true
	default:
		return nil, false
	}
}

// GetStats 获取队列统计信息
func (tq *TaskQueue) GetStats() (int64, int64, int64) {
	return atomic.LoadInt64(&tq.stats.queued),
		atomic.LoadInt64(&tq.stats.processed),
		atomic.LoadInt64(&tq.stats.failed)
}

// MarkProcessed 标记任务处理完成
func (tq *TaskQueue) MarkProcessed() {
	atomic.AddInt64(&tq.stats.processed, 1)
}

// MarkFailed 标记任务处理失败
func (tq *TaskQueue) MarkFailed() {
	atomic.AddInt64(&tq.stats.failed, 1)
}

// TaskFetchState 任务状态管理结构
type TaskFetchState struct {
	lastFetchTime    time.Time
	lastQueueLogTime time.Time
	queueLogInterval time.Duration
	Consecutive404s  int
}

// NewTaskFetchState 创建新的任务获取状态
func NewTaskFetchState() *TaskFetchState {
	return &TaskFetchState{
		lastFetchTime:    time.Now().Add(-180*time.Second - time.Second), // 允许立即首次获取
		lastQueueLogTime: time.Now(),
		queueLogInterval: 30 * time.Second,
		Consecutive404s:  0,
	}
}

// ShouldFetch 检查是否应该获取任务
func (s *TaskFetchState) ShouldFetch() bool {
	return time.Since(s.lastFetchTime) >= 180*time.Second // 固定间隔检查
}

// SetLastFetchTime 设置获取任务的时间
func (s *TaskFetchState) SetLastFetchTime() {
	s.lastFetchTime = time.Now()
}

// ShouldPrintLog 检查是否应该打印日志
func (s *TaskFetchState) ShouldPrintLog() bool {
	return time.Since(s.lastQueueLogTime) >= s.queueLogInterval // 队列日志间隔检查
}

// SetPrintLogTime 设置队列日志时间
func (s *TaskFetchState) SetPrintLogTime() {
	s.lastQueueLogTime = time.Now()
}
