// Package plugintest provides testing utilities for launcher plugins.
//
// This package includes mock implementations of ResponseWriter and StreamResponseWriter,
// fluent builders for constructing test jobs, and assertion helpers for common test scenarios.
//
// # Mock Response Writers
//
// MockResponseWriter and MockStreamResponseWriter capture all responses written by a plugin,
// allowing tests to make assertions about what the plugin did:
//
//	func TestSubmitJob(t *testing.T) {
//		w := plugintest.NewMockResponseWriter()
//		plugin := NewMyPlugin()
//
//		job := plugintest.NewJob().
//			WithUser("testuser").
//			WithCommand("echo hello").
//			Build()
//
//		plugin.SubmitJob(w, "testuser", job)
//
//		plugintest.AssertNoError(t, w)
//		plugintest.AssertJobCount(t, w, 1)
//	}
//
// # Job Builders
//
// JobBuilder provides a fluent API for constructing test jobs:
//
//	job := plugintest.NewJob().
//		WithID("job-123").
//		WithUser("alice").
//		WithCommand("python train.py").
//		WithEnv("MODEL_PATH", "/models/v1").
//		WithTag("ml-training").
//		Running().
//		Build()
//
// # Test Helpers
//
// The package provides assertion helpers for common test scenarios:
//
//	plugintest.AssertNoError(t, w)
//	plugintest.AssertErrorCode(t, w, api.CodeJobNotFound)
//	plugintest.AssertJobStatus(t, job, api.StatusRunning)
//	plugintest.AssertStreamClosed(t, streamWriter)
//
// # Best Practices
//
// When testing plugins:
//
//  1. Use builders to create test data - they provide sensible defaults and a clear API
//  2. Use mock writers to capture plugin responses - avoid testing internal state
//  3. Use assertion helpers for clear, consistent test failures
//  4. Test both success and error paths
//  5. Test streaming methods with context cancellation
//
// # Example Test
//
//	func TestGetJob(t *testing.T) {
//		// Arrange: set up test data
//		cache, _ := cache.NewJobCache(context.Background(), logger)
//		job := plugintest.NewJob().
//			WithID("test-1").
//			WithUser("alice").
//			Running().
//			Build()
//		cache.Put(job)
//
//		plugin := &MyPlugin{cache: cache}
//		w := plugintest.NewMockResponseWriter()
//
//		// Act: call the plugin method
//		plugin.GetJob(w, "alice", "test-1", nil)
//
//		// Assert: verify the response
//		plugintest.AssertNoError(t, w)
//		plugintest.AssertJobCount(t, w, 1)
//		returnedJob := plugintest.FindJobByID(w.AllJobs(), "test-1")
//		plugintest.AssertJobStatus(t, returnedJob, api.StatusRunning)
//	}
package plugintest
