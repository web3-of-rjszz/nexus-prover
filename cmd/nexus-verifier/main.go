package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"nexus-prover/pkg/types"
	"nexus-prover/pkg/verifier"
)

// VerificationRequest 验证请求
type VerificationRequest struct {
	TaskID       string `json:"task_id"`
	ProgramID    string `json:"program_id"`
	PublicInputs []byte `json:"public_inputs"`
	NodeID       string `json:"node_id"`
	Proof        []byte `json:"proof"`
}

// VerificationResponse 验证响应
type VerificationResponse struct {
	TaskID       string   `json:"task_id"`
	Success      bool     `json:"success"`
	Error        string   `json:"error,omitempty"`
	ExitCode     uint32   `json:"exit_code"`
	PublicOutput []byte   `json:"public_output,omitempty"`
	Logs         []string `json:"logs,omitempty"`
}

func main() {
	// 定义命令行参数
	requestFile := flag.String("request", "", "验证请求文件路径")
	responseFile := flag.String("response", "", "验证响应文件路径（可选，默认与请求文件同目录）")
	useLocal := flag.Bool("local", false, "使用本地验证模式")
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

	// 检查请求文件
	if *requestFile == "" {
		log.Fatal("❌ 必须指定验证请求文件路径 (-request)")
	}

	// 检查请求文件是否存在
	if _, err := os.Stat(*requestFile); os.IsNotExist(err) {
		log.Fatalf("❌ 请求文件不存在: %s", *requestFile)
	}

	// 读取请求文件
	requestData, err := os.ReadFile(*requestFile)
	if err != nil {
		log.Fatalf("❌ 读取请求文件失败: %v", err)
	}

	var request VerificationRequest
	if err := json.Unmarshal(requestData, &request); err != nil {
		log.Fatalf("❌ 解析请求文件失败: %v", err)
	}

	// 验证请求数据
	if request.TaskID == "" {
		log.Fatal("❌ 请求中缺少task_id")
	}
	if request.ProgramID == "" {
		log.Fatal("❌ 请求中缺少program_id")
	}
	if len(request.Proof) == 0 {
		log.Fatal("❌ 请求中缺少proof数据")
	}

	// 创建任务对象
	task := &types.Task{
		TaskID:       request.TaskID,
		ProgramID:    request.ProgramID,
		PublicInputs: request.PublicInputs,
		NodeID:       request.NodeID,
	}

	// 创建验证器
	v := verifier.NewNexusVerifier(*useLocal)

	// 执行验证
	fmt.Printf("🔍 开始验证证明...\n")
	fmt.Printf("   任务ID: %s\n", task.TaskID)
	fmt.Printf("   程序ID: %s\n", task.ProgramID)
	fmt.Printf("   输入长度: %d 字节\n", len(task.PublicInputs))
	fmt.Printf("   证明长度: %d 字节\n", len(request.Proof))
	fmt.Printf("   验证模式: %s\n", getModeString(*useLocal))

	result, err := v.VerifyProof(request.Proof, task)
	if err != nil {
		log.Fatalf("❌ 验证过程出错: %v", err)
	}

	// 构造响应
	response := VerificationResponse{
		TaskID:       request.TaskID,
		Success:      result.Success,
		ExitCode:     result.ExitCode,
		PublicOutput: result.PublicOutput,
		Logs:         result.Logs,
	}

	if !result.Success {
		response.Error = result.Error
	}

	// 输出验证结果
	if result.Success {
		fmt.Printf("✅ 验证成功!\n")
		fmt.Printf("   退出码: %d\n", result.ExitCode)
		if len(result.PublicOutput) >= 4 {
			outputValue := binary.LittleEndian.Uint32(result.PublicOutput)
			fmt.Printf("   输出值: %d\n", outputValue)
		}
		if len(result.Logs) > 0 {
			fmt.Printf("   日志:\n")
			for _, log := range result.Logs {
				fmt.Printf("     %s\n", log)
			}
		}
	} else {
		fmt.Printf("❌ 验证失败: %s\n", result.Error)
	}

	// 确定响应文件路径
	if *responseFile == "" {
		dir := filepath.Dir(*requestFile)
		base := filepath.Base(*requestFile)
		ext := filepath.Ext(base)
		name := base[:len(base)-len(ext)]
		*responseFile = filepath.Join(dir, name+"_response.json")
	}

	// 写入响应文件
	responseData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		log.Fatalf("❌ 序列化响应失败: %v", err)
	}

	if err := os.WriteFile(*responseFile, responseData, 0644); err != nil {
		log.Fatalf("❌ 写入响应文件失败: %v", err)
	}

	fmt.Printf("📄 响应已写入: %s\n", *responseFile)
}

func getModeString(useLocal bool) string {
	if useLocal {
		return "本地模式 (Go算法验证)"
	}
	return "zkVM模式 (Nexus zkVM验证)"
}

func printHelp() {
	fmt.Println("Nexus Verifier CLI (本地/zkVM验证模式)")
	fmt.Println("")
	fmt.Println("用法:")
	fmt.Println("  ./nexus-verifier -request <请求文件> [-response <响应文件>] [-local]")
	fmt.Println("")
	fmt.Println("参数:")
	fmt.Println("  -request <文件>           # 指定验证请求文件路径")
	fmt.Println("  -response <文件>          # 指定验证响应文件路径 (可选)")
	fmt.Println("  -local                    # 启用本地验证模式")
	fmt.Println("  -h, --help                # 显示帮助信息")
	fmt.Println("  -v, --version             # 显示版本信息")
	fmt.Println("")
	fmt.Println("示例:")
	fmt.Println("  ./nexus-verifier -request verify_request.json")
	fmt.Println("  ./nexus-verifier -request verify_request.json -local")
	fmt.Println("  ./nexus-verifier -request verify_request.json -response result.json")
	fmt.Println("")
	fmt.Println("请求文件格式:")
	fmt.Println("  {")
	fmt.Println("    \"task_id\": \"任务ID\",")
	fmt.Println("    \"program_id\": \"程序ID\",")
	fmt.Println("    \"public_inputs\": [字节数组],")
	fmt.Println("    \"node_id\": \"节点ID\",")
	fmt.Println("    \"proof\": [证明字节数组]")
	fmt.Println("  }")
	fmt.Println("")
	fmt.Println("响应文件格式:")
	fmt.Println("  {")
	fmt.Println("    \"task_id\": \"任务ID\",")
	fmt.Println("    \"success\": true/false,")
	fmt.Println("    \"error\": \"错误信息 (如果失败)\",")
	fmt.Println("    \"exit_code\": 0,")
	fmt.Println("    \"public_output\": [输出字节数组],")
	fmt.Println("    \"logs\": [\"日志1\", \"日志2\"]")
	fmt.Println("  }")
	fmt.Println("")
}

func printVersion() {
	fmt.Println("Nexus Verifier CLI v1.0.0 (本地/zkVM验证模式)")
}
