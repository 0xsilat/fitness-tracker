package web

import (
	"bytes"
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/local/fitness-tracker/internal/domain"
)

func chartFixture(values ...float64) []domain.ChartPoint {
	start := time.Date(2025, 12, 29, 0, 0, 0, 0, time.UTC)
	points := make([]domain.ChartPoint, len(values))
	for i, value := range values {
		day := start.AddDate(0, 0, i*7)
		points[i] = domain.ChartPoint{Date: day, Label: day.Format("02 Jan 2006"), Value: value}
	}
	return points
}

func TestChartScale(t *testing.T) {
	for _, tc := range []struct {
		name      string
		max       float64
		whole     bool
		top, step float64
	}{
		{"empty", 0, true, 1, 1},
		{"single session", 1, true, 1, 1},
		{"sessions", 3, true, 3, 1},
		{"reps", 83, true, 100, 50},
		{"fractional minutes", 12.5, false, 15, 5},
		{"small minutes", 0.3, false, 0.3, 0.1},
		{"small weighted volume", 0.03, false, 0.03, 0.01},
		{"large volume", 125000, false, 150000, 50000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			top, step := chartScale(tc.max, tc.whole)
			if math.Abs(top-tc.top) > 1e-9 || math.Abs(step-tc.step) > 1e-9 {
				t.Fatalf("scale=(%v,%v), want (%v,%v)", top, step, tc.top, tc.step)
			}
		})
	}
}

func TestChartLayout(t *testing.T) {
	if got := buildChart(nil, chartOptions{}); len(got.Points) != 0 || got.Line != "" {
		t.Fatalf("empty chart=%#v", got)
	}
	single := buildChart(chartFixture(12.5), chartOptions{Unit: "minutes"})
	if single.Points[0].X != 50 || !single.Points[0].ShowLabel || !single.Points[0].MobileLabel || single.Points[0].Value != "12.5" {
		t.Fatalf("single point=%#v", single.Points)
	}
	volume := buildChart(chartFixture(150.75), exerciseChartOptions("weighted"))
	if volume.Points[0].Value != "150.75" {
		t.Fatalf("weighted volume lost precision: %s", volume.Points[0].Value)
	}
	for _, values := range [][]float64{{0, 0, 0}, {25, 25, 25}} {
		chart := buildChart(chartFixture(values...), chartOptions{})
		if len(chart.Ticks) < 2 || chart.Ticks[0].Label != "0" || strings.Contains(chart.Line, "NaN") || strings.Contains(chart.Line, "Inf") {
			t.Fatalf("invalid constant chart=%#v", chart)
		}
	}
	points := chartFixture(10, 20, 30)
	points[1].Date = points[0].Date.AddDate(0, 0, 1)
	chart := buildChart(points, chartOptions{Whole: true})
	if math.Abs(chart.Points[1].X-100.0/14) > 1e-9 {
		t.Fatalf("middle date is not time-spaced: %v", chart.Points[1].X)
	}
	if !strings.Contains(chart.Dates[0].Label, "26") {
		t.Fatalf("year-boundary date lacks year: %#v", chart.Dates)
	}
}

func TestChartLabelThinningAndFormatting(t *testing.T) {
	values := make([]float64, 100)
	for i := range values {
		values[i] = float64(i)
	}
	chart := buildChart(chartFixture(values...), chartOptions{Whole: true})
	labels, mobile := 0, 0
	for _, point := range chart.Points {
		if point.ShowLabel {
			labels++
		}
		if point.MobileLabel {
			mobile++
		}
	}
	if labels > 6 || mobile > 2 || len(chart.Dates) > 5 || len(chart.Points) != 100 || !chart.Points[99].ShowLabel {
		t.Fatalf("bad thinning: labels=%d mobile=%d dates=%d points=%d", labels, mobile, len(chart.Dates), len(chart.Points))
	}
	if !chart.Points[0].ShowLabel || chart.Dates[len(chart.Dates)-1].Position != 0 {
		t.Fatal("dense charts must retain the first value and date")
	}
	for value, want := range map[float64]string{0: "0", 0.01: "0.01", 12.5: "12.5", 1500: "1.5k", 50000: "50k", 1500000: "1.5M"} {
		if got := chartTickNumber(value); got != want {
			t.Errorf("%v: %q want %q", value, got, want)
		}
	}
}

func TestChartRendering(t *testing.T) {
	for _, options := range []chartOptions{{Unit: "sessions", Whole: true}, exerciseChartOptions("bodyweight"), exerciseChartOptions("weighted"), {Unit: "minutes"}} {
		t.Run(options.Unit, func(t *testing.T) {
			var output bytes.Buffer
			if err := Chart(chartFixture(0, 10, 25), "Training trend", options).Render(context.Background(), &output); err != nil {
				t.Fatal(err)
			}
			html := output.String()
			for _, want := range []string{"chart-y-axis", "chart-gridline", options.Unit, "View data (3 points)", `scope="col"`, `scope="row"`, `aria-live="polite"`, "12 Jan 2026", "25 " + options.Unit, `tabindex="0"`, `aria-pressed="true"`} {
				if !strings.Contains(html, want) {
					t.Errorf("missing %q", want)
				}
			}
			if strings.Count(html, `tabindex="0"`) != 1 || strings.Count(html, `data-chart-point=`) != 3 {
				t.Fatal("expected one keyboard entry point and three observations")
			}
			if strings.Contains(html, `role="img"`) {
				t.Fatal("interactive controls must not be hidden by role=img")
			}
		})
	}
	var empty, single bytes.Buffer
	if err := Chart(nil, "Empty", chartOptions{}).Render(context.Background(), &empty); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(empty.String(), "No completed activity") || strings.Contains(empty.String(), "<svg") {
		t.Fatal("bad empty state")
	}
	if err := Chart(chartFixture(12.5), "Cardio", chartOptions{Unit: "minutes"}).Render(context.Background(), &single); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(single.String(), "<polyline") || !strings.Contains(single.String(), "12.5 minutes") {
		t.Fatal("bad single point rendering")
	}
	if !strings.Contains(single.String(), "View data (1 point)") {
		t.Fatal("single-point count should use singular wording")
	}
}

func TestAnalyticsPagesUseChartUnits(t *testing.T) {
	points := chartFixture(10, 20)
	for _, mode := range []string{"weighted", "bodyweight"} {
		var output bytes.Buffer
		if err := ExerciseAnalyticsPage(domain.ExerciseAnalytics{Exercise: domain.Exercise{Mode: mode}, Points: points}).Render(context.Background(), &output); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), "20 "+exerciseChartOptions(mode).Unit) {
			t.Fatalf("missing unit for %s", mode)
		}
	}
	var routine, cardio bytes.Buffer
	if err := RoutineAnalyticsPage(domain.RoutineAnalytics{Points: points}).Render(context.Background(), &routine); err != nil {
		t.Fatal(err)
	}
	points[0].Label = "Week of 29 Dec 2025"
	if err := CardioAnalyticsPage(domain.CardioAnalytics{Points: points}, nil).Render(context.Background(), &cardio); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(routine.String(), "20 sessions") || !strings.Contains(cardio.String(), "20 minutes") || !strings.Contains(cardio.String(), "Week of 29 Dec 2025") {
		t.Fatal("page metrics or week label missing")
	}
}
