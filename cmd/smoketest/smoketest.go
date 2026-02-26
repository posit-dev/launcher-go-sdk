package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/posit-dev/launcher-go-sdk/internal/protocol"
)

// Menu action indices. The order matches the C++ smoke test menu.
const (
	actionClusterInfo = iota
	actionGetAllJobs
	actionGetFilteredJobs
	actionGetRunningJobs
	actionGetFinishedJobs
	actionGetJobStatuses
	actionSubmitJob1
	actionSubmitJob2
	actionSubmitStderrJob
	actionSubmitLongJob
	actionStreamOutputBoth
	actionStreamOutputStdout
	actionStreamOutputStderr
	actionStreamResource
	actionGetJobNetwork
	actionCancelJob
	actionKillJob
	actionStopJob
	actionSuspendResumeJob
	actionExit
)

var menuLabels = []string{
	"Get cluster info",
	"Get all jobs",
	"Get filtered jobs",
	"Get running jobs",
	"Get finished jobs",
	"Get job statuses",
	"Submit quick job (matches filter)",
	"Submit quick job 2 (doesn't match filter)",
	"Submit stderr job (doesn't match filter)",
	"Submit long job (matches filter)",
	"Stream last job's output (stdout and stderr)",
	"Stream last job's output (stdout)",
	"Stream last job's output (stderr)",
	"Stream last job's resource utilization (must be running)",
	"Get last job's network information",
	"Submit a slow job and then cancel it",
	"Submit a slow job and then kill it",
	"Submit a slow job and then stop it",
	"Submit a slow job, suspend it, and then resume it",
	"Exit (q or Q also exits)",
}

type smokeTest struct {
	pluginPath string
	username   string
	requestID  uint64 // accessed atomically

	cmd     *exec.Cmd
	stdin   io.WriteCloser
	encoder *protocol.Encoder
	reader  *bufio.Reader

	mu                     sync.Mutex
	cond                   *sync.Cond
	exited                 bool
	responseCount          map[uint64]uint64
	submittedJobIDs        []string
	lastRequestType        int
	outputStreamFinished   bool
	resourceStreamFinished bool

	readerDone chan struct{}
}

func newSmokeTest(pluginPath, username string) *smokeTest {
	st := &smokeTest{
		pluginPath:    pluginPath,
		username:      username,
		responseCount: make(map[uint64]uint64),
		readerDone:    make(chan struct{}),
		reader:        bufio.NewReader(os.Stdin),
	}
	st.cond = sync.NewCond(&st.mu)
	return st
}

func (st *smokeTest) nextID() uint64 {
	return atomic.AddUint64(&st.requestID, 1)
}

func (st *smokeTest) initialize() error {
	args := []string{"--heartbeat-interval-seconds=0", "--enable-debug-logging=1"}
	if os.Getuid() != 0 {
		args = append(args, "--unprivileged=1")
	}

	st.cmd = exec.Command(st.pluginPath, args...) //nolint:gosec // pluginPath is a trusted CLI argument

	var err error
	st.stdin, err = st.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("creating stdin pipe: %w", err)
	}

	stdout, err := st.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating stdout pipe: %w", err)
	}

	st.cmd.Stderr = os.Stderr

	if err := st.cmd.Start(); err != nil {
		return fmt.Errorf("starting plugin: %w", err)
	}

	st.encoder = protocol.NewEncoder(st.stdin, protocol.DefaultMaxMsgSize)
	decoder := protocol.NewDecoder(stdout, protocol.DefaultMaxMsgSize)

	go func() {
		defer close(st.readerDone)
		st.readResponses(decoder)
		err := st.cmd.Wait()
		if err == nil {
			fmt.Println("Plugin exited normally")
		} else {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				fmt.Fprintf(os.Stderr, "Plugin exited with code %d\n", exitErr.ExitCode())
			}
		}
	}()

	// Bootstrap the plugin.
	fmt.Println("Bootstrapping...")
	st.mu.Lock()
	st.responseCount[0] = 0
	st.lastRequestType = msgBootstrap
	st.mu.Unlock()

	if err := st.encoder.Encode(bootstrapReq()); err != nil {
		return fmt.Errorf("sending bootstrap: %w", err)
	}

	if !st.waitForResponse(0, 1) {
		return fmt.Errorf("failed to bootstrap plugin")
	}

	return nil
}

