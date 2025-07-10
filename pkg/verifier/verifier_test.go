package verifier

import (
	"encoding/binary"
	"fmt"
	"testing"

	"nexus-prover/pkg/types"
)

// TestVerifierConsistency 测试验证器的一致性
func TestVerifierConsistency(t *testing.T) {
	tests := []struct {
		name      string
		programID string
		inputs    []byte
		expected  uint32
	}{
		{
			name:      "fib_input_initial - 基础测试",
			programID: "fib_input_initial",
			inputs:    makeFibInputInitial(5, 1, 2), // F(5) with init values 1,2
			expected:  13,                           // F(5) = 13 when F(0)=1, F(1)=2
		},
		{
			name:      "fib_input_initial - 零值测试",
			programID: "fib_input_initial",
			inputs:    makeFibInputInitial(0, 1, 2), // F(0) with init values 1,2
			expected:  1,                            // F(0) = 1
		},
		{
			name:      "fib_input_initial - 第一项测试",
			programID: "fib_input_initial",
			inputs:    makeFibInputInitial(1, 1, 2), // F(1) with init values 1,2
			expected:  2,                            // F(1) = 2
		},
		{
			name:      "fib_input - 标准斐波那契",
			programID: "fib_input",
			inputs:    makeFibInput(10), // F(10) with standard init values 0,1
			expected:  55,               // F(10) = 55
		},
		{
			name:      "fib_input - 零值测试",
			programID: "fib_input",
			inputs:    makeFibInput(0), // F(0) with standard init values 0,1
			expected:  0,               // F(0) = 0
		},
		{
			name:      "fib_input - 第一项测试",
			programID: "fib_input",
			inputs:    makeFibInput(1), // F(1) with standard init values 0,1
			expected:  1,               // F(1) = 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建测试任务
			task := &types.Task{
				TaskID:       "test-task-" + tt.name,
				ProgramID:    tt.programID,
				PublicInputs: tt.inputs,
				NodeID:       "test-node",
			}

			// 创建验证器
			verifier := NewNexusVerifier(true) // 使用本地模式

			// 验证本地计算结果
			result, err := verifier.VerifyLocalResult(task, tt.expected)
			if err != nil {
				t.Fatalf("验证本地结果失败: %v", err)
			}

			if !result.Success {
				t.Errorf("验证失败: %s", result.Error)
			}

			// 构造证明数据
			proof := make([]byte, 4)
			binary.LittleEndian.PutUint32(proof, tt.expected)

			// 验证证明
			verifyResult, err := verifier.VerifyProof(proof, task)
			if err != nil {
				t.Fatalf("验证证明失败: %v", err)
			}

			if !verifyResult.Success {
				t.Errorf("证明验证失败: %s", verifyResult.Error)
			}

			// 验证输出结果
			if len(verifyResult.PublicOutput) != 4 {
				t.Errorf("输出长度错误，期望4字节，实际%d字节", len(verifyResult.PublicOutput))
			}

			outputResult := binary.LittleEndian.Uint32(verifyResult.PublicOutput)
			if outputResult != tt.expected {
				t.Errorf("输出结果错误，期望%d，实际%d", tt.expected, outputResult)
			}

			t.Logf("✅ 验证成功: %s, 结果: %d", tt.name, outputResult)
		})
	}
}

// TestVerifierErrorCases 测试验证器的错误情况
func TestVerifierErrorCases(t *testing.T) {
	tests := []struct {
		name       string
		programID  string
		inputs     []byte
		expected   uint32
		shouldFail bool
	}{
		{
			name:       "fib_input_initial - 输入不足",
			programID:  "fib_input_initial",
			inputs:     []byte{1, 2, 3}, // 只有3字节，需要12字节
			expected:   0,
			shouldFail: true,
		},
		{
			name:       "fib_input - 输入不足",
			programID:  "fib_input",
			inputs:     []byte{}, // 空输入
			expected:   0,
			shouldFail: true,
		},
		{
			name:       "不支持的程序ID",
			programID:  "unsupported_program",
			inputs:     []byte{1, 2, 3, 4},
			expected:   0,
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建测试任务
			task := &types.Task{
				TaskID:       "test-task-" + tt.name,
				ProgramID:    tt.programID,
				PublicInputs: tt.inputs,
				NodeID:       "test-node",
			}

			// 创建验证器
			verifier := NewNexusVerifier(true) // 使用本地模式

			// 验证本地计算结果
			result, err := verifier.VerifyLocalResult(task, tt.expected)
			if err != nil {
				t.Fatalf("验证本地结果失败: %v", err)
			}

			if tt.shouldFail {
				if result.Success {
					t.Errorf("期望验证失败，但验证成功了")
				}
				t.Logf("✅ 正确捕获错误: %s", result.Error)
			} else {
				if !result.Success {
					t.Errorf("期望验证成功，但验证失败了: %s", result.Error)
				}
			}
		})
	}
}

