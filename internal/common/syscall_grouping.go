package common

import (
	"cmp"
	"slices"
)

// GroupAndSortSyscalls groups a slice of SyscallInfo entries by syscall number,
// merging Occurrences from entries that share the same Number.
// When merging, a non-empty Name is preferred over an empty one, and IsNetwork
// is set to true if any entry for that number has IsNetwork=true.
// Each group's Occurrences are sorted by Location in ascending order.
// Groups are sorted by Number in ascending order, with Number=-1 placed last.
func GroupAndSortSyscalls(infos []SyscallInfo) []SyscallInfo {
	if len(infos) == 0 {
		return nil
	}

	groups := make(map[int]*SyscallInfo)
	var numberOrder []int
	seenNumber := make(map[int]bool)

	for _, info := range infos {
		if !seenNumber[info.Number] {
			seenNumber[info.Number] = true
			numberOrder = append(numberOrder, info.Number)
		}
		group, exists := groups[info.Number]
		if !exists {
			group = &SyscallInfo{
				Number:      info.Number,
				Name:        info.Name,
				IsNetwork:   info.IsNetwork,
				Occurrences: make([]SyscallOccurrence, 0),
			}
			groups[info.Number] = group
		} else {
			if group.Name == "" && info.Name != "" {
				group.Name = info.Name
			}
			if info.IsNetwork {
				group.IsNetwork = true
			}
		}
		group.Occurrences = append(group.Occurrences, info.Occurrences...)
	}

	// Sort each group's Occurrences by Location
	for _, group := range groups {
		slices.SortStableFunc(group.Occurrences, func(a, b SyscallOccurrence) int {
			return cmp.Compare(a.Location, b.Location)
		})
	}

	// Sort number groups: ascending order, with -1 at the end
	slices.SortStableFunc(numberOrder, func(ni, nj int) int {
		if ni == -1 && nj == -1 {
			return 0
		}
		if ni == -1 {
			return 1
		}
		if nj == -1 {
			return -1
		}
		return cmp.Compare(ni, nj)
	})

	result := make([]SyscallInfo, 0, len(groups))
	for _, num := range numberOrder {
		result = append(result, *groups[num])
	}
	return result
}
