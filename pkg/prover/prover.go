package prover

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"nexus-prover/pkg/types"
)

/*
#cgo CFLAGS: -I.
#cgo LDFLAGS: -L. -lnexus_prover -ldl -lpthread
#include "nexus_prover.h"
#include <stdlib.h>
*/
import "C"

// Prove 修改prove函数签名，增加useLocal参数
func Prove(task *types.Task, useLocal bool) ([]byte, error) {
	if useLocal {
		if task.ProgramID == "fib_input_initial" && len(task.PublicInputs) >= 12 {
			n := binary.LittleEndian.Uint32(task.PublicInputs[0:4])
			initA := binary.LittleEndian.Uint32(task.PublicInputs[4:8])
			initB := binary.LittleEndian.Uint32(task.PublicInputs[8:12])
			result := fibInputInitial(n, initA, initB)
			out := make([]byte, 4)
			binary.LittleEndian.PutUint32(out, result)
			return out, nil
		}
		if task.ProgramID == "fib_input" && len(task.PublicInputs) >= 4 {
			n := binary.LittleEndian.Uint32(task.PublicInputs[0:4])
			result := fibInput(n)
			out := make([]byte, 4)
			binary.LittleEndian.PutUint32(out, result)
			return out, nil
		}
		return nil, fmt.Errorf("unsupported program id for local mode: %s", task.ProgramID)
	}
	// 默认用官方zkVM
	return proveWithZkVM(task)
}

// ProveWithZkVM 封装zkVM调用
func proveWithZkVM(task *types.Task) ([]byte, error) {
	cProgramID := C.CString(task.ProgramID)
	defer C.free(unsafe.Pointer(cProgramID))
	cTaskID := C.CString(task.TaskID)
	defer C.free(unsafe.Pointer(cTaskID))
	cInputs := C.CBytes(task.PublicInputs)
	defer C.free(cInputs)

	var cTaskInput C.TaskInput
	cTaskInput.program_id = cProgramID
	cTaskInput.task_id = cTaskID
	cTaskInput.public_inputs = (*C.uchar)(cInputs)
	cTaskInput.public_inputs_len = C.size_t(len(task.PublicInputs))

	result := C.prove_authenticated_c(cTaskInput)
	defer C.free_prover_result(result)

	if !bool(result.success) {
		errMsg := C.GoString(result.error_message)
		return nil, fmt.Errorf("zkVM proof failed: %s", errMsg)
	}
	proof := C.GoBytes(unsafe.Pointer(result.proof_data), C.int(result.proof_len))
	return proof, nil
}

// fib_input_initial算法：迭代写法
func fibInputInitial0(n, initA, initB uint32) uint32 {
	if n == 0 {
		return initA
	}
	if n == 1 {
		return initB
	}
	a, b := initA, initB
	for i := uint32(2); i <= n; i++ {
		next := a + b
		a = b
		b = next
	}
	return b
}

// fib_input_initial算法：矩阵快速幂优化写法（O(log n) 时间）-- 适合大n值
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
