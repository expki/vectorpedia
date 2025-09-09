package compute

type Vector interface {
	Clone() Vector
	VectorCosineSimilarity(vector Vector) float64
	MatrixCosineSimilarity(matrix Matrix) (similarity []float64)
}

type Matrix interface {
	Clone() Matrix
	MatrixCosineSimilarity(matrix Matrix) (relativeSimilaritieList []float64, nearestIndexList []int)
}
