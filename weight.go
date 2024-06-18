package smartdns

import (
	"math/rand"
	"time"
)

// shuffleAByWeight shuffles the records based on their weight
func shuffleAByWeight(records []A_Record) []A_Record {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Create a list of indexes based on the weight
	weightedIndexes := make([]int, 0, len(records)*2)
	for i, record := range records {
		for j := 0; j < record.Weight; j++ {
			weightedIndexes = append(weightedIndexes, i)
		}
	}

	shuffledRecords := make([]A_Record, len(records))
	used := make(map[int]bool)
	for i := 0; i < len(records); i++ {
		for {
			index := weightedIndexes[r.Intn(len(weightedIndexes))]
			if !used[index] {
				shuffledRecords[i] = records[index]
				used[index] = true
				break
			}
		}
	}
	return shuffledRecords
}
