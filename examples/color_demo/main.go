// Command-line demo for the internal/color package.
package main

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/color"
)

// Example demonstrating the new color API usage
func main() {
	const errorCount = 5
	const successRate = 95.5

	fmt.Println("=== Color API Demo ===")
	fmt.Println()

	// Basic color usage
	fmt.Println("Basic colors:")
	fmt.Println(color.Red("ERROR: This is a critical error"))
	fmt.Println(color.Yellow("WARN: This is a warning"))
	fmt.Println(color.Green("INFO: This is informational"))
	fmt.Println(color.Gray("DEBUG: This is debug information"))
	fmt.Println()

	// Additional colors
	fmt.Println("Additional colors:")
	fmt.Println(color.Blue("BLUE: Blue text for information"))
	fmt.Println(color.Purple("PURPLE: Purple text for special cases"))
	fmt.Println(color.Cyan("CYAN: Cyan text for hints"))
	fmt.Println(color.White("WHITE: White text for emphasis"))
	fmt.Println()

	// Using Sprintf for formatted strings
	fmt.Println("Formatted strings:")
	fmt.Println(color.Red.Sprintf("Error count: %d", errorCount))
	fmt.Println(color.Green.Sprintf("Success rate: %.1f%%", successRate))
	fmt.Println()

	// Conditional color support
	fmt.Println("Conditional colors:")
	colorEnabled := true
	colorDisabled := false

	conditionalRed := color.ConditionalColor(color.Red, colorEnabled)
	conditionalGray := color.ConditionalColor(color.Gray, colorDisabled)

	fmt.Printf("With color enabled: %s\n", conditionalRed("This should be red"))
	fmt.Printf("With color disabled: %s\n", conditionalGray("This should be plain"))
	fmt.Println()

	// NoColor function
	fmt.Println("Plain text (no color):")
	fmt.Println(color.NoColor("This text has no color formatting"))
	fmt.Println()

	// Demonstration that colors don't interfere with each other
	fmt.Println("Multiple colors in sequence:")
	fmt.Print(color.Red("RED"))
	fmt.Print(" -> ")
	fmt.Print(color.Green("GREEN"))
	fmt.Print(" -> ")
	fmt.Print(color.Blue("BLUE"))
	fmt.Println(" (Colors are properly reset)")
	fmt.Println()

	fmt.Println("=== Demo Complete ===")
}
