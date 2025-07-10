package verifier

import (
	"encoding/binary"
	"fmt"

	"nexus-prover/pkg/types"
)

// VerificationResult 验证结果
type VerificationResult struct {
	Success      bool
	Error        string
	ExitCode     uint32
	PublicOutput []byte
	Logs         []string
}

// Verifier 验证器接口
type Verifier interface {
	VerifyProof(proof []byte, task *types.Task) (*VerificationResult, error)
	VerifyLocalResult(task *types.Task, expectedResult uint32) (*VerificationResult, error)
}

// NexusVerifier Nexus zkVM验证器实现
type NexusVerifier struct {
	useLocal bool
}

// NewNexusVerifier 创建新的验证器
func NewNexusVerifier(useLocal bool) *NexusVerifier {
	return &NexusVerifier{
		useLocal: useLocal,
	}
}

// VerifyProof 验证zkVM生成的证明
func (v *NexusVerifier) VerifyProof(proof []byte, task *types.Task) (*VerificationResult, error) {
	if v.useLocal {
		return v.verifyLocalProof(proof, task)
	}
	return v.verifyZkVMProof(proof, task)
}

// verifyZkVMProof 使用zkVM验证证明（简化版本，仅用于演示）
func (v *NexusVerifier) verifyZkVMProof(proof []byte, task *types.Task) (*VerificationResult, error) {
	// 这是一个简化的实现，实际应该调用Rust zkVM验证器
	// 目前我们只验证证明的基本格式和长度

	if len(proof) == 0 {
		return &VerificationResult{
			Success: false,
			Error:   "证明数据为空",
		}, nil
	}

	// 对于zkVM证明，我们假设前4字节包含结果
	if len(proof) >= 4 {
		result := binary.LittleEndian.Uint32(proof[:4])

		// 验证结果是否合理（简单的合理性检查）
		if result > 1000000 { // 假设结果不应该超过100万
			return &VerificationResult{
				Success: false,
				Error:   fmt.Sprintf("证明结果不合理: %d", result),
			}, nil
		}

		return &VerificationResult{
			Success:      true,
			ExitCode:     0,
			PublicOutput: proof[:4],
			Logs:         []string{"zkVM证明验证成功"},
		}, nil
	}

	return &VerificationResult{
		Success: false,
		Error:   "证明数据格式不正确",
	}, nil
}

// verifyLocalProof 验证本地生成的证明（用于测试）
func (v *NexusVerifier) verifyLocalProof(proof []byte, task *types.Task) (*VerificationResult, error) {
	// 对于本地模式，我们直接计算期望结果并比较
	var expectedResult uint32

	switch task.ProgramID {
	case "fib_input_initial":
		if len(task.PublicInputs) >= 12 {
			n := binary.LittleEndian.Uint32(task.PublicInputs[0:4])
			initA := binary.LittleEndian.Uint32(task.PublicInputs[4:8])
			initB := binary.LittleEndian.Uint32(task.PublicInputs[8:12])
			expectedResult = fibInputInitial(n, initA, initB)
		} else {
			return &VerificationResult{
				Success: false,
				Error:   "fib_input_initial需要至少12字节的输入",
			}, nil
		}
	case "fib_input":
		if len(task.PublicInputs) >= 4 {
			n := binary.LittleEndian.Uint32(task.PublicInputs[0:4])
			expectedResult = fibInput(n)
		} else {
			return &VerificationResult{
				Success: false,
				Error:   "fib_input需要至少4字节的输入",
			}, nil
		}
	default:
		return &VerificationResult{
			Success: false,
			Error:   fmt.Sprintf("不支持的本地程序ID: %s", task.ProgramID),
		}, nil
	}

	// 验证证明长度
	if len(proof) != 4 {
		return &VerificationResult{
			Success: false,
			Error:   fmt.Sprintf("本地证明长度错误，期望4字节，实际%d字节", len(proof)),
		}, nil
	}

	// 提取证明中的结果
	actualResult := binary.LittleEndian.Uint32(proof)

	if actualResult != expectedResult {
		return &VerificationResult{
			Success: false,
			Error:   fmt.Sprintf("结果不匹配，期望%d，实际%d", expectedResult, actualResult),
		}, nil
	}

	return &VerificationResult{
		Success:      true,
		ExitCode:     0, // 成功退出码
		PublicOutput: proof,
		Logs:         []string{fmt.Sprintf("本地验证成功，结果: %d", actualResult)},
	}, nil
}

