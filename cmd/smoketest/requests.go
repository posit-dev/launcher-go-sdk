package main

import "github.com/posit-dev/launcher-go-sdk/api"

// Message type constants matching the Launcher Plugin API protocol.
const (
	msgBootstrap          = 1
	msgSubmitJob          = 2
	msgGetJob             = 3
	msgGetJobStatus       = 4
	msgControlJob         = 5
	msgGetJobOutput       = 6
	msgGetJobResourceUtil = 7
	msgGetJobNetwork      = 8
	msgGetClusterInfo     = 9
)

// Control operation constants.
const (
	opSuspend = 0
	opResume  = 1
	opStop    = 2
	opKill    = 3
	opCancel  = 4
)

// Output type constants.
const (
	outputStdout = 0
	outputStderr = 1
	outputBoth   = 2
)

func bootstrapReq() map[string]any {
	return map[string]any{
		"messageType": msgBootstrap,
		"requestId":   uint64(0),
		"version":     api.APIVersion,
	}
}

func clusterInfoReq(id uint64, user string) map[string]any {
	return map[string]any{
		"messageType":     msgGetClusterInfo,
		"requestId":       id,
		"username":        user,
		"requestUsername": user,
	}
}

func allJobsReq(id uint64, user string) map[string]any {
	return map[string]any{
		"messageType":     msgGetJob,
		"requestId":       id,
		"username":        user,
		"requestUsername": user,
		"jobId":           "*",
		"encodedJobId":    "",
	}
}

func getJobReq(id uint64, jobID, user string) map[string]any {
	return map[string]any{
		"messageType":     msgGetJob,
		"requestId":       id,
		"username":        user,
		"requestUsername": user,
		"jobId":           jobID,
		"encodedJobId":    "",
	}
}

func filteredJobsReq(id uint64, user string) map[string]any {
	return map[string]any{
		"messageType":     msgGetJob,
		"requestId":       id,
		"username":        user,
		"requestUsername": user,
		"jobId":           "*",
		"encodedJobId":    "",
		"tags":            []string{"filter job"},
	}
}

func statusJobsReq(id uint64, user, status string) map[string]any {
	return map[string]any{
		"messageType":     msgGetJob,
		"requestId":       id,
		"username":        user,
		"requestUsername": user,
		"jobId":           "*",
		"encodedJobId":    "",
		"statuses":        []string{status},
	}
}

func jobStatusStreamReq(id uint64, user string) map[string]any {
	return map[string]any{
		"messageType":     msgGetJobStatus,
		"requestId":       id,
		"username":        user,
		"requestUsername": user,
		"jobId":           "*",
		"encodedJobId":    "",
		"cancel":          false,
	}
}

func cancelJobStatusStreamReq(id uint64, user string) map[string]any {
	return map[string]any{
		"messageType":     msgGetJobStatus,
		"requestId":       id,
		"username":        user,
		"requestUsername": user,
		"jobId":           "*",
		"encodedJobId":    "",
		"cancel":          true,
	}
}

func controlJobMsg(id uint64, jobID, user string, op int) map[string]any {
	return map[string]any{
		"messageType":     msgControlJob,
		"requestId":       id,
		"username":        user,
		"requestUsername": user,
		"jobId":           jobID,
		"encodedJobId":    "",
		"operation":       op,
	}
}

func submitJobMsg(id uint64, user string, job *api.Job) map[string]any {
	return map[string]any{
		"messageType":     msgSubmitJob,
		"requestId":       id,
		"username":        user,
		"requestUsername": user,
		"job":             job,
	}
}

func outputStreamReq(id uint64, jobID, user string, outputType int) map[string]any {
	return map[string]any{
		"messageType":     msgGetJobOutput,
		"requestId":       id,
		"username":        user,
		"requestUsername": user,
		"jobId":           jobID,
		"encodedJobId":    "",
		"outputType":      outputType,
		"cancel":          false,
	}
}

func cancelOutputStreamReq(id uint64, jobID, user string) map[string]any {
	return map[string]any{
		"messageType":     msgGetJobOutput,
		"requestId":       id,
		"username":        user,
		"requestUsername": user,
		"jobId":           jobID,
		"encodedJobId":    "",
		"cancel":          true,
	}
}

func resourceStreamReq(id uint64, jobID, user string) map[string]any {
	return map[string]any{
		"messageType":     msgGetJobResourceUtil,
		"requestId":       id,
		"username":        user,
		"requestUsername": user,
		"jobId":           jobID,
		"encodedJobId":    "",
		"cancel":          false,
	}
}

func cancelResourceStreamReq(id uint64, jobID, user string) map[string]any {
	return map[string]any{
		"messageType":     msgGetJobResourceUtil,
		"requestId":       id,
		"username":        user,
		"requestUsername": user,
		"jobId":           jobID,
		"encodedJobId":    "",
		"cancel":          true,
	}
}

func networkReqMsg(id uint64, jobID, user string) map[string]any {
	return map[string]any{
		"messageType":     msgGetJobNetwork,
		"requestId":       id,
		"username":        user,
		"requestUsername": user,
		"jobId":           jobID,
		"encodedJobId":    "",
	}
}

// Job definitions matching the C++ smoke test.

func quickJob1(user string) *api.Job {
	return &api.Job{
		User:  user,
		Exe:   "/bin/sh",
		Env:   []api.Env{{Name: "ENV_VAR", Value: "This is an environment variable!"}},
		Stdin: "#!/bin/sh\necho $ENV_VAR",
		Name:  "Quick Job 1",
		Tags:  []string{"filter job"},
	}
}

func quickJob2(user string) *api.Job {
	return &api.Job{
		User:    user,
		Command: "echo",
		Args:    []string{"This is a shell command."},
		Env:     []api.Env{{Name: "ENV_VAR", Value: "This is not used!"}},
		Name:    "Quick Job 2",
		Tags:    []string{"other tag"},
	}
}

func longJob(user string) *api.Job {
	return &api.Job{
		User:  user,
		Exe:   "/bin/bash",
		Stdin: "#!/bin/bash\nset -e\nfor I in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24; do\n  echo \"$I...\"\n  sleep $I\ndone",
		Name:  "Slow job",
		Tags:  []string{"filter job"},
	}
}

func stderrJob(user string) *api.Job {
	return &api.Job{
		User:    user,
		Command: "grep",
		Name:    "Stderr job",
		Tags:    []string{"other", "tags", "filter", "job"},
	}
}

func slowSignalJob(user string) *api.Job {
	return &api.Job{
		User:    user,
		Command: "sleep 20 && echo Done",
		Name:    "Job for signaling",
		Tags:    []string{"signal"},
	}
}
