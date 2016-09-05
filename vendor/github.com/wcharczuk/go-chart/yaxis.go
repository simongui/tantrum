package chart

import (
	"math"
	"sort"
)

// YAxis is a veritcal rule of the range.
// There can be (2) y-axes; a primary and secondary.
type YAxis struct {
	Name      string
	NameStyle Style

	Style Style

	Zero GridLine

	AxisType YAxisType

	ValueFormatter ValueFormatter
	Range          Range

	Ticks     []Tick
	GridLines []GridLine

	GridMajorStyle Style
	GridMinorStyle Style
}

// GetName returns the name.
func (ya YAxis) GetName() string {
	return ya.Name
}

// GetNameStyle returns the name style.
func (ya YAxis) GetNameStyle() Style {
	return ya.NameStyle
}

// GetStyle returns the style.
func (ya YAxis) GetStyle() Style {
	return ya.Style
}

// GetTicks returns the ticks for a series.
// The coalesce priority is:
// 	- User Supplied Ticks (i.e. Ticks array on the axis itself).
// 	- Range ticks (i.e. if the range provides ticks).
//	- Generating continuous ticks based on minimum spacing and canvas width.
func (ya YAxis) GetTicks(r Renderer, ra Range, defaults Style, vf ValueFormatter) []Tick {
	if len(ya.Ticks) > 0 {
		return ya.Ticks
	}
	if tp, isTickProvider := ra.(TicksProvider); isTickProvider {
		return tp.GetTicks(r, defaults, vf)
	}
	tickStyle := ya.Style.InheritFrom(defaults)
	return GenerateContinuousTicks(r, ra, true, tickStyle, vf)
}

// GetGridLines returns the gridlines for the axis.
func (ya YAxis) GetGridLines(ticks []Tick) []GridLine {
	if len(ya.GridLines) > 0 {
		return ya.GridLines
	}
	return GenerateGridLines(ticks, ya.GridMajorStyle, ya.GridMinorStyle)
}

// Measure returns the bounds of the axis.
func (ya YAxis) Measure(r Renderer, canvasBox Box, ra Range, defaults Style, ticks []Tick) Box {
	ya.Style.InheritFrom(defaults).WriteToRenderer(r)

	sort.Sort(Ticks(ticks))

	var tx int
	if ya.AxisType == YAxisPrimary {
		tx = canvasBox.Right + DefaultYAxisMargin
	} else if ya.AxisType == YAxisSecondary {
		tx = canvasBox.Left - DefaultYAxisMargin
	}

	var minx, maxx, miny, maxy = math.MaxInt32, 0, math.MaxInt32, 0
	var maxTextHeight int
	for _, t := range ticks {
		v := t.Value
		ly := canvasBox.Bottom - ra.Translate(v)

		tb := r.MeasureText(t.Label)
		finalTextX := tx
		if ya.AxisType == YAxisSecondary {
			finalTextX = tx - tb.Width()
		}

		if tb.Height() > maxTextHeight {
			maxTextHeight = tb.Height()
		}

		if ya.AxisType == YAxisPrimary {
			minx = canvasBox.Right
			maxx = Math.MaxInt(maxx, tx+tb.Width())
		} else if ya.AxisType == YAxisSecondary {
			minx = Math.MinInt(minx, finalTextX)
			maxx = Math.MaxInt(maxx, tx)
		}
		miny = Math.MinInt(miny, ly-tb.Height()>>1)
		maxy = Math.MaxInt(maxy, ly+tb.Height()>>1)
	}

	if ya.NameStyle.Show && len(ya.Name) > 0 {
		maxx += (DefaultYAxisMargin + maxTextHeight)
	}

	return Box{
		Top:    miny,
		Left:   minx,
		Right:  maxx,
		Bottom: maxy,
	}
}

// Render renders the axis.
func (ya YAxis) Render(r Renderer, canvasBox Box, ra Range, defaults Style, ticks []Tick) {
	ya.Style.InheritFrom(defaults).WriteToRenderer(r)

	sort.Sort(Ticks(ticks))

	sw := ya.Style.GetStrokeWidth(defaults.StrokeWidth)

	var lx int
	var tx int
	if ya.AxisType == YAxisPrimary {
		lx = canvasBox.Right + int(sw)
		tx = lx + DefaultYAxisMargin
	} else if ya.AxisType == YAxisSecondary {
		lx = canvasBox.Left - int(sw)
		tx = lx - DefaultYAxisMargin
	}

	r.MoveTo(lx, canvasBox.Bottom)
	r.LineTo(lx, canvasBox.Top)
	r.Stroke()

	var maxTextWidth int
	for _, t := range ticks {
		v := t.Value
		ly := canvasBox.Bottom - ra.Translate(v)

		tb := r.MeasureText(t.Label)

		if tb.Width() > maxTextWidth {
			maxTextWidth = tb.Width()
		}

		finalTextX := tx
		finalTextY := ly + tb.Height()>>1
		if ya.AxisType == YAxisSecondary {
			finalTextX = tx - tb.Width()
		}

		r.Text(t.Label, finalTextX, finalTextY)

		r.MoveTo(lx, ly)
		if ya.AxisType == YAxisPrimary {
			r.LineTo(lx+DefaultHorizontalTickWidth, ly)
		} else if ya.AxisType == YAxisSecondary {
			r.LineTo(lx-DefaultHorizontalTickWidth, ly)
		}
		r.Stroke()
	}

	nameStyle := ya.NameStyle.InheritFrom(defaults)
	if ya.NameStyle.Show && len(ya.Name) > 0 {
		nameStyle.GetTextOptions().WriteToRenderer(r)

		r.SetTextRotation(Math.DegreesToRadians(90))

		tb := r.MeasureText(ya.Name)

		var tx int
		if ya.AxisType == YAxisPrimary {
			tx = canvasBox.Right + int(sw) + DefaultYAxisMargin + maxTextWidth + DefaultYAxisMargin
		} else if ya.AxisType == YAxisSecondary {
			tx = canvasBox.Left - (DefaultYAxisMargin + int(sw) + maxTextWidth + DefaultYAxisMargin)
		}

		ty := canvasBox.Bottom - (canvasBox.Height()>>1 + tb.Width()>>1)

		r.Text(ya.Name, tx, ty)
		r.ClearTextRotation()
	}

	if ya.Zero.Style.Show {
		ya.Zero.Render(r, canvasBox, ra, false, Style{})
	}

	if ya.GridMajorStyle.Show || ya.GridMinorStyle.Show {
		for _, gl := range ya.GetGridLines(ticks) {
			if (gl.IsMinor && ya.GridMinorStyle.Show) || (!gl.IsMinor && ya.GridMajorStyle.Show) {
				defaults := ya.GridMajorStyle
				if gl.IsMinor {
					defaults = ya.GridMinorStyle
				}
				gl.Render(r, canvasBox, ra, false, gl.Style.InheritFrom(defaults))
			}
		}
	}
}