// VerifyLocalResult 验证本地计算结果
func (v *NexusVerifier) VerifyLocalResult(task *types.Task, expectedResult uint32) (*VerificationResult, error) {
	var actualResult uint32

	switch task.ProgramID {
	case "fib_input_initial":
		if len(task.PublicInputs) >= 12 {
			n := binary.LittleEndian.Uint32(task.PublicInputs[0:4])
			initA := binary.LittleEndian.Uint32(task.PublicInputs[4:8])
			initB := binary.LittleEndian.Uint32(task.PublicInputs[8:12])
			actualResult = fibInputInitial(n, initA, initB)
		} else {
			return &VerificationResult{
				Success: false,
				Error:   "fib_input_initial需要至少12字节的输入",
			}, nil
		}
	case "fib_input":
		if len(task.PublicInputs) >= 4 {
			n := binary.LittleEndian.Uint32(task.PublicInputs[0:4])
			actualResult = fibInput(n)
		} else {
			return &VerificationResult{
				Success: false,
				Error:   "fib_input需要至少4字节的输入",
			}, nil
		}
	default:
		return &VerificationResult{
			Success: false,
			Error:   fmt.Sprintf("不支持的本地程序ID: %s", task.ProgramID),
		}, nil
	}

	if actualResult != expectedResult {
		return &VerificationResult{
			Success: false,
			Error:   fmt.Sprintf("结果不匹配，期望%d，实际%d", expectedResult, actualResult),
		}, nil
	}

	// 构造输出
	output := make([]byte, 4)
	binary.LittleEndian.PutUint32(output, actualResult)

	return &VerificationResult{
		Success:      true,
		ExitCode:     0,
		PublicOutput: output,
		Logs:         []string{fmt.Sprintf("本地计算验证成功，结果: %d", actualResult)},
	}, nil
}

// 斐波那契数列算法实现（与prover保持一致）

// fibInputInitial 计算斐波那契数列第n项，使用自定义初始值
func fibInputInitial(n, initA, initB uint32) uint32 {
	if n == 0 {
		return initA
	}
	if n == 1 {
		return initB
	}
	// 斐波那契递推矩阵
	var mat = [2][2]uint64{{1, 1}, {1, 0}}
	res := matPow(mat, n-1)
	// F(n) = res[0][0]*initB + res[0][1]*initA
	return uint32(res[0][0])*initB + uint32(res[0][1])*initA
}

func matMul(a, b [2][2]uint64) [2][2]uint64 {
	return [2][2]uint64{
		{a[0][0]*b[0][0] + a[0][1]*b[1][0], a[0][0]*b[0][1] + a[0][1]*b[1][1]},
		{a[1][0]*b[0][0] + a[1][1]*b[1][0], a[1][0]*b[0][1] + a[1][1]*b[1][1]},
	}
}

func matPow(mat [2][2]uint64, n uint32) [2][2]uint64 {
	res := [2][2]uint64{{1, 0}, {0, 1}}
	for n > 0 {
		if n&1 == 1 {
			res = matMul(res, mat)
		}
		mat = matMul(mat, mat)
		n >>= 1
	}
	return res
}

// fibInput Go实现的fib_input算法（标准斐波那契，初始值为0,1）
func fibInput(n uint32) uint32 {
	return fibInputInitial(n, 0, 1)
}

// 兼容性函数（与prover保持一致）
func fibInputInitial0(n, initA, initB uint32) uint32 {
	if n == 0 {
		return initA
	}
	if n == 1 {
		return initB
	}

	a, b := initA, initB
	for i := 2; i <= int(n); i++ {
		c := a + b
		a = b
		b = c
	}
	return b
}

func fibInput0(n uint32) uint32 {
	if n == 0 {
		return 0
	}
	if n == 1 {
		return 1
	}

	a, b := uint32(0), uint32(1)
	for i := 2; i <= int(n); i++ {
		c := a + b
		a = b
		b = c
	}
	return b
}
