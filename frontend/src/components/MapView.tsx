// MapView is a MapLibre GL wrapper for dashboard GIS panels. It renders an
// optional raster tile base layer (VITE_MAP_TILES_URL, default OpenStreetMap)
// plus device markers and an optional live track.
//
// For offline/private deployments point VITE_MAP_TILES_URL at an internal
// tile server (or leave it empty and only the GeoJSON layers render).

import { useEffect, useRef, useState } from "react";
import * as maplibregl from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";

export interface MapPoint {
  key: string;
  name: string;
  online: boolean;
  lat: number;
  lng: number;
}

function tileUrl(): string | "" {
  const u = ((import.meta as any).env?.VITE_MAP_TILES_URL as string | undefined)?.trim();
  if (u) return u;
  return "https://tile.openstreetmap.org/{z}/{x}/{y}.png";
}

export interface FenceGeo {
  name: string;
  coordinates: number[][][] | number[][];
}

export function MapView({
  points,
  track,
  fences,
  heat,
  drawMode,
  initialVertices,
  onSelect,
  onDrawn,
}: {
  points: MapPoint[];
  track?: { lat: number; lng: number }[];
  fences?: FenceGeo[];
  heat?: boolean;
  drawMode?: boolean;
  initialVertices?: [number, number][];
  onSelect?: (key: string) => void;
  onDrawn?: (coords: number[][]) => void;
}) {
  const [verts, setVerts] = useState<[number, number][]>([]);
  const container = useRef<HTMLDivElement>(null);
  const mapRef = useRef<maplibregl.Map | null>(null);

  useEffect(() => {
    if (!container.current || mapRef.current) return;
    const tiles = tileUrl();
    const map = new maplibregl.Map({
      container: container.current,
      style: {
        version: 8,
        sources: {
          basemap: {
            type: "raster",
            tiles: [tiles],
            tileSize: 256,
            ...(tiles.includes("{") ? {} : { attribution: "© OpenStreetMap" }),
          },
          devices: { type: "geojson", data: { type: "FeatureCollection", features: [] } },
          track: { type: "geojson", data: { type: "FeatureCollection", features: [] } },
          fences: { type: "geojson", data: { type: "FeatureCollection", features: [] } },
          heatrender: { type: "geojson", data: { type: "FeatureCollection", features: [] } },
          draw: { type: "geojson", data: { type: "FeatureCollection", features: [] } },
        },
        layers: [
          { id: "basemap", type: "raster", source: "basemap" },
          {
            id: "draw-poly",
            type: "fill",
            source: "draw",
            filter: ["==", ["geometry-type"], "Polygon"],
            paint: { "fill-color": "#ef4444", "fill-opacity": 0.15 },
          },
          {
            id: "draw-line",
            type: "line",
            source: "draw",
            paint: { "line-color": "#ef4444", "line-width": 2, "line-dasharray": [2, 1] },
          },
          {
            id: "fence-fill",
            type: "fill",
            source: "fences",
            paint: { "fill-color": "#8b5cf6", "fill-opacity": 0.12 },
          },
          {
            id: "fence-line",
            type: "line",
            source: "fences",
            paint: { "line-color": "#8b5cf6", "line-width": 2 },
          },
          {
            id: "heat-line",
            type: "heatmap",
            source: "heatrender",
            paint: {
              "heatmap-weight": 1,
              "heatmap-intensity": 2,
              "heatmap-color": [
                "interpolate", ["linear"], ["heatmap-density"],
                0, "rgba(16,185,129,0)",
                0.4, "rgba(16,185,129,0.6)",
                0.8, "rgba(245,158,11,0.8)",
                1, "rgba(239,68,68,0.95)",
              ],
            },
          },
          {
            id: "track-line",
            type: "line",
            source: "track",
            paint: { "line-color": "#f59e0b", "line-width": 2.5 },
          },
          {
            id: "devices-offline",
            type: "circle",
            source: "devices",
            filter: ["==", ["get", "online"], false],
            paint: {
              "circle-radius": 7,
              "circle-color": "#94a3b8",
              "circle-stroke-color": "#ffffff",
              "circle-stroke-width": 1.5,
            },
          },
          {
            id: "devices-online",
            type: "circle",
            source: "devices",
            filter: ["==", ["get", "online"], true],
            paint: {
              "circle-radius": 7,
              "circle-color": "#10b981",
              "circle-stroke-color": "#ffffff",
              "circle-stroke-width": 1.5,
            },
          },
        ],
      },
      center: points.length ? [points[0].lng, points[0].lat] : [104.0, 35.0],
      zoom: zoomFor(points),
    });
    map.addControl(new maplibregl.NavigationControl({ showCompass: false }), "top-right");
    if (onSelect) {
      map.on("click", (e: maplibregl.MapMouseEvent) => {
        const feats = map.queryRenderedFeatures(e.point, { layers: ["devices-online", "devices-offline"] });
        if (!feats.length) return;
        const k = String(feats[0].properties?.key ?? "");
        if (k) onSelect(k);
      });
    }
    mapRef.current = map;
    return () => {
      map.remove();
      mapRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ---- draw mode: click to add vertices, toolbar to finish / undo ----
  useEffect(() => {
    const map = mapRef.current;
    if (!map || !drawMode) return;
    // preload existing vertices when editing a fence
    if (initialVertices && initialVertices.length >= 2) setVerts(initialVertices);
    const onClick = (e: maplibregl.MapMouseEvent) => {
      const c: [number, number] = [Number(e.lngLat.lng.toFixed(6)), Number(e.lngLat.lat.toFixed(6))];
      setVerts((prev) => [...prev, c]);
    };
    map.on("click", onClick);
    return () => {
      map.off("click", onClick);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [drawMode]);

  // render the in-progress polygon from the collected vertices
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const src = map.getSource("draw") as maplibregl.GeoJSONSource | undefined;
    if (!src || verts.length === 0) return;
    src.setData({
      type: "FeatureCollection",
      features: [
        {
          type: "Feature",
          geometry:
            verts.length < 3
              ? { type: "LineString", coordinates: verts }
              : { type: "Polygon", coordinates: [verts, ...(verts.length > 3 ? [] : [])] },
          properties: {},
        },
      ],
    });
  }, [verts]);

  // clean the draw layer when leaving draw mode
  useEffect(() => {
    if (drawMode) return;
    setVerts([]);
    const map = mapRef.current;
    const src = map?.getSource("draw") as maplibregl.GeoJSONSource | undefined;
    if (src) src.setData({ type: "FeatureCollection", features: [] });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [drawMode]);

  // update device points (circles hidden when heat mode is on)
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const src = map.getSource("devices") as maplibregl.GeoJSONSource | undefined;
    if (src) {
      src.setData({
        type: "FeatureCollection",
        features: points.map((p) => ({
          type: "Feature" as const,
          geometry: { type: "Point" as const, coordinates: [p.lng, p.lat] },
          properties: { key: p.key, name: p.name, online: p.online },
        })),
      });
    }
    const heatSrc = map.getSource("heatrender") as maplibregl.GeoJSONSource | undefined;
    if (heatSrc) {
      heatSrc.setData({
        type: "FeatureCollection",
        features: points.map((p) => ({
          type: "Feature" as const,
          geometry: { type: "Point" as const, coordinates: [p.lng, p.lat] },
          properties: {},
        })),
      });
    }
    map.setLayoutProperty("devices-online", "visibility", heat ? "none" : "visible");
    map.setLayoutProperty("devices-offline", "visibility", heat ? "none" : "visible");
    map.setLayoutProperty("heat-line", "visibility", heat ? "visible" : "none");
  }, [points, heat]);

  // update the live track polyline
  useEffect(() => {
    const map = mapRef.current;
    if (!map || !track || track.length < 2) return;
    const src = map.getSource("track") as maplibregl.GeoJSONSource | undefined;
    if (!src) return;
    src.setData({
      type: "FeatureCollection",
      features: [
        {
          type: "Feature",
          geometry: { type: "LineString", coordinates: track.map((p) => [p.lng, p.lat]) },
          properties: {},
        },
      ],
    });
  }, [track]);

  // update geofence polygons
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;
    const src = map.getSource("fences") as maplibregl.GeoJSONSource | undefined;
    if (!src) return;
    src.setData({
      type: "FeatureCollection",
      features: (fences ?? []).map((f) => ({
        type: "Feature",
        geometry: { type: "Polygon", coordinates: f.coordinates },
        properties: { name: f.name },
      })),
    });
  }, [fences]);

  return (
    <div className="relative">
      <div ref={container} className="h-64 w-full rounded-lg border" />
      {drawMode && (
        <div className="absolute left-2 top-2 z-10 flex items-center gap-1 rounded-md border bg-background/90 p-1 shadow">
          <span className="px-1 text-[10px] text-muted-foreground">{verts.length} pts · click map</span>
          <button
            type="button"
            className="rounded border px-1.5 py-0.5 text-[10px] hover:bg-muted"
            disabled={verts.length < 3}
            onClick={() => {
              if (verts.length >= 3 && onDrawn) onDrawn(verts);
              setVerts([]);
            }}
          >
            finish
          </button>
          <button
            type="button"
            className="rounded border px-1.5 py-0.5 text-[10px] hover:bg-muted"
            disabled={verts.length === 0}
            onClick={() => setVerts((prev) => prev.slice(0, -1))}
          >
            undo
          </button>
          <button
            type="button"
            className="rounded border px-1.5 py-0.5 text-[10px] hover:bg-muted"
            onClick={() => setVerts([])}
          >
            clear
          </button>
        </div>
      )}
    </div>
  );
}

function zoomFor(points: MapPoint[]): number {
  if (points.length === 0) return 3;
  const lats = points.map((p) => p.lat);
  const lngs = points.map((p) => p.lng);
  const span = Math.max(Math.max(...lats) - Math.min(...lats), Math.max(...lngs) - Math.min(...lngs), 0.01);
  return Math.max(2, Math.min(13, Math.round(9 - Math.log2(span) + 3)));
}