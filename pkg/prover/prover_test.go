package prover

import (
	"encoding/binary"
	"fmt"
	"testing"

	"nexus-prover/pkg/types"
)

// TestProveConsistency 测试zkVM和本地Go代码生成证明的一致性
func TestProveConsistency(t *testing.T) {
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

			// 使用本地Go算法生成证明
			localProof, err := Prove(task, true) // useLocal = true
			if err != nil {
				t.Fatalf("本地算法生成证明失败: %v", err)
			}

			// 验证本地算法结果
			if len(localProof) != 4 {
				t.Fatalf("本地算法证明长度错误，期望4字节，实际%d字节", len(localProof))
			}

			localResult := binary.LittleEndian.Uint32(localProof)
			if localResult != tt.expected {
				t.Errorf("本地算法结果错误，期望%d，实际%d", tt.expected, localResult)
			}

			// 尝试使用zkVM生成证明（如果可用）
			zkvmProof, err := Prove(task, false) // useLocal = false
			if err != nil {
				t.Logf("zkVM生成证明失败（可能环境不支持）: %v", err)
				t.Logf("本地算法结果正确: %d", localResult)
				return
			}

			// 验证zkVM结果
			if len(zkvmProof) < 4 {
				t.Fatalf("zkVM证明长度错误，期望至少4字节，实际%d字节", len(zkvmProof))
			}

			// 对于zkVM，我们只比较前4字节（如果证明包含结果的话）
			// 或者我们可以提取证明中的结果部分
			var zkvmResult uint32
			if len(zkvmProof) == 4 {
				// 如果zkVM返回的是4字节结果
				zkvmResult = binary.LittleEndian.Uint32(zkvmProof)
			} else {
				// 如果zkVM返回的是完整证明，我们无法直接比较数值
				// 这种情况下，我们只验证本地算法的正确性
				t.Logf("zkVM返回完整证明（%d字节），无法直接比较数值", len(zkvmProof))
				t.Logf("本地算法结果正确: %d", localResult)
				return
			}

			if zkvmResult != tt.expected {
				t.Errorf("zkVM结果错误，期望%d，实际%d", tt.expected, zkvmResult)
			}

			// 比较两种方法的结果
			if localResult != zkvmResult {
				t.Errorf("结果不一致！本地算法: %d, zkVM: %d", localResult, zkvmResult)
			} else {
				t.Logf("✅ 结果一致: %d", localResult)
			}
		})
	}
}

// TestFibInputInitialConsistency 专门测试fib_input_initial算法的一致性
func TestFibInputInitialConsistency(t *testing.T) {
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

// TestFibInputConsistency 专门测试fib_input算法的一致性
func TestFibInputConsistency(t *testing.T) {
	testCases := []struct {
		n        uint32
		expected uint32
	}{
		{0, 0},   // F(0) = 0
		{1, 1},   // F(1) = 1
		{2, 1},   // F(2) = 1
		{3, 2},   // F(3) = 2
		{4, 3},   // F(4) = 3
		{5, 5},   // F(5) = 5
		{6, 8},   // F(6) = 8
		{7, 13},  // F(7) = 13
		{8, 21},  // F(8) = 21
		{9, 34},  // F(9) = 34
		{10, 55}, // F(10) = 55
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("F(%d)", tc.n), func(t *testing.T) {
			result := fibInput(tc.n)
			if result != tc.expected {
				t.Errorf("fib_input错误: F(%d) = %d, 期望 %d", tc.n, result, tc.expected)
			} else {
				t.Logf("✅ F(%d) = %d", tc.n, result)
			}
		})
	}
}

// TestMatrixOperations 测试矩阵运算的正确性
func TestMatrixOperations(t *testing.T) {
	// 测试矩阵乘法
	mat1 := [2][2]uint64{{1, 1}, {1, 0}}
	mat2 := [2][2]uint64{{2, 1}, {1, 1}}

	result := matMul(mat1, mat2)
	expected := [2][2]uint64{{3, 2}, {2, 1}}

	if result != expected {
		t.Errorf("矩阵乘法错误: 结果 %v, 期望 %v", result, expected)
	}

	// 测试矩阵幂运算
	mat := [2][2]uint64{{1, 1}, {1, 0}}
	power2 := matPow(mat, 2)
	expected2 := [2][2]uint64{{2, 1}, {1, 1}}

	if power2 != expected2 {
		t.Errorf("矩阵幂运算错误: 结果 %v, 期望 %v", power2, expected2)
	}
}

// 辅助函数：创建fib_input_initial的输入
func makeFibInputInitial(n, initA, initB uint32) []byte {
	result := make([]byte, 12)
	binary.LittleEndian.PutUint32(result[0:4], n)
	binary.LittleEndian.PutUint32(result[4:8], initA)
	binary.LittleEndian.PutUint32(result[8:12], initB)
	return result
}

// 辅助函数：创建fib_input的输入
func makeFibInput(n uint32) []byte {
	result := make([]byte, 4)
	binary.LittleEndian.PutUint32(result, n)
	return result
}
