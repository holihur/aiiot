package service

import (
	"testing"

	"github.com/aiiot/server/internal/models"
)

func ring(latLngs ...[2]float64) models.JSONMap {
	coords := []any{}
	for _, p := range latLngs {
		coords = append(coords, []any{p[1], p[0]})
	}
	return models.JSONMap{"type": "Polygon", "coordinates": []any{coords}}
}

func TestPointInRing(t *testing.T) {
	// square: lng 121.46..121.49, lat 31.22..31.24
	poly := ring([2]float64{31.22, 121.46}, [2]float64{31.22, 121.49}, [2]float64{31.24, 121.49}, [2]float64{31.24, 121.46}, [2]float64{31.22, 121.46})
	shapes, ok := fenceRing(poly)
	if !ok || len(shapes) != 5 {
		t.Fatalf("ring=%v ok=%v", len(shapes), ok)
	}
	if !pointInRing(31.2304, 121.4737, shapes) {
		t.Fatal("inside point must match")
	}
	if pointInRing(31.30, 121.60, shapes) {
		t.Fatal("outside point must not match")
	}
}
