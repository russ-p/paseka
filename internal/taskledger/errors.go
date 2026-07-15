package taskledger

import "errors"

var (
	ErrTaskNotFound           = errors.New("taskledger: task not found")
	ErrTaskAlreadyReady       = errors.New("taskledger: task already ready")
	ErrTaskCompleted          = errors.New("taskledger: task already completed")
	ErrTaskNotEligible        = errors.New("taskledger: task is not eligible to start")
	ErrDependenciesIncomplete = errors.New("taskledger: task dependencies are not completed")
	ErrNoEligibleTasks        = errors.New("taskledger: no eligible tasks to start")
	ErrTaskNotRetryable       = errors.New("taskledger: task is not eligible to retry")
)
