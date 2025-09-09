package compute

import (
	"encoding/binary"
	"math"
)

func Quantize(value float64, min float64, max float64) (valueQuantized uint8) {
	if value < min {
		value = min
	} else if value > max {
		value = max
	}
	// Normalize the value to the range [0, 1]
	normalized := (value - min) / (max - min)
	// Scale to [0, 255] and convert to uint8
	valueQuantized = uint8(normalized * 255)
	return valueQuantized
}

func Dequantize(valueQuantized uint8, min float64, max float64) (value float64) {
	// Normalize the uint8 value to the range [0, 1]
	normalized := float64(valueQuantized) / 255.0
	// Scale back to the original range [min, max]
	value = min + normalized*(max-min)
	return value
}

func QuantizeVector(vector []float64) (vectorQuantized []uint8) {
	vectorQuantized = make([]uint8, 8+len(vector))
	min, max := rangeFloat(vector)
	binary.LittleEndian.PutUint32(vectorQuantized, math.Float32bits(float32(min)))
	binary.LittleEndian.PutUint32(vectorQuantized[4:], math.Float32bits(float32(max)))
	for i, value := range vector {
		vectorQuantized[8+i] = Quantize(value, min, max)
	}
	return vectorQuantized
}

func DequantizeVector(vectorQuantized []uint8) (vector []float64) {
	min := float64(math.Float32frombits(binary.LittleEndian.Uint32(vectorQuantized)))
	max := float64(math.Float32frombits(binary.LittleEndian.Uint32(vectorQuantized[4:])))
	vector = make([]float64, len(vectorQuantized)-8)
	for i, value := range vectorQuantized[8:] {
		vector[i] = Dequantize(value, min, max)
	}
	return vector
}

func QuantizeMatrix(matrix [][]float64) (matrixQuantized [][]uint8) {
	matrixQuantized = make([][]uint8, len(matrix))
	for i, vector := range matrix {
		matrixQuantized[i] = QuantizeVector(vector)
	}
	return matrixQuantized
}

func DequantizeMatrix(matrixQuantized [][]uint8) (matrix [][]float64) {
	matrix = make([][]float64, len(matrixQuantized))
	for i, vector := range matrixQuantized {
		matrix[i] = DequantizeVector(vector)
	}
	return matrix
}

func rangeFloat(slice []float64) (min float64, max float64) {
	if len(slice) == 0 {
		return 0, 0
	}
	min = slice[0]
	max = slice[0]
	for _, v := range slice[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}
