// Package main is a test program for security analysis functions
package main

import (
	"fmt"
	"os/exec"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

func main() {
	// Test sudo command
	sudoPath, _ := exec.LookPath("sudo")
	riskLevel, pattern, reason := security.AnalyzeCommandSecurityWithResolvedPath(sudoPath, []string{"systemctl", "restart", "nginx"})
	fmt.Printf("sudo: Risk: %s, Pattern: %s, Reason: %s\n", riskLevel, pattern, reason)

	// Test rm command
	rmPath, _ := exec.LookPath("rm")
	riskLevel2, pattern2, reason2 := security.AnalyzeCommandSecurityWithResolvedPath(rmPath, []string{"-rf", "/tmp/*"})
	fmt.Printf("rm: Risk: %s, Pattern: %s, Reason: %s\n", riskLevel2, pattern2, reason2)

	// Test systemctl command
	systemctlPath, _ := exec.LookPath("systemctl")
	riskLevel3, pattern3, reason3 := security.AnalyzeCommandSecurityWithResolvedPath(systemctlPath, []string{"restart", "nginx"})
	fmt.Printf("systemctl: Risk: %s, Pattern: %s, Reason: %s\n", riskLevel3, pattern3, reason3)
}