func (st *smokeTest) readResponses(decoder *protocol.Decoder) {
	for decoder.More() {
		raw := decoder.Raw()
		if raw == nil {
			break
		}

		var obj map[string]any
		if err := json.Unmarshal(*raw, &obj); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing response from plugin:\n%v\nResponse:\n%s\n",
				err, string(*raw))
			continue
		}

		pretty, _ := json.MarshalIndent(obj, "", "   ") //nolint:errcheck // best-effort pretty printing
		fmt.Println(string(pretty))

		st.mu.Lock()

		requestID := uint64(0)
		if rid, ok := obj["requestId"]; ok {
			if ridF, ok := rid.(float64); ok {
				requestID = uint64(ridF)
			}
		}

		if _, exists := st.responseCount[requestID]; !exists {
			st.responseCount[requestID] = 0
		}
		st.responseCount[requestID]++

		switch st.lastRequestType {
		case msgSubmitJob:
			if jobs, ok := obj["jobs"]; ok {
				if jobsArr, ok := jobs.([]any); ok {
					for _, j := range jobsArr {
						if jobMap, ok := j.(map[string]any); ok {
							if id, ok := jobMap["id"].(string); ok {
								st.submittedJobIDs = append(st.submittedJobIDs, id)
							}
						}
					}
				}
			}
		case msgGetJobOutput:
			if msgType, ok := obj["messageType"].(float64); ok && int(msgType) == -1 {
				st.outputStreamFinished = true
			} else if complete, ok := obj["complete"].(bool); ok {
				st.outputStreamFinished = complete
			}
		case msgGetJobResourceUtil:
			if msgType, ok := obj["messageType"].(float64); ok && int(msgType) == -1 {
				st.resourceStreamFinished = true
			} else if complete, ok := obj["complete"].(bool); ok {
				st.resourceStreamFinished = complete
			}
		}

		st.mu.Unlock()
		st.cond.Broadcast()
	}

	if err := decoder.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading from plugin: %v\n", err)
	}

	st.mu.Lock()
	st.exited = true
	st.mu.Unlock()
	st.cond.Broadcast()
}

// waitForResponse waits for at least `expected` responses for the given
// request ID. It resets the 30-second timeout each time a new response
// arrives. Returns false on timeout or plugin exit.
func (st *smokeTest) waitForResponse(requestID, expected uint64) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.waitForResponseLocked(requestID, expected)
}

func (st *smokeTest) waitForResponseLocked(requestID, expected uint64) bool {
	lastCount := st.responseCount[requestID]

	for lastCount < expected && !st.exited {
		// Schedule a broadcast after 30 seconds to unblock cond.Wait.
		timer := time.AfterFunc(30*time.Second, func() {
			st.cond.Broadcast()
		})
		st.cond.Wait()
		timer.Stop()

		currentCount := st.responseCount[requestID]
		if currentCount <= lastCount {
			// No new responses since last check.
			fmt.Fprintln(os.Stderr, "Timed out waiting for response.")
			return false
		}
		lastCount = currentCount
	}

	return lastCount >= expected
}

func (st *smokeTest) sendRequest() bool {
	st.mu.Lock()
	if st.exited {
		st.mu.Unlock()
		return false
	}
	st.mu.Unlock()

	fmt.Println("\nActions:")
	for i, label := range menuLabels {
		fmt.Printf("  %2d. %s\n", i+1, label)
	}
	fmt.Print("\nEnter a number: ")

	line, err := st.reader.ReadString('\n')
	if err != nil {
		fmt.Println()
		return false
	}
	line = strings.TrimSpace(line)

	st.mu.Lock()
	if st.exited {
		st.mu.Unlock()
		fmt.Fprintln(os.Stderr, "Plugin exited unexpectedly. Shutting down...")
		return false
	}
	st.mu.Unlock()

	var choice int
	if n, err := strconv.Atoi(line); err == nil {
		choice = n
	} else if line == "q" || line == "Q" {
		choice = len(menuLabels)
	} else {
		fmt.Printf("Invalid choice (%s). Please enter a positive integer.\n", line)
		return true
	}

	if choice < 1 || choice > len(menuLabels) {
		fmt.Printf("Invalid choice (%s). Please enter a positive integer less than %d\n",
			line, len(menuLabels)+1)
		return true
	}

	action := choice - 1
	var success bool

	switch action {
	case actionExit:
		_ = st.stdin.Close() //nolint:errcheck // best-effort cleanup
		return false
	case actionGetJobStatuses:
		success = st.sendJobStatusStreamRequest()
	case actionStreamOutputBoth:
		success = st.sendJobOutputStreamRequest(outputBoth)
	case actionStreamOutputStdout:
		success = st.sendJobOutputStreamRequest(outputStdout)
	case actionStreamOutputStderr:
		success = st.sendJobOutputStreamRequest(outputStderr)
	case actionStreamResource:
		success = st.sendJobResourceStreamRequest()
	case actionCancelJob:
		success = st.sendControlJobRequest(opCancel)
	case actionKillJob:
		success = st.sendControlJobRequest(opKill)
	case actionStopJob:
		success = st.sendControlJobRequest(opStop)
	case actionSuspendResumeJob:
		success = st.sendSuspendResumeJobRequest()
	default:
		success = st.sendSimpleRequest(action)
	}

	st.mu.Lock()
	exited := st.exited
	st.mu.Unlock()
	return !exited && success
}

