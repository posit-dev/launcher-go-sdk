// Smoke test is a manual CLI utility for exercising basic code paths of
// Launcher Plugins built with the Go SDK.
package main

import (
	"fmt"
	"os"
	"os/user"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr,
			"Unexpected number of arguments: %d\nUsage: %s <path/to/plugin/exe> <request user>\n",
			len(os.Args), os.Args[0])
		os.Exit(1)
	}

	pluginPath := os.Args[1]
	username := os.Args[2]

	if _, err := user.Lookup(username); err != nil {
		fmt.Fprintf(os.Stderr,
			"User %q could not be found. Please ensure that it exists.\nError: %v\n",
			username, err)
		os.Exit(1)
	}

	st := newSmokeTest(pluginPath, username)
	if err := st.initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "An error occurred while initializing:\n%v\n", err)
		os.Exit(1)
	}
	defer st.stop()

	for st.sendRequest() {
	}
}
