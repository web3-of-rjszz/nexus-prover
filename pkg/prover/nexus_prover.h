#include <stdbool.h>
#include <stdint.h>
#include <stddef.h>
typedef struct ProverResult {
  bool success;
  char *error_message;
  uint8_t *proof_data;
  uintptr_t proof_len;
} ProverResult;

typedef struct TaskInput {
  const char *program_id;
  const char *task_id;
  const uint8_t *public_inputs;
  uintptr_t public_inputs_len;
} TaskInput;

struct ProverResult prove_anonymously_c(void);

struct ProverResult prove_authenticated_c(struct TaskInput task);

void free_prover_result(struct ProverResult result);
