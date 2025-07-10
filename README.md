# Nexus Prover Go 客户端

这是一个重构后的 Nexus Prover Go 客户端，采用了标准的 Go 项目结构。

## 项目结构

```
nexus-prover/
├── cmd/                         # 可执行文件入口
│   └── nexus-prover/
│       └── main.go              # 主程序入口
├── internal/                    # 内部包（不对外暴露）
│   ├── api/                     # API 客户端
│   │   └── client.go
│   ├── config/                  # 配置管理
│   │   └── config.go
│   ├── utils/                   # 工具函数
│   │   └── utils.go
│   └── worker/                  # 工作器模块
│       ├── worker.go
│       └── process_isolation.go
├── pkg/                         # 可导出的包
│   ├── prover/                  # 证明计算, 官方完整的zkVM静态库文件
│   │   └── prover.go
│   └── types/                   # 类型定义
│       └── task.go
├── proto/                       # 协议定义
│   ├── orchestrator.proto
│   └── orchestrator.pb.go
├── configs/                     # 配置文件
│   └── config.json
├── scripts/                     # 构建脚本
│   ├── build.sh
│   └── run.sh
├── go.mod
└── go.sum
```
## 核心特性

### 1. 进程隔离
- 每个证明任务在独立进程中执行
- 进程完成后自动释放所有CGO内存
- 支持超时控制和重启限制

### 2. 双模式支持
- **普通模式**: 传统的内存共享方式, 执行go代码实现的证明计算(但不包括官方的验证、序列化、证明签名验证), 不推荐使用此模式
- **进程隔离模式**: 每个任务独立进程执行, 执行官方rust zkVM库计算证明及验证

### 3. 自动恢复
- 进程崩溃自动重启
- 可配置最大重启次数
- 超时保护防止进程卡死

## 工作原理

### 进程隔离流程
1. **主进程** 从队列获取任务
2. **创建内存文件系统临时目录** 存放请求和响应文件
3. **启动子进程** 执行证明计算
4. **子进程** 读取请求文件，执行官方rust zkVM库计算证明及验证，写入响应文件
5. **子进程** 完成后自动退出，释放所有内存
6. **主进程** 读取响应文件，提交证明

### 优势
- ✅ **内存安全**: CGO内存随进程退出自动释放
- ✅ **稳定性**: 单个任务失败不影响其他任务
- ✅ **可监控**: 每个进程独立运行，便于监控
- ✅ **自动恢复**: 进程崩溃自动重启

### 日志分析
程序会输出详细的运行日志，包括：
- 进程启动/退出信息
- 任务处理状态
- 错误和异常信息
- 内存使用统计

### 注意事项
- ⚠️ **性能开销**: 进程创建有一定开销
- ⚠️ **文件I/O**: 需要临时文件传递数据
- ⚠️ **资源管理**: 需要合理设置超时和重启限制

## 构建和运行

### 构建
```bash
git clone https://github.com/moran666666/nexus-prover.git
cd nexus-prover/
./scripts/build.sh
```

### 运行
```bash
./nexus-prover -c configs/config.json -ps
```

### 配置
编辑 `configs/config.json` 文件：
```json
{
  "node_ids": ["节点ID1", "节点ID2"],
  "user_id": "用户ID",
  "wallet_address": "钱包地址",
  "request_delay": 0,
  "prover_workers": 9,
  "task_queue_capacity": 1000
}
```
