package worker

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"nexus-prover/internal/api"
	"nexus-prover/internal/utils"
	"nexus-prover/pkg/prover"
	"nexus-prover/pkg/types"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ProcessIsolationConfig 进程隔离配置
type ProcessIsolationConfig struct {
	NodeIDs           []string `json:"node_ids"`
	UserID            string   `json:"user_id"`
	WalletAddress     string   `json:"wallet_address"`
	RequestDelay      int      `json:"request_delay"`
	ProverWorkers     int      `json:"prover_workers"`
	TaskQueueCapacity int      `json:"task_queue_capacity"`
	MaxLifetime       int      `json:"max_lifetime"` // 秒
	MaxRestarts       int      `json:"max_restarts"`
}

// ProcessProverRequest 进程证明请求
type ProcessProverRequest struct {
	TaskID       string `json:"task_id"`
	ProgramID    string `json:"program_id"`
	PublicInputs []byte `json:"public_inputs"`
	NodeID       string `json:"node_id"`
}

// ProcessProverResponse 进程证明响应
type ProcessProverResponse struct {
	Success bool   `json:"success"`
	Proof   []byte `json:"proof,omitempty"`
	Error   string `json:"error,omitempty"`
	TaskID  string `json:"task_id"`
}

// ProcessProver 进程隔离的证明器
type ProcessProver struct {
	execPath      string // 原始可执行文件
	memfsExecPath string // 内存盘可执行文件
	memfsNexusDir string // 内存盘nexus目录
	maxLifetime   time.Duration
	maxRestarts   int
	restartCount  int
	mu            sync.Mutex
}

// NewProcessProver 创建新的进程证明器
func NewProcessProver(execPath string, maxLifetime, maxRestarts int) *ProcessProver {
	memfs := ""
	memfsNexus := ""
	memfsExec := execPath
	if mfs, err := FindMemoryFS(); err == nil && mfs != "" {
		memfs = mfs
		memfsNexus = filepath.Join(memfs, "nexus")
		_ = os.MkdirAll(memfsNexus, 0755)
		if exec, err := EnsureExecInMemFS(execPath, memfsNexus); err == nil {
			memfsExec = exec
		}
	}
	return &ProcessProver{
		execPath:      execPath,
		memfsExecPath: memfsExec,
		memfsNexusDir: memfsNexus,
		maxLifetime:   time.Duration(maxLifetime) * time.Second,
		maxRestarts:   maxRestarts,
	}
}

// 检查目录是否可写（权限+实际写入测试）
func isWritable(dir string) bool {
	if syscall.Access(dir, 2) != nil {
		return false
	}
	testFile := filepath.Join(dir, ".writable_test")
	f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(testFile)
	return true
}

// 查找可用的内存文件系统挂载点（带可写检查）
func FindMemoryFS() (string, error) {
	const minSpaceBytes = 3 * 1024 * 1024 * 1024 // 3GB

	f, err := os.Open("/proc/mounts")
	if err != nil {
		return "", err
	}
	defer f.Close()

	type mountInfo struct {
		path      string
		available uint64
		isDevShm  bool
	}

	var availableMounts []mountInfo

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		mountPoint := fields[1]
		fsType := fields[2]

		if (fsType == "tmpfs" || fsType == "ramfs") && isWritable(mountPoint) {
			available, err := getAvailableSpace(mountPoint)
			if err != nil {
				continue
			}
			availableMounts = append(availableMounts, mountInfo{
				path:      mountPoint,
				available: available,
				isDevShm:  mountPoint == "/dev/shm",
			})
		}
	}

	// 兜底
	if len(availableMounts) == 0 {
		for _, fallback := range []string{"/dev/shm", "/tmp"} {
			if isWritable(fallback) {
				available, err := getAvailableSpace(fallback)
				if err != nil {
					continue
				}
				availableMounts = append(availableMounts, mountInfo{
					path:      fallback,
					available: available,
					isDevShm:  fallback == "/dev/shm",
				})
			}
		}
	}

	if len(availableMounts) == 0 {
		return "", fmt.Errorf("未找到可用的内存文件系统")
	}

	for _, mount := range availableMounts {
		if mount.isDevShm && mount.available >= minSpaceBytes {
			return mount.path, nil
		}
	}

	var bestMount mountInfo
	for _, mount := range availableMounts {
		if mount.available > bestMount.available {
			bestMount = mount
		}
	}

	if bestMount.available > 0 {
		return bestMount.path, nil
	}

	return "", fmt.Errorf("未找到有足够空间的内存文件系统")
}

// 检查目录的可用空间（以字节为单位）
func getAvailableSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

