package main

import "desk-chart/fence"

// The chart core (series parsing, SVG geometry) lives in the fence package
// so kit Markdown ```chart fences reuse the C1 renderer instead of
// duplicating it. The aliases below keep the historic main-package names
// (and every test and call site) working unchanged.

type (
	point     = fence.Point
	bucket    = fence.Bucket
	series    = fence.Series
	num       = fence.Num
	nanos     = fence.Nanos
	otlpPoint = fence.OTLPPoint
)

const (
	maxInput  = fence.MaxInput
	maxPoints = fence.MaxPoints
)

func parseData(raw []byte, format string) ([]series, error) {
	return fence.ParseData(raw, format)
}

func histBuckets(p otlpPoint, kind string) ([]bucket, error) {
	return fence.HistBuckets(p, kind)
}

func quantile(bs []bucket, q float64) string {
	return fence.Quantile(bs, q)
}

func ratePoints(s series) []point {
	return fence.RatePoints(s)
}
