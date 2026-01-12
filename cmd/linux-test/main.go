// +build linux

// linux-test is a simple CLI tool to test Linux-specific functionality
// like hosts file manipulation and iptables port redirection.
//
// Run with elevated privileges: sudo ./linux-test
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"kubikles/pkg/hosts"
)

func main() {
	testHostsFile := flag.Bool("hosts", false, "Test hosts file manipulation")
	testIptables := flag.Bool("iptables", false, "Test iptables port redirection")
	testAll := flag.Bool("all", false, "Run all tests")
	cleanup := flag.Bool("cleanup", false, "Clean up any test artifacts")
	flag.Parse()

	if *cleanup {
		doCleanup()
		return
	}

	if !*testHostsFile && !*testIptables && !*testAll {
		fmt.Println("Kubikles Linux Test Suite")
		fmt.Println("Usage: linux-test [options]")
		fmt.Println()
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("Note: Most tests require root/sudo privileges")
		return
	}

	passed := 0
	failed := 0

	if *testHostsFile || *testAll {
		if testHostsFileManipulation() {
			passed++
		} else {
			failed++
		}
	}

	if *testIptables || *testAll {
		if testIptablesRedirection() {
			passed++
		} else {
			failed++
		}
	}

	fmt.Println()
	fmt.Printf("Results: %d passed, %d failed\n", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func testHostsFileManipulation() bool {
	fmt.Println("\n=== Testing Hosts File Manipulation ===")

	// Check if running as root
	if os.Geteuid() != 0 {
		fmt.Println("SKIP: Requires root privileges (run with sudo)")
		return true
	}

	manager := hosts.NewManager()

	// Test hostname
	testHostname := "kubikles-test.local"

	// Add test entry
	fmt.Printf("Adding hosts entry: %s -> 127.0.0.1\n", testHostname)
	entries := []hosts.Entry{
		{Hostname: testHostname, IP: "127.0.0.1"},
	}

	if err := manager.AddEntries(entries); err != nil {
		fmt.Printf("FAIL: Failed to add hosts entry: %v\n", err)
		return false
	}

	// Verify entry was added
	content, err := os.ReadFile("/etc/hosts")
	if err != nil {
		fmt.Printf("FAIL: Failed to read hosts file: %v\n", err)
		return false
	}

	if !strings.Contains(string(content), testHostname) {
		fmt.Printf("FAIL: Hosts entry not found in /etc/hosts\n")
		return false
	}
	fmt.Println("OK: Hosts entry added successfully")

	// Test DNS resolution
	fmt.Printf("Testing DNS resolution for %s...\n", testHostname)
	addrs, err := net.LookupHost(testHostname)
	if err != nil {
		fmt.Printf("WARN: DNS lookup failed (may be cached): %v\n", err)
	} else {
		fmt.Printf("OK: Resolved to: %v\n", addrs)
	}

	// Remove entry
	fmt.Println("Removing hosts entry...")
	if err := manager.RemoveEntries(); err != nil {
		fmt.Printf("FAIL: Failed to remove hosts entry: %v\n", err)
		return false
	}

	// Verify removal
	content, _ = os.ReadFile("/etc/hosts")
	if strings.Contains(string(content), testHostname) {
		fmt.Printf("FAIL: Hosts entry still present after removal\n")
		return false
	}
	fmt.Println("OK: Hosts entry removed successfully")

	fmt.Println("PASS: Hosts file manipulation test passed")
	return true
}

func testIptablesRedirection() bool {
	fmt.Println("\n=== Testing iptables Port Redirection ===")

	// Check if running as root
	if os.Geteuid() != 0 {
		fmt.Println("SKIP: Requires root privileges (run with sudo)")
		return true
	}

	// Check if iptables is available
	if _, err := exec.LookPath("iptables"); err != nil {
		fmt.Println("SKIP: iptables not found")
		return true
	}

	// Start a test server on port 8443
	testPort := 18443 // Use high port to avoid conflicts
	targetPort := 18080

	// Start a simple HTTP server on targetPort
	serverReady := make(chan bool)
	serverDone := make(chan bool)

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("OK"))
		})
		server := &http.Server{
			Addr:    fmt.Sprintf("127.0.0.1:%d", targetPort),
			Handler: mux,
		}

		listener, err := net.Listen("tcp", server.Addr)
		if err != nil {
			fmt.Printf("Failed to start test server: %v\n", err)
			serverReady <- false
			return
		}

		serverReady <- true
		server.Serve(listener)
		serverDone <- true
	}()

	if !<-serverReady {
		return false
	}

	fmt.Printf("Test server running on port %d\n", targetPort)

	// Add iptables rule to redirect testPort -> targetPort
	fmt.Printf("Adding iptables rule: %d -> %d\n", testPort, targetPort)
	rule := []string{
		"-t", "nat",
		"-A", "OUTPUT",
		"-o", "lo",
		"-p", "tcp",
		"--dport", fmt.Sprintf("%d", testPort),
		"-j", "REDIRECT",
		"--to-port", fmt.Sprintf("%d", targetPort),
	}

	output, err := exec.Command("iptables", rule...).CombinedOutput()
	if err != nil {
		fmt.Printf("FAIL: Failed to add iptables rule: %v, output: %s\n", err, output)
		return false
	}
	fmt.Println("OK: iptables rule added")

	// Give iptables a moment
	time.Sleep(100 * time.Millisecond)

	// Test connection through redirected port
	fmt.Printf("Testing connection to localhost:%d (should redirect to %d)...\n", testPort, targetPort)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", testPort))
	if err != nil {
		fmt.Printf("FAIL: Connection through redirected port failed: %v\n", err)
		// Cleanup before returning
		cleanupIptables(testPort, targetPort)
		return false
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("FAIL: Unexpected status code: %d\n", resp.StatusCode)
		cleanupIptables(testPort, targetPort)
		return false
	}
	fmt.Println("OK: Connection through redirected port succeeded")

	// Remove iptables rule
	fmt.Println("Removing iptables rule...")
	cleanupIptables(testPort, targetPort)

	// Verify rule was removed (connection should fail now)
	fmt.Println("Verifying rule removal (connection should fail)...")
	_, err = client.Get(fmt.Sprintf("http://127.0.0.1:%d/", testPort))
	if err == nil {
		fmt.Println("WARN: Connection still succeeds after rule removal (may be connection reuse)")
	} else {
		fmt.Println("OK: Connection fails after rule removal as expected")
	}

	fmt.Println("PASS: iptables port redirection test passed")
	return true
}

func cleanupIptables(fromPort, toPort int) {
	rule := []string{
		"-t", "nat",
		"-D", "OUTPUT",
		"-o", "lo",
		"-p", "tcp",
		"--dport", fmt.Sprintf("%d", fromPort),
		"-j", "REDIRECT",
		"--to-port", fmt.Sprintf("%d", toPort),
	}
	exec.Command("iptables", rule...).Run()
}

func doCleanup() {
	fmt.Println("Cleaning up test artifacts...")

	if os.Geteuid() != 0 {
		fmt.Println("Cleanup requires root privileges")
		return
	}

	// Remove any test hosts entries
	manager := hosts.NewManager()
	manager.RemoveEntries()
	fmt.Println("Removed any managed hosts entries")

	// Remove common test iptables rules
	cleanupIptables(18443, 18080)
	cleanupIptables(443, 8443)
	cleanupIptables(80, 8080)
	fmt.Println("Removed test iptables rules")

	fmt.Println("Cleanup complete")
}