// TestZkVMVerifier 测试zkVM验证器
func TestZkVMVerifier(t *testing.T) {
	tests := []struct {
		name          string
		programID     string
		inputs        []byte
		proof         []byte
		shouldSucceed bool
	}{
		{
			name:          "有效的zkVM证明",
			programID:     "fib_input_initial",
			inputs:        makeFibInputInitial(5, 1, 2),
			proof:         []byte{13, 0, 0, 0}, // 小端序的13
			shouldSucceed: true,
		},
		{
			name:          "空证明",
			programID:     "fib_input_initial",
			inputs:        makeFibInputInitial(5, 1, 2),
			proof:         []byte{},
			shouldSucceed: false,
		},
		{
			name:          "结果过大",
			programID:     "fib_input_initial",
			inputs:        makeFibInputInitial(5, 1, 2),
			proof:         []byte{0xFF, 0xFF, 0xFF, 0xFF}, // 最大值
			shouldSucceed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建测试任务
			task := &types.Task{
				TaskID:       "test-task-" + tt.name,
				ProgramID:    tt.programID,
				PublicInputs: tt.inputs,
				NodeID:       "test-node",
			}

			// 创建zkVM验证器
			verifier := NewNexusVerifier(false) // 使用zkVM模式

			// 验证证明
			result, err := verifier.VerifyProof(tt.proof, task)
			if err != nil {
				t.Fatalf("验证证明失败: %v", err)
			}

			if tt.shouldSucceed {
				if !result.Success {
					t.Errorf("期望验证成功，但验证失败了: %s", result.Error)
				} else {
					t.Logf("✅ zkVM验证成功: %s", tt.name)
				}
			} else {
				if result.Success {
					t.Errorf("期望验证失败，但验证成功了")
				} else {
					t.Logf("✅ 正确捕获zkVM错误: %s", result.Error)
				}
			}
		})
	}
}

// TestAlgorithmConsistency 测试算法一致性
func TestAlgorithmConsistency(t *testing.T) {
	testCases := []struct {
		n, initA, initB uint32
		expected        uint32
	}{
		{0, 1, 2, 1},    // F(0) = initA = 1
		{1, 1, 2, 2},    // F(1) = initB = 2
		{2, 1, 2, 3},    // F(2) = F(0) + F(1) = 1 + 2 = 3
		{3, 1, 2, 5},    // F(3) = F(1) + F(2) = 2 + 3 = 5
		{4, 1, 2, 8},    // F(4) = F(2) + F(3) = 3 + 5 = 8
		{5, 1, 2, 13},   // F(5) = F(3) + F(4) = 5 + 8 = 13
		{10, 1, 2, 144}, // F(10) with init values 1,2
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("F(%d) with init(%d,%d)", tc.n, tc.initA, tc.initB), func(t *testing.T) {
			// 测试迭代算法
			result1 := fibInputInitial0(tc.n, tc.initA, tc.initB)
			if result1 != tc.expected {
				t.Errorf("迭代算法错误: F(%d) = %d, 期望 %d", tc.n, result1, tc.expected)
			}

			// 测试矩阵快速幂算法
			result2 := fibInputInitial(tc.n, tc.initA, tc.initB)
			if result2 != tc.expected {
				t.Errorf("矩阵快速幂算法错误: F(%d) = %d, 期望 %d", tc.n, result2, tc.expected)
			}

			// 比较两种算法结果
			if result1 != result2 {
				t.Errorf("算法结果不一致！迭代: %d, 矩阵快速幂: %d", result1, result2)
			} else {
				t.Logf("✅ F(%d) = %d", tc.n, result1)
			}
		})
	}
}

// 辅助函数
func makeFibInputInitial(n, initA, initB uint32) []byte {
	result := make([]byte, 12)
	binary.LittleEndian.PutUint32(result[0:4], n)
	binary.LittleEndian.PutUint32(result[4:8], initA)
	binary.LittleEndian.PutUint32(result[8:12], initB)
	return result
}

func makeFibInput(n uint32) []byte {
	result := make([]byte, 4)
	binary.LittleEndian.PutUint32(result, n)
	return result
}
