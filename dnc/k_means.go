package dnc

import (
	"cmp"
	"fmt"
	"math/rand"
	"time"

	"slices"

	"github.com/expki/vectorpedia/compute"
	"github.com/expki/vectorpedia/config"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// assign data to k centroids
func kMeans(multibar *mpb.Progress, id uint64, data [][]uint8, k int) [][]uint8 {
	if k <= 0 {
		return nil
	}
	dlen := len(data)
	if dlen == 0 || dlen <= k {
		return data
	}

	// Step 1: Initialize utilities and dequantize data once
	random := rand.New(rand.NewSource(time.Now().UnixNano()))
	dataFloat := compute.DequantizeMatrix(data)
	chunkedDataMatrix := chunkDataFloat(dataFloat, config.BATCH_SIZE_CACHE)

	// Step 2: Randomly initialize unique centroids superset (as float64)
	kS := min(len(data), k*config.SUPERSET_MUL)
	centroidsFloat := make([][]float64, 0, kS)
	used := make(map[int]struct{}, kS)
	for len(centroidsFloat) < kS {
		i := random.Intn(dlen)
		if _, ok := used[i]; !ok {
			used[i] = struct{}{}
			centroidsFloat = append(centroidsFloat, dataFloat[i])
		}
	}
	used = nil

	// progress bar
	bar := multibar.AddBar(
		0,
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("%d K-Means Superset: ", id)),
			decor.CountersNoUnit("%d / %d"),
		),
		mpb.BarRemoveOnComplete(),
	)

	// Step 3: Iterate superset until convergence
	vectorLen := len(dataFloat[0])
	counts := make([]int, kS)
	sumVectors := make([][]float64, kS)
	meanVectors := make([][]float64, kS)
	prevMeanVectors := make([][]float64, kS)
	for i := range kS {
		sumVectors[i] = make([]float64, vectorLen)
		meanVectors[i] = make([]float64, vectorLen)
		prevMeanVectors[i] = make([]float64, vectorLen)
	}
	var converged bool
	for n := 0; n < config.KMEANS_ITERATION_LIMIT && !converged; n++ {
		bar.Increment()
		// create centroid matrix
		centroidMatrix := compute.NewMatrix(centroidsFloat)

		// find nearest centroid for each data point
		centroidIndexes := make([]int, 0, len(dataFloat))
		for _, dataMatrix := range chunkedDataMatrix {
			_, chunkedCentroidIndexes := centroidMatrix.Clone().MatrixCosineSimilarity(dataMatrix.Clone())
			centroidIndexes = append(centroidIndexes, chunkedCentroidIndexes...)
		}

		// accumulate vectors
		for i, centroidIdx := range centroidIndexes {
			for j, val := range dataFloat[i] {
				sumVectors[centroidIdx][j] += val
			}
			counts[centroidIdx]++
		}

		// compute means
		for i := range sumVectors {
			if counts[i] <= 0 {
				continue
			}
			for j, sum := range sumVectors[i] {
				meanVectors[i][j] = sum / float64(counts[i])
			}
		}

		// check for convergence using cosine similarity
		if n > 0 {
			converged = true
			const threshold float64 = 0.9999 // Nearly identical vectors
			for i := range meanVectors {
				if counts[i] <= 0 {
					continue
				}
				currentVec := compute.NewVector(meanVectors[i])
				prevVec := compute.NewVector(prevMeanVectors[i])
				similarity := currentVec.Clone().VectorCosineSimilarity(prevVec.Clone())
				if similarity < threshold {
					converged = false
					break
				}
			}
		}

		// copy current to previous for next iteration
		for i := range meanVectors {
			copy(prevMeanVectors[i], meanVectors[i])
		}

		// update centroids in-place with computed means
		for i := range meanVectors {
			if counts[i] > 0 {
				copy(centroidsFloat[i], meanVectors[i])
			} else if len(dataFloat) > 0 {
				// Reinitialize empty centroid with a random data point
				randIdx := random.Intn(len(dataFloat))
				copy(centroidsFloat[i], dataFloat[randIdx])
			}
		}
		// reset counts and sumVectors for next iteration
		for idx := range counts {
			counts[idx] = 0
		}
		for idx := range sumVectors {
			for j := range sumVectors[idx] {
				sumVectors[idx][j] = 0
			}
		}
	}
	bar.EnableTriggerComplete()

	// Step 4: Final cluster assignment to get accurate counts
	centroidMatrix := compute.NewMatrix(centroidsFloat)
	counts = make([]int, kS)
	for _, dataMatrix := range chunkedDataMatrix {
		_, chunkedCentroidIndexes := centroidMatrix.Clone().MatrixCosineSimilarity(dataMatrix.Clone())
		for _, idx := range chunkedCentroidIndexes {
			counts[idx]++
		}
	}

	// Step 5: Order superset by size desc
	type result struct {
		vector []float64
		count  int
	}
	results := make([]result, len(centroidsFloat))
	for idx, centroid := range centroidsFloat {
		results[idx] = result{
			vector: centroid,
			count:  counts[idx],
		}
	}
	slices.SortFunc(results, func(a, b result) int {
		return cmp.Compare(b.count, a.count)
	})

	// Step 5: Truncate superset to set
	centroidsFloat = make([][]float64, k)
	for idx := range k {
		centroidsFloat[idx] = results[idx].vector
	}

	// progress bar
	bar = multibar.AddBar(
		0,
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("%d K-Means Set: ", id)),
			decor.CountersNoUnit("%d / %d"),
		),
		mpb.BarRemoveOnComplete(),
	)

	// Step 6: Iterate set until convergence
	counts = make([]int, k)
	sumVectors = sumVectors[:k]
	meanVectors = meanVectors[:k]
	prevMeanVectors = make([][]float64, k)
	for i := range k {
		prevMeanVectors[i] = make([]float64, vectorLen)
	}
	converged = false
	for n := 0; n < config.KMEANS_ITERATION_LIMIT && !converged; n++ {
		bar.Increment()
		// Create centroid matrix (already float64, no dequantization needed)
		centroidMatrix := compute.NewMatrix(centroidsFloat)

		// Find nearest centroid for each data point
		centroidIndexes := make([]int, 0, len(centroidsFloat))
		for _, dataMatrix := range chunkedDataMatrix {
			_, chunkedCentroidIndexes := centroidMatrix.Clone().MatrixCosineSimilarity(dataMatrix.Clone())
			centroidIndexes = append(centroidIndexes, chunkedCentroidIndexes...)
		}

		// Accumulate vectors (already float64, no dequantization needed)
		for i, centroidIdx := range centroidIndexes {
			for j, val := range dataFloat[i] {
				sumVectors[centroidIdx][j] += val
			}
			counts[centroidIdx]++
		}

		// Compute means
		for i := range sumVectors {
			if counts[i] <= 0 {
				continue
			}
			for j, sum := range sumVectors[i] {
				meanVectors[i][j] = sum / float64(counts[i])
			}
		}

		// Check for convergence using cosine similarity
		if n > 0 {
			converged = true
			const threshold float64 = 0.9999 // Nearly identical vectors
			for i := range meanVectors {
				if counts[i] <= 0 {
					continue
				}
				currentVec := compute.NewVector(meanVectors[i])
				prevVec := compute.NewVector(prevMeanVectors[i])
				similarity := currentVec.Clone().VectorCosineSimilarity(prevVec.Clone())
				if similarity < threshold {
					converged = false
					break
				}
			}
		}

		// Copy current to previous for next iteration
		for i := range meanVectors {
			copy(prevMeanVectors[i], meanVectors[i])
		}

		// Update centroids in-place with computed means
		for i := range meanVectors {
			if counts[i] > 0 {
				copy(centroidsFloat[i], meanVectors[i])
			} else if len(dataFloat) > 0 {
				// Reinitialize empty centroid with a random data point
				randIdx := random.Intn(len(dataFloat))
				copy(centroidsFloat[i], dataFloat[randIdx])
			}
		}
		// Reset counts and sumVectors for next iteration
		for idx := range counts {
			counts[idx] = 0
		}
		for idx := range sumVectors {
			for j := range sumVectors[idx] {
				sumVectors[idx][j] = 0
			}
		}
	}
	bar.EnableTriggerComplete()

	// Step 7: Quantize and return converged set
	return compute.QuantizeMatrix(centroidsFloat)
}

func chunkDataFloat(input [][]float64, size int) []compute.Matrix {
	chunks := make([]compute.Matrix, 0, (len(input)/size)+1)
	for i := 0; i < len(input); i += size {
		end := min(i+size, len(input))
		chunks = append(chunks, compute.NewMatrix(input[i:end]))
	}
	return chunks
}
