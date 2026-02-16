package main

import (
	"fmt"
	"math"
	"math/rand"
)

// Sample — одна обучающая точка.
type Sample struct {
	x1, x2 float64
	y      float64
}

// sigmoid — самая простая нелинейная активация.
func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

func main() {
	// "Банальная" задача: научим 1 нейрон реализовывать OR.
	dataset := []Sample{
		{0, 0, 0},
		{0, 1, 1},
		{1, 0, 1},
		{1, 1, 1},
	}

	rand.Seed(42)
	w1 := rand.Float64()*2 - 1
	w2 := rand.Float64()*2 - 1
	b := rand.Float64()*2 - 1

	learningRate := 0.5
	epochs := 5000

	for epoch := 0; epoch < epochs; epoch++ {
		for _, s := range dataset {
			z := w1*s.x1 + w2*s.x2 + b
			pred := sigmoid(z)

			// Градиент бинарной кросс-энтропии с сигмоидой.
			error := pred - s.y
			w1 -= learningRate * error * s.x1
			w2 -= learningRate * error * s.x2
			b -= learningRate * error
		}
	}

	fmt.Printf("Обученные параметры: w1=%.3f w2=%.3f b=%.3f\n\n", w1, w2, b)
	fmt.Println("Проверка:")
	for _, s := range dataset {
		pred := sigmoid(w1*s.x1 + w2*s.x2 + b)
		class := 0
		if pred >= 0.5 {
			class = 1
		}
		fmt.Printf("%.0f OR %.0f => %.3f (class=%d, target=%.0f)\n", s.x1, s.x2, pred, class, s.y)
	}
}
