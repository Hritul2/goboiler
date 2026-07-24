package asyncprocessor

import "github.com/hibiken/asynq"

// ListArchivedTasks returns tasks that exhausted their retry policy for the
// given queue — asynq's "archive" is its dead-letter-queue equivalent.
//
// Go-API only, deliberately no HTTP route: task payloads may contain PII
// (e.g. the welcome-email task's recipient address) and this codebase has
// no auth yet. Wire an HTTP route once auth lands, gated appropriately.
func (j *JobService) ListArchivedTasks(queue string) ([]*asynq.TaskInfo, error) {
	return j.inspector.ListArchivedTasks(queue)
}
