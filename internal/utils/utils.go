package utils

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"time"
)

// LogWithTime 带时间戳的日志函数
func LogWithTime(format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)
	fmt.Printf("[%s] %s\n", timestamp, message)
}

// SleepWithContext 可中断的睡眠函数
func SleepWithContext(ctx context.Context, duration time.Duration) bool {
	select {
	case <-time.After(duration):
		return true
	case <-ctx.Done():
		return false
	}
}

// IsRateLimitError 检查是否是速率限制错误
func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "rate limit exceeded") ||
		strings.Contains(errStr, "Rate limit exceeded") ||
		strings.Contains(errStr, "429")
}

// ClearProofData 清理证明数据，帮助GC回收内存
func ClearProofData(proof []byte) {
	if proof != nil {
		// 将证明数据清零，帮助GC更快回收
		for i := range proof {
			proof[i] = 0
		}
	}
}

// GetProcMemUsage 获取进程真实物理内存（MB）
func GetProcMemUsage() float64 {
	data, err := ioutil.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				// VmRSS:   123456 kB
				var kb int
				fmt.Sscanf(fields[1], "%d", &kb)
				return float64(kb) / 1024.0
			}
		}
	}
	return 0
}
