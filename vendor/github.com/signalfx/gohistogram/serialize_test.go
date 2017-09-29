package gohistogram

import (
	"testing"
)

func TestHistogramBytes(t *testing.T) {
	h := NewHistogram(160)
	for _, val := range testData {
		h.Add(float64(val))
	}
	bq := h.Bytes()
	h = NewHistogramBytes(bq)
	if h.Count() != 14999 {
		t.Errorf("Expected h.Count() to be 14999, got %f", h.Count())
	}

	if firstQ := h.Quantile(0.25); !approx(firstQ, 14) {
		t.Errorf("Expected 25th percentile to be %v, got %v", 14, firstQ)
	}
	if median := h.Quantile(0.5); !approx(median, 18) {
		t.Errorf("Expected 50th percentile to be %v, got %v", 18, median)
	}
	if thirdQ := h.Quantile(0.75); !approx(thirdQ, 22) {
		t.Errorf("Expected 75th percentile to be %v, got %v", 22, thirdQ)
	}
	if cdf := h.CDF(18); !approx(cdf, 0.5) {
		t.Errorf("Expected CDF(median) to be %v, got %v", 0.5, cdf)
	}
	if cdf := h.CDF(22); !approx(cdf, 0.75) {
		t.Errorf("Expected CDF(3rd quartile) to be %v, got %v", 0.75, cdf)
	}
}

func TestWeightedHistogramBytes(t *testing.T) {
	h := NewWeightedHistogram(160, 1)
	for _, val := range testData {
		h.Add(float64(val))
	}
	bq := h.Bytes()
	h = NewWeightedHistogramBytes(bq)
	if firstQ := h.Quantile(0.25); !approx(firstQ, 14) {
		t.Errorf("Expected 25th percentile to be %v, got %v", 14, firstQ)
	}
	if median := h.Quantile(0.5); !approx(median, 18) {
		t.Errorf("Expected 50th percentile to be %v, got %v", 18, median)
	}
	if thirdQ := h.Quantile(0.75); !approx(thirdQ, 22) {
		t.Errorf("Expected 75th percentile to be %v, got %v", 22, thirdQ)
	}
	if cdf := h.CDF(18); !approx(cdf, 0.5) {
		t.Errorf("Expected CDF(median) to be %v, got %v", 0.5, cdf)
	}
	if cdf := h.CDF(22); !approx(cdf, 0.75) {
		t.Errorf("Expected CDF(3rd quartile) to be %v, got %v", 0.75, cdf)
	}
}
