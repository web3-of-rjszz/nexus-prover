#ifndef NEXUS_VERIFIER_H
#define NEXUS_VERIFIER_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// 验证输入结构
typedef struct {
    const char* program_id;
    const char* task_id;
    const unsigned char* public_inputs;
    size_t public_inputs_len;
    const unsigned char* proof_data;
    size_t proof_len;
} VerificationInput;

// 验证结果结构
typedef struct {
    int success;
    char* error_message;
    uint32_t exit_code;
    unsigned char* public_output;
    size_t public_output_len;
    char** logs;
    size_t logs_len;
} VerificationResult;

// 验证证明函数
VerificationResult verify_proof_c(VerificationInput input);

// 释放验证结果内存
void free_verification_result(VerificationResult result);

#ifdef __cplusplus
}
#endif

#endif // NEXUS_VERIFIER_H 