func (st *smokeTest) sendSimpleRequest(action int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	id := st.nextID()
	var msg map[string]any

	switch action {
	case actionClusterInfo:
		st.lastRequestType = msgGetClusterInfo
		msg = clusterInfoReq(id, st.username)
	case actionGetAllJobs:
		st.lastRequestType = msgGetJob
		msg = allJobsReq(id, st.username)
	case actionGetFilteredJobs:
		st.lastRequestType = msgGetJob
		msg = filteredJobsReq(id, st.username)
	case actionGetRunningJobs:
		st.lastRequestType = msgGetJob
		msg = statusJobsReq(id, st.username, "Running")
	case actionGetFinishedJobs:
		st.lastRequestType = msgGetJob
		msg = statusJobsReq(id, st.username, "Finished")
	case actionSubmitJob1:
		st.lastRequestType = msgSubmitJob
		msg = submitJobMsg(id, st.username, quickJob1(st.username))
	case actionSubmitJob2:
		st.lastRequestType = msgSubmitJob
		msg = submitJobMsg(id, st.username, quickJob2(st.username))
	case actionSubmitStderrJob:
		st.lastRequestType = msgSubmitJob
		msg = submitJobMsg(id, st.username, stderrJob(st.username))
	case actionSubmitLongJob:
		st.lastRequestType = msgSubmitJob
		msg = submitJobMsg(id, st.username, longJob(st.username))
	case actionGetJobNetwork:
		if len(st.submittedJobIDs) == 0 {
			fmt.Println("There are no recently submitted jobs. Choose another option.")
			return true
		}
		st.lastRequestType = msgGetJobNetwork
		msg = networkReqMsg(id, st.submittedJobIDs[len(st.submittedJobIDs)-1], st.username)
	default:
		fmt.Println("Invalid request. Choose another option.")
		return true
	}

	st.responseCount[id] = 0

	if err := st.encoder.Encode(msg); err != nil {
		return st.handleError(err)
	}

	return st.waitForResponseLocked(id, 1)
}

func (st *smokeTest) sendJobStatusStreamRequest() bool {
	id := st.nextID()

	st.mu.Lock()
	// Job status stream responses carry requestId: 0 in the Go SDK.
	st.responseCount[0] = 0
	st.lastRequestType = msgGetJobStatus
	expected := uint64(1)
	if len(st.submittedJobIDs) > 0 {
		expected = uint64(len(st.submittedJobIDs))
	}
	st.mu.Unlock()

	if err := st.encoder.Encode(jobStatusStreamReq(id, st.username)); err != nil {
		return st.handleError(err)
	}

	if !st.waitForResponse(0, expected) {
		st.mu.Lock()
		count := st.responseCount[0]
		jobCount := uint64(len(st.submittedJobIDs))
		st.mu.Unlock()

		if count == 0 {
			fmt.Println("No job status stream response returned. Are there any jobs?")
		} else if count < jobCount {
			fmt.Printf("Received fewer job status stream responses than expected. Actual: %d Expected (minimum): %d\n",
				count, jobCount)
		}
	}

	if err := st.encoder.Encode(cancelJobStatusStreamReq(id, st.username)); err != nil {
		return st.handleError(err)
	}

	// Wait briefly to ensure the stream has time to finish.
	time.Sleep(500 * time.Millisecond)
	return true
}

