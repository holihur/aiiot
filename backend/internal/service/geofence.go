package service

import (
	"context"
	"log/slog"
	"sync"

	"github.com/aiiot/server/internal/models"
	"gorm.io/gorm"
)

// GeofenceService keeps the enabled fences in memory (per project) and
// answers point-in-polygon queries so ingest can tag devices with the fences
// they are inside. The index is reloaded whenever fences change.
type GeofenceService struct {
	db     *gorm.DB
	log    *slog.Logger
	mu     sync.RWMutex
	byProj map[uint][]fenceShape
}

type fenceShape struct {
	name string
	ring []geoPoint // GeoJSON Polygon outer ring, [lng, lat] order
}

type geoPoint struct{ lng, lat float64 }

// NewGeofenceService wires the service (call Reload once at startup).
func NewGeofenceService(db *gorm.DB, log *slog.Logger) *GeofenceService {
	if log == nil {
		log = slog.Default()
	}
	return &GeofenceService{db: db, log: log, byProj: map[uint][]fenceShape{}}
}

// Reload rebuilds the in-memory index from the database.
func (g *GeofenceService) Reload(ctx context.Context) error {
	var fences []models.Geofence
	if err := g.db.WithContext(ctx).Where("enabled = ?", true).Find(&fences).Error; err != nil {
		return err
	}
	idx := map[uint][]fenceShape{}
	for i := range fences {
		ring, ok := fenceRing(fences[i].Polygon)
		if !ok {
			continue
		}
		idx[fences[i].ProjectID] = append(idx[fences[i].ProjectID], fenceShape{name: fences[i].Name, ring: ring})
	}
	g.mu.Lock()
	g.byProj = idx
	g.mu.Unlock()
	g.log.Info("geofences reloaded", "projects", len(idx))
	return nil
}

// Match returns the names of enabled fences the point falls inside.
func (g *GeofenceService) Match(projectID uint, lat, lng float64) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var out []string
	for _, f := range g.byProj[projectID] {
		if pointInRing(lat, lng, f.ring) {
			out = append(out, f.name)
		}
	}
	return out
}

// AlertReload tells a running instance to rebuild its index after CRUD.
func (g *GeofenceService) AlertReload(reload func() error) {
	g.log.Info("geofence changed, reloading")
	if err := reload(); err != nil {
		g.log.Warn("geofence reload failed", "error", err)
	}
}

// fenceRing unwraps a GeoJSON polygon into its outer ring.
func fenceRing(poly models.JSONMap) ([]geoPoint, bool) {
	t, _ := poly["type"].(string)
	pts := []geoPoint{}
	if t != "Polygon" {
		return nil, false
	}
	coords, _ := poly["coordinates"].([]any)
	if len(coords) == 0 {
		return nil, false
	}
	ring, _ := coords[0].([]any)
	for _, p := range ring {
		pt, _ := p.([]any)
		if len(pt) < 2 {
			continue
		}
		lng, ok1 := toFloat64(pt[0])
		lat, ok2 := toFloat64(pt[1])
		if ok1 && ok2 {
			pts = append(pts, geoPoint{lng: lng, lat: lat})
		}
	}
	return pts, len(pts) >= 3
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// pointInRing is the even-odd ray-cast test (GeoJSON winding independent).
func pointInRing(lat, lng float64, ring []geoPoint) bool {
	inside := false
	j := len(ring) - 1
	for i := 0; i < len(ring); i++ {
		a, b := ring[i], ring[j]
		if (a.lat > lat) != (b.lat > lat) && lng < (b.lng-a.lng)*(lat-a.lat)/(b.lat-a.lat)+a.lng {
			inside = !inside
		}
		j = i
	}
	return inside
}
