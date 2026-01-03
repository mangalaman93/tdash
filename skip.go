package main

var (
	lowFrequencyCells = map[int]map[int]int{
		0:  {0: 0, 4: 0, 5: 0, 6: 0, 9: 0, 10: 0, 20: 0},
		1:  {0: 0, 1: 0, 10: 0},
		2:  {20: 0},
		3:  {0: 0, 13: 0, 17: 0},
		4:  {14: 0, 20: 0},
		5:  {20: 0},
		6:  {6: 0, 8: 0, 18: 0},
		7:  {20: 0},
		8:  {0: 1, 1: 1, 2: 1},
		9:  {0: 1, 1: 1, 2: 1, 3: 1, 4: 1, 16: 1},
		10: {0: 1, 1: 1, 11: 1, 12: 1, 13: 1, 14: 1},
		11: {3: 1, 5: 1, 6: 1, 8: 1, 10: 1, 11: 1},
		12: {3: 2, 5: 2, 6: 2, 7: 2, 9: 2, 12: 2, 13: 2, 20: 2},
		13: {0: 2, 1: 2, 3: 2, 9: 2, 16: 2, 17: 2, 20: 2},
		14: {0: 2, 5: 2, 12: 2, 13: 2, 14: 2, 17: 2},
	}
)

func shouldSkip(x, y int) bool {
	inner, exists := lowFrequencyCells[x]
	if !exists {
		return false
	}

	skipCount, exists := inner[y]
	if !exists {
		return false
	}

	skipCount++
	if skipCount >= 3 {
		lowFrequencyCells[x][y] = 0
		return false
	}

	lowFrequencyCells[x][y] = skipCount
	return true
}