func (st *smokeTest) sendJobOutputStreamRequest(outputType int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	if len(st.submittedJobIDs) == 0 {
		fmt.Println("There are no recently submitted jobs. Choose another option.")
		return true
	}

	id := st.nextID()
	jobID := st.submittedJobIDs[len(st.submittedJobIDs)-1]
	st.outputStreamFinished = false
	st.responseCount[id] = 0
	st.lastRequestType = msgGetJobOutput

	if err := st.encoder.Encode(outputStreamReq(id, jobID, st.username, outputType)); err != nil {
		return st.handleError(err)
	}

	timedOut := false
	for !timedOut && !st.outputStreamFinished {
		timedOut = !st.waitForResponseLocked(id, st.responseCount[id]+1)
	}

	if timedOut && !st.outputStreamFinished {
		fmt.Println("No output stream response received within the last 30 seconds: canceling...")
		if err := st.encoder.Encode(cancelOutputStreamReq(id, jobID, st.username)); err != nil {
			return st.handleError(err)
		}
	}

	return true
}

func (st *smokeTest) sendJobResourceStreamRequest() bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	if len(st.submittedJobIDs) == 0 {
		fmt.Println("There are no recently submitted jobs. Choose another option.")
		return true
	}

	id := st.nextID()
	jobID := st.submittedJobIDs[len(st.submittedJobIDs)-1]
	st.resourceStreamFinished = false
	// Resource stream responses carry requestId: 0 in the Go SDK.
	st.responseCount[0] = 0
	st.lastRequestType = msgGetJobResourceUtil

	if err := st.encoder.Encode(resourceStreamReq(id, jobID, st.username)); err != nil {
		return st.handleError(err)
	}

	timedOut := false
	for !timedOut && !st.resourceStreamFinished {
		timedOut = !st.waitForResponseLocked(0, st.responseCount[0]+1)
	}

	if timedOut && !st.resourceStreamFinished {
		fmt.Println("No resource util stream response received within the last 30 seconds: canceling...")
		if err := st.encoder.Encode(cancelResourceStreamReq(id, jobID, st.username)); err != nil {
			return st.handleError(err)
		}
	}

	return true
}

func (st *smokeTest) sendControlJobRequest(op int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	currJobCount := len(st.submittedJobIDs)
	id := st.nextID()
	st.responseCount[id] = 0
	st.lastRequestType = msgSubmitJob

	if err := st.encoder.Encode(submitJobMsg(id, st.username, slowSignalJob(st.username))); err != nil {
		return st.handleError(err)
	}

	if !st.waitForResponseLocked(id, 1) || currJobCount >= len(st.submittedJobIDs) {
		return false
	}

	return st.controlLastJobAndPrintStatus(op)
}

// controlLastJobAndPrintStatus sends a control operation to the most recently
// submitted job, then queries and prints its status. Caller must hold st.mu.
func (st *smokeTest) controlLastJobAndPrintStatus(op int) bool {
	// Give the job a moment to start, unless we're canceling a pending job.
	if op != opCancel {
		st.mu.Unlock()
		time.Sleep(1 * time.Second)
		st.mu.Lock()
	}

	jobID := st.submittedJobIDs[len(st.submittedJobIDs)-1]

	// Send control request.
	controlID := st.nextID()
	st.responseCount[controlID] = 0
	st.lastRequestType = msgControlJob

	if err := st.encoder.Encode(controlJobMsg(controlID, jobID, st.username, op)); err != nil {
		return st.handleError(err)
	}

	if !st.waitForResponseLocked(controlID, 1) {
		return false
	}

	// Get job status to confirm the operation.
	statusID := st.nextID()
	st.responseCount[statusID] = 0
	st.lastRequestType = msgGetJob

	if err := st.encoder.Encode(getJobReq(statusID, jobID, st.username)); err != nil {
		return st.handleError(err)
	}

	return st.waitForResponseLocked(statusID, 1)
}

func (st *smokeTest) sendSuspendResumeJobRequest() bool {
	if !st.sendControlJobRequest(opSuspend) {
		return false
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	return st.controlLastJobAndPrintStatus(opResume)
}

func (st *smokeTest) handleError(err error) bool {
	fmt.Fprintf(os.Stderr, "Error communicating with plugin: %v\n", err)
	return false
}

func (st *smokeTest) stop() {
	st.mu.Lock()
	st.exited = true
	st.mu.Unlock()
	st.cond.Broadcast()

	_ = st.stdin.Close() //nolint:errcheck // best-effort cleanup

	select {
	case <-st.readerDone:
	case <-time.After(30 * time.Second):
		fmt.Fprintln(os.Stderr, "Plugin did not exit in time, killing...")
		_ = st.cmd.Process.Kill() //nolint:errcheck // best-effort cleanup
		<-st.readerDone
	}
}
