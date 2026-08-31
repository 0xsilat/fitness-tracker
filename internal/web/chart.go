package web

import (
	"fmt"
	"math"
	"strings"

	"github.com/local/fitness-tracker/internal/domain"
)

type chartOptions struct {
	Unit  string
	Whole bool
}

func exerciseChartOptions(mode string) chartOptions {
	if mode == "weighted" {
		return chartOptions{Unit: "kg·reps"}
	}
	return chartOptions{Unit: "reps", Whole: true}
}

type chartTick struct {
	Position float64
	Label    string
	Mobile   bool
}

type chartDatum struct {
	X, Y         float64
	Label, Value string
	ShowLabel    bool
	MobileLabel  bool
}

type chartView struct {
	Points []chartDatum
	Ticks  []chartTick
	Dates  []chartTick
	Line   string
}

// Tick intervals use 1, 2, or 5 times a power of ten. All chart metrics are
// non-negative; counts never get fractional ticks.
func chartScale(max float64, whole bool) (top, step float64) {
	if max <= 0 {
		return 1, 1
	}
	raw := max / 4
	power := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, multiplier := range []float64{1, 2, 5, 10} {
		step = multiplier * power
		if step >= raw {
			break
		}
	}
	minimum := 0.01
	if whole {
		minimum = 1
	}
	step = math.Max(minimum, step)
	top = math.Ceil(max/step) * step
	return top, step
}

func chartTickNumber(value float64) string {
	for _, unit := range []struct {
		value float64
		label string
	}{{1e9, "B"}, {1e6, "M"}, {1e3, "k"}} {
		if value >= unit.value {
			return chartNumber(value/unit.value) + unit.label
		}
	}
	return chartNumber(value)
}

// Stored weights have two decimal places and cardio minutes have one. Keep
// that precision in point labels, the readout, and the data table.
func chartNumber(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", value), "0"), ".")
}

func chartPointCount(count int) string {
	if count == 1 {
		return "1 point"
	}
	return fmt.Sprintf("%d points", count)
}

func chartPosition(x, y float64) string {
	return fmt.Sprintf("left:%.4f%%;top:%.4f%%", x, y)
}

func chartTop(y float64) string  { return fmt.Sprintf("top:%.4f%%", y) }
func chartLeft(x float64) string { return fmt.Sprintf("left:%.4f%%", x) }

func chartTabIndex(index, count int) string {
	if index == count-1 {
		return "0"
	}
	return "-1"
}

func chartAnchor(x float64) string {
	if x < 10 {
		return "chart-align-start"
	}
	if x > 90 {
		return "chart-align-end"
	}
	return ""
}

func buildChart(points []domain.ChartPoint, options chartOptions) chartView {
	view := chartView{}
	if len(points) == 0 {
		return view
	}
	max := 0.0
	for _, point := range points {
		max = math.Max(max, point.Value)
	}
	top, step := chartScale(max, options.Whole)
	// Insets leave room for labels above the peak and for zero-valued markers.
	for i := 0; i <= int(math.Round(top/step)); i++ {
		value := float64(i) * step
		view.Ticks = append(view.Ticks, chartTick{Position: 90 - value/top*75, Label: chartTickNumber(value)})
	}
	first, last := points[0].Date, points[len(points)-1].Date
	timed := !first.IsZero() && last.After(first)
	var coordinates []string
	for i, point := range points {
		x := 50.0
		if timed {
			x = point.Date.Sub(first).Seconds() / last.Sub(first).Seconds() * 100
		} else if len(points) > 1 {
			x = float64(i) / float64(len(points)-1) * 100
		}
		y := 90 - point.Value/top*75
		value := chartNumber(point.Value)
		if options.Whole {
			value = fmt.Sprintf("%.0f", point.Value)
		}
		view.Points = append(view.Points, chartDatum{X: x, Y: y, Label: point.Label, Value: value})
		coordinates = append(coordinates, fmt.Sprintf("%.4f,%.4f", x*10, y*2))
	}
	view.Line = strings.Join(coordinates, " ")
	// Prefer the latest value and avoid crowded labels, including irregular dates.
	previous := 120.0
	for i := len(view.Points) - 1; i >= 0; i-- {
		point := &view.Points[i]
		if i == 0 || (previous-point.X >= 20 && (i == len(view.Points)-1 || point.X-view.Points[0].X >= 20)) {
			point.ShowLabel = true
			previous = point.X
		}
		point.MobileLabel = i == len(view.Points)-1 || (i == 0 && view.Points[len(view.Points)-1].X-point.X >= 60)
	}
	// Label a subset of observed dates. Unlike equal index spacing, these labels
	// and the line both preserve the actual gaps between observations.
	previous = 130
	for i := len(points) - 1; i >= 0; i-- {
		x := view.Points[i].X
		if i != 0 && (previous-x < 25 || (i != len(points)-1 && x-view.Points[0].X < 25)) {
			continue
		}
		label := points[i].Label
		if !points[i].Date.IsZero() {
			label = points[i].Date.Format("02 Jan")
			if first.Year() != last.Year() {
				label = points[i].Date.Format("02 Jan 06")
			}
		}
		view.Dates = append(view.Dates, chartTick{Position: x, Label: label, Mobile: i == 0 || i == len(points)-1})
		previous = x
	}
	return view
}