// 确保内存盘 nexus 目录下有可执行文件（与原名一致）
func EnsureExecInMemFS(execPath, nexusDir string) (string, error) {
	// nexusDir := filepath.Join(memfsBase, "nexus")
	if err := os.MkdirAll(nexusDir, 0755); err != nil {
		return "", err
	}
	execName := filepath.Base(execPath)
	memfsExec := filepath.Join(nexusDir, execName)
	if fi, err := os.Stat(memfsExec); err == nil && fi.Mode()&0111 != 0 {
		return memfsExec, nil
	}
	src, err := os.Open(execPath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.OpenFile(memfsExec, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return memfsExec, nil
}

// Prove 使用进程隔离执行证明
func (pp *ProcessProver) Prove(task *types.Task) ([]byte, error) {
	pp.mu.Lock()
	if pp.restartCount >= pp.maxRestarts {
		pp.mu.Unlock()
		return nil, fmt.Errorf("进程重启次数已达上限: %d", pp.maxRestarts)
	}
	pp.mu.Unlock()

	// 创建临时目录（优先用内存盘nexus目录）
	tempDir, err := os.MkdirTemp(pp.memfsNexusDir, "prover-*")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建请求
	request := ProcessProverRequest{
		TaskID:       task.TaskID,
		ProgramID:    task.ProgramID,
		PublicInputs: task.PublicInputs,
		NodeID:       task.NodeID,
	}

	// 写入请求文件
	requestFile := filepath.Join(tempDir, "request.json")
	requestData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %v", err)
	}

	if err := os.WriteFile(requestFile, requestData, 0644); err != nil {
		return nil, fmt.Errorf("写入请求文件失败: %v", err)
	}

	// 启动进程
	ctx, cancel := context.WithTimeout(context.Background(), pp.maxLifetime)
	defer cancel()

	cmd := exec.CommandContext(ctx, pp.memfsExecPath, "--prove", "--request", requestFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		pp.mu.Lock()
		pp.restartCount++
		pp.mu.Unlock()
		return nil, fmt.Errorf("进程执行失败: %v, 输出: %s", err, string(output))
	}

	// 读取响应
	responseFile := filepath.Join(tempDir, "response.json")
	responseData, err := os.ReadFile(responseFile)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	var response ProcessProverResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("证明失败: %s", response.Error)
	}

	// 重置重启计数
	pp.mu.Lock()
	pp.restartCount = 0
	pp.mu.Unlock()

	return response.Proof, nil
}

// GetRestartCount 获取重启次数
func (pp *ProcessProver) GetRestartCount() int {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	return pp.restartCount
}

// ProcessWorker 进程隔离的worker
func ProcessWorker(ctx context.Context, id int, priv ed25519.PrivateKey, taskQueue *types.TaskQueue, wg *sync.WaitGroup, prover *ProcessProver) {
	defer wg.Done()
	utils.LogWithTime("[process-worker-%d] 开始进程隔离证明计算", id)

	for {
		select {
		case <-ctx.Done():
			utils.LogWithTime("[process-worker-%d] Shutting down...", id)
			return
		default:
			// 从队列获取任务
			task, ok := taskQueue.GetTask()
			if !ok {
				time.Sleep(1 * time.Second)
				continue
			}

			utils.LogWithTime("[process-worker-%d] 任务 %s PublicInputs 长度: %d 字节", id, task.TaskID, len(task.PublicInputs))

			// 使用进程隔离执行证明
			proof, err := prover.Prove(task)
			if err != nil {
				utils.LogWithTime("[process-worker-%d] ❌ 任务 %s 证明计算失败: %v", id, task.TaskID, err)
				taskQueue.MarkFailed()
				continue
			}

			utils.LogWithTime("[process-worker-%d] 任务 %s Proof 长度: %d 字节", id, task.TaskID, len(proof))

			// 增加证明计数器
			incProved()
			taskQueue.MarkProcessed()

			// 提交证明
			apiClient := api.NewClient()
			err = apiClient.SubmitProof(task, proof, priv)
			if err != nil {
				if strings.Contains(err.Error(), "NotFoundError") && strings.Contains(err.Error(), "Task not found") && strings.Contains(err.Error(), "httpCode\":404") {
					utils.LogWithTime("❌ 任务 %s 提交失败(404 NotFound)，直接丢弃: %v", task.TaskID, err)
					utils.ClearProofData(proof)
					proof = nil
				} else {
					utils.LogWithTime("[process-worker-%d] ❌ 任务 %s 证明提交失败: %v", id, task.TaskID, err)
					taskQueue.AddRetry(&types.RetryProof{Task: task, Proof: proof, RetryCount: 1})
				}
			} else {
				utils.LogWithTime("[process-worker-%d] ✅ 任务 %s 证明提交成功", id, task.TaskID)
				incSubmitted() // 增加提交成功计数器
				utils.ClearProofData(proof)
				proof = nil
			}
		}
	}
}

// RunProcessWorker 运行进程worker模式
func RunProcessWorker() {
	var (
		proveMode   = flag.Bool("prove", false, "运行证明模式")
		requestFile = flag.String("request", "", "请求文件路径")
	)
	flag.Parse()

	if *proveMode {
		if *requestFile == "" {
			log.Fatal("证明模式需要指定请求文件路径")
		}

		// 读取请求文件
		requestData, err := os.ReadFile(*requestFile)
		if err != nil {
			log.Fatalf("读取请求文件失败: %v", err)
		}

		var request ProcessProverRequest
		if err := json.Unmarshal(requestData, &request); err != nil {
			log.Fatalf("解析请求失败: %v", err)
		}

		// 创建任务对象
		task := &types.Task{
			TaskID:       request.TaskID,
			ProgramID:    request.ProgramID,
			PublicInputs: request.PublicInputs,
			NodeID:       request.NodeID,
			CreatedAt:    time.Now(),
		}

		// 执行证明
		proof, err := prover.Prove(task, false) // 使用官方zkVM

		// 创建响应
		response := ProcessProverResponse{
			TaskID: request.TaskID,
		}

		if err != nil {
			response.Success = false
			response.Error = err.Error()
		} else {
			response.Success = true
			response.Proof = proof
		}

		// 写入响应文件
		responseFile := filepath.Join(filepath.Dir(*requestFile), "response.json")
		responseData, err := json.Marshal(response)
		if err != nil {
			log.Fatalf("序列化响应失败: %v", err)
		}

		if err := os.WriteFile(responseFile, responseData, 0644); err != nil {
			log.Fatalf("写入响应文件失败: %v", err)
		}

		utils.LogWithTime("✅ 证明完成")
		os.Exit(0)
	}
}
