package audit

import (
	"sort"
	"sync"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// RiskFactorCount represents a risk factor and its occurrence count
type RiskFactorCount struct {
	Factor string
	Count  int
}

// RiskStatistics tracks command execution statistics by risk factors
type RiskStatistics struct {
	mu               sync.RWMutex
	totalCommands    int
	riskLevelCounts  map[runnertypes.RiskLevel]int
	riskFactorCounts map[string]int
	commandsByRisk   map[runnertypes.RiskLevel]map[string]bool
}

// NewRiskStatistics creates a new risk statistics tracker
func NewRiskStatistics() *RiskStatistics {
	return &RiskStatistics{
		riskLevelCounts:  make(map[runnertypes.RiskLevel]int),
		riskFactorCounts: make(map[string]int),
		commandsByRisk:   make(map[runnertypes.RiskLevel]map[string]bool),
	}
}

// RecordCommand records a command execution with its risk level and factors
func (s *RiskStatistics) RecordCommand(commandName string, riskLevel runnertypes.RiskLevel, riskFactors []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalCommands++
	s.riskLevelCounts[riskLevel]++

	// Track risk factors
	for _, factor := range riskFactors {
		if factor != "" {
			s.riskFactorCounts[factor]++
		}
	}

	// Track commands by risk level
	if s.commandsByRisk[riskLevel] == nil {
		s.commandsByRisk[riskLevel] = make(map[string]bool)
	}
	s.commandsByRisk[riskLevel][commandName] = true
}

// TotalCommands returns the total number of commands recorded
func (s *RiskStatistics) TotalCommands() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totalCommands
}

// GetRiskLevelCounts returns the count of commands by risk level
func (s *RiskStatistics) GetRiskLevelCounts() map[runnertypes.RiskLevel]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create a copy to avoid race conditions
	counts := make(map[runnertypes.RiskLevel]int, len(s.riskLevelCounts))
	for level, count := range s.riskLevelCounts {
		counts[level] = count
	}
	return counts
}

// GetTopRiskFactors returns the most common risk factors up to the specified limit
func (s *RiskStatistics) GetTopRiskFactors(limit int) []RiskFactorCount {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Convert map to slice
	factors := make([]RiskFactorCount, 0, len(s.riskFactorCounts))
	for factor, count := range s.riskFactorCounts {
		factors = append(factors, RiskFactorCount{
			Factor: factor,
			Count:  count,
		})
	}

	// Sort by count (descending), then by factor name (ascending) for deterministic order
	sort.Slice(factors, func(i, j int) bool {
		if factors[i].Count != factors[j].Count {
			return factors[i].Count > factors[j].Count
		}
		return factors[i].Factor < factors[j].Factor
	})

	// Apply limit
	if limit > 0 && limit < len(factors) {
		return factors[:limit]
	}
	return factors
}

// GetCommandsByRiskLevel returns unique command names for a given risk level
func (s *RiskStatistics) GetCommandsByRiskLevel(riskLevel runnertypes.RiskLevel) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	commands := make([]string, 0)
	if cmdMap, exists := s.commandsByRisk[riskLevel]; exists {
		for cmd := range cmdMap {
			commands = append(commands, cmd)
		}
		sort.Strings(commands)
	}
	return commands
}
