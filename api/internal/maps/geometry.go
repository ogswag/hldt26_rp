package maps

import "math"

const eps = 1e-9

func dist(a, b XY) float64 {
	return math.Hypot(b.X-a.X, b.Y-a.Y)
}

func cross(o, a, b XY) float64 {
	return (a.X-o.X)*(b.Y-o.Y) - (a.Y-o.Y)*(b.X-o.X)
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// insidePolygon uses ray casting; points on the border count as outside.
func insidePolygon(p XY, ring []XY) bool {
	in := false
	n := len(ring)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		a, b := ring[i], ring[j]
		if onSegment(p, a, b) {
			return false
		}
		if (a.Y > p.Y) != (b.Y > p.Y) {
			x := (b.X-a.X)*(p.Y-a.Y)/(b.Y-a.Y) + a.X
			if p.X < x {
				in = !in
			}
		}
	}
	return in
}

func onSegment(p, a, b XY) bool {
	if math.Abs(cross(a, b, p)) > 1e-7*math.Max(1, dist(a, b)) {
		return false
	}
	return p.X >= math.Min(a.X, b.X)-1e-7 && p.X <= math.Max(a.X, b.X)+1e-7 &&
		p.Y >= math.Min(a.Y, b.Y)-1e-7 && p.Y <= math.Max(a.Y, b.Y)+1e-7
}

// properCross reports segments that cross at a point interior to both.
func properCross(a, b, c, d XY) bool {
	d1 := cross(c, d, a)
	d2 := cross(c, d, b)
	d3 := cross(a, b, c)
	d4 := cross(a, b, d)
	return ((d1 > eps && d2 < -eps) || (d1 < -eps && d2 > eps)) &&
		((d3 > eps && d4 < -eps) || (d3 < -eps && d4 > eps))
}

// segmentHitsPolygon reports a segment that passes through the polygon interior.
func segmentHitsPolygon(a, b XY, ring []XY) bool {
	n := len(ring)
	for i := 0; i < n; i++ {
		if properCross(a, b, ring[i], ring[(i+1)%n]) {
			return true
		}
	}
	const samples = 8
	for k := 1; k < samples; k++ {
		u := float64(k) / samples
		p := XY{X: a.X + (b.X-a.X)*u, Y: a.Y + (b.Y-a.Y)*u}
		if insidePolygon(p, ring) {
			return true
		}
	}
	return false
}

// rayHit returns the distance along dir from p to the nearest wall segment, capped at limit.
func rayHit(p, dir XY, walls [][2]XY, limit float64) float64 {
	best := limit
	for _, w := range walls {
		ex := w[1].X - w[0].X
		ey := w[1].Y - w[0].Y
		den := dir.X*ey - dir.Y*ex
		if math.Abs(den) < eps {
			continue
		}
		qx := w[0].X - p.X
		qy := w[0].Y - p.Y
		t := (qx*ey - qy*ex) / den
		u := (qx*dir.Y - qy*dir.X) / den
		if t >= 0 && u >= -eps && u <= 1+eps && t < best {
			best = t
		}
	}
	return best
}

// clearWidth measures the free corridor across a segment by casting rays to both sides.
func clearWidth(a, b XY, walls [][2]XY, limit float64) float64 {
	length := dist(a, b)
	if length < eps || len(walls) == 0 {
		return 2 * limit
	}
	dir := XY{X: (b.X - a.X) / length, Y: (b.Y - a.Y) / length}
	left := XY{X: -dir.Y, Y: dir.X}
	right := XY{X: dir.Y, Y: -dir.X}
	margin := math.Min(0.75, length*0.2)
	usable := length - 2*margin
	steps := int(usable / 0.5)
	if steps < 1 {
		steps = 1
	}
	best := 2 * limit
	for k := 0; k <= steps; k++ {
		s := margin + usable*float64(k)/float64(steps)
		if usable <= 0 {
			s = length / 2
		}
		p := XY{X: a.X + dir.X*s, Y: a.Y + dir.Y*s}
		w := rayHit(p, left, walls, limit) + rayHit(p, right, walls, limit)
		if w < best {
			best = w
		}
	}
	return best
}

func ringWalls(ring []XY) [][2]XY {
	out := make([][2]XY, 0, len(ring))
	for i := range ring {
		out = append(out, [2]XY{ring[i], ring[(i+1)%len(ring)]})
	}
	return out
}

func polygonArea(ring []XY) float64 {
	s := 0.0
	for i := range ring {
		j := (i + 1) % len(ring)
		s += ring[i].X*ring[j].Y - ring[j].X*ring[i].Y
	}
	return math.Abs(s) / 2
}
