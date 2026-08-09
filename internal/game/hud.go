package game

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"

	crucihud "github.com/danielriddell21/crucible/hud"
	"github.com/danielriddell21/crucible/keymap"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/vision"
)

var (
	hudPanel  = rl.NewColor(16, 18, 22, 195)
	hudEdge   = rl.NewColor(96, 104, 116, 255)
	hudText   = rl.NewColor(226, 230, 236, 255)
	hudDim    = rl.NewColor(150, 158, 170, 255)
	hudGood   = rl.NewColor(96, 206, 128, 255)
	hudWarn   = rl.NewColor(238, 176, 66, 255)
	hudBad    = rl.NewColor(232, 86, 74, 255)
	hudAccent = rl.NewColor(104, 172, 232, 255)
)

func panel(x, y, w, h int32) {
	rl.DrawRectangle(x, y, w, h, hudPanel)
	rl.DrawRectangleLines(x, y, w, h, hudEdge)
}

func (g *Game) drawHUD() {
	g.drawStatusBar()
	g.drawSpeedo()
	g.drawScore()
	g.drawInfractions()
	if g.showPanel {
		g.drawAIPanel()
	}
	if g.auto {
		g.drawAutopilotPanel()
	}
	g.drawMinimap(int32(g.opts.Width)/2-72, 8, 144)
	g.drawNotice()
	if g.showHelp {
		g.drawHelp()
	}
	rl.DrawFPS(int32(g.opts.Width)-92, 8)
}

func (g *Game) drawStatusBar() {
	mode, col := "MANUAL", hudAccent
	if g.auto {
		mode, col = "AUTOPILOT", hudGood
	}
	panel(12, 8, 232, 44)
	rl.DrawText("AUTOBAHN", 24, 14, 12, hudDim)
	rl.DrawText(mode, 24, 28, 20, col)
	if g.paused {
		rl.DrawText("PAUSED", 150, 30, 16, hudWarn)
	}
}

func (g *Game) drawSpeedo() {
	p := g.world.Player
	j := g.world.Judge
	speed := mathx.ToMPH(p.Speed())
	limit := mathx.ToMPH(j.SpeedLimit())

	x, y := int32(12), int32(g.opts.Height)-116
	panel(x, y, 250, 104)

	col := hudText
	if speed > limit+5 {
		col = hudBad
	}
	rl.DrawText(fmt.Sprintf("%.0f", speed), x+16, y+16, 56, col)
	rl.DrawText("mph", x+120, y+52, 16, hudDim)

	// The speed limit, drawn as a roundel like the signs in the world.
	cx, cy := x+200, y+44
	rl.DrawCircle(cx, cy, 26, rl.NewColor(228, 226, 218, 255))
	rl.DrawCircle(cx, cy, 22, rl.NewColor(190, 60, 54, 255))
	rl.DrawCircle(cx, cy, 17, rl.NewColor(238, 236, 230, 255))
	txt := fmt.Sprintf("%.0f", limit)
	rl.DrawText(txt, cx-rl.MeasureText(txt, 18)/2, cy-9, 18, rl.NewColor(24, 24, 26, 255))

	// Signal aspect for the junction ahead, when one governs us.
	if st, ok := j.Signal(); ok {
		lightCol := hudBad
		switch st {
		case sim.SignalGreen:
			lightCol = hudGood
		case sim.SignalAmber, sim.SignalRedAmber:
			lightCol = hudWarn
		}
		rl.DrawCircle(x+228, y+88, 7, lightCol)
		rl.DrawText(st.String(), x+16, y+80, 14, lightCol)
	} else if !j.OnRoad() {
		rl.DrawText("OFF ROAD", x+16, y+80, 14, hudWarn)
	}
}

func (g *Game) drawScore() {
	j := g.world.Judge
	x, y := int32(g.opts.Width)-236, int32(56)
	panel(x, y, 224, 92)
	rl.DrawText("ROAD RULES", x+14, y+10, 12, hudDim)

	col := hudGood
	switch {
	case j.Points > 150:
		col = hudBad
	case j.Points > 40:
		col = hudWarn
	}
	rl.DrawText(fmt.Sprintf("%d", j.Points), x+14, y+26, 34, col)
	rl.DrawText("penalty points", x+14, y+64, 12, hudDim)
	rl.DrawText(fmt.Sprintf("%.2f km", j.Distance/1000), x+140, y+30, 16, hudText)
	rl.DrawText(fmt.Sprintf("%d faults", j.Faults), x+140, y+52, 12, hudDim)
}

func (g *Game) drawInfractions() {
	recent := g.world.Judge.Recent(5)
	if len(recent) == 0 {
		return
	}
	x, y := int32(g.opts.Width)-236, int32(156)
	h := int32(24*len(recent)) + 26
	panel(x, y, 224, h)
	rl.DrawText("RECENT FAULTS", x+14, y+8, 12, hudDim)
	for i, in := range recent {
		ty := y + 26 + int32(i)*24
		age := g.world.Time - in.At
		col := hudBad
		if age > 6 {
			col = hudDim
		}
		rl.DrawText(in.Kind.String(), x+14, ty, 13, col)
		rl.DrawText(fmt.Sprintf("+%d", in.Points), x+190, ty, 13, col)
		if in.Detail != "" && i == 0 {
			rl.DrawText(in.Detail, x+14, ty+12, 10, hudDim)
		}
	}
}

// drawAIPanel shows the exact image the autopilot is reading, boxes and all.
func (g *Game) drawAIPanel() {
	w := int32(g.opts.CamWidth)
	h := int32(g.opts.CamHeight)
	x := int32(g.opts.Width) - w - 12
	y := int32(g.opts.Height) - h - 40

	panel(x-4, y-22, w+8, h+30)
	rl.DrawText("AI CAMERA", x, y-17, 12, hudDim)
	rl.DrawText(fmt.Sprintf("%d detections", len(g.dets)), x+w-96, y-17, 12, hudAccent)

	// Render targets are stored bottom-up, so the source height is negative.
	src := rl.NewRectangle(0, 0, float32(w), -float32(h))
	dst := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	rl.DrawTexturePro(g.aiTarget.Texture, src, dst, rl.NewVector2(0, 0), 0, rl.White)
	rl.DrawRectangleLines(x, y, w, h, hudEdge)

	g.drawClassLegend(x, y+h+4)
}

func (g *Game) drawClassLegend(x, y int32) {
	// Only list the classes actually present this frame, so the legend doubles
	// as a readout of what the AI can currently see.
	seen := map[vision.Class]int{}
	for _, d := range g.dets {
		seen[d.Class]++
	}
	classes := [...]vision.Class{
		vision.ClassVehicle, vision.ClassLightRed, vision.ClassLightRedAmber,
		vision.ClassLightAmber, vision.ClassLightGreen,
		vision.ClassStopSign, vision.ClassGiveWay, vision.ClassStopLine,
		vision.ClassSpeed20, vision.ClassSpeed30, vision.ClassSpeed40,
		vision.ClassLaneMarker, vision.ClassLaneMarkerAlt,
	}
	var cx int32
	for _, c := range classes {
		n, ok := seen[c]
		if !ok {
			continue
		}
		v := vision.RGBA(c)
		col := rl.NewColor(v.R, v.G, v.B, v.A)
		label := fmt.Sprintf("%s:%d", c.String(), n)
		width := rl.MeasureText(label, 10) + 14
		if cx+width > int32(g.opts.CamWidth) {
			break
		}
		rl.DrawRectangle(x+cx, y+2, 8, 8, col)
		rl.DrawText(label, x+cx+11, y+1, 10, hudDim)
		cx += width + 6
	}
}

func (g *Game) drawAutopilotPanel() {
	d := g.driver
	x, y := int32(12), int32(60)
	panel(x, y, 250, 108)
	rl.DrawText("AUTOPILOT", x+14, y+8, 12, hudDim)

	col := hudGood
	switch d.State {
	case autopilot.StateHalted, autopilot.StateWaiting:
		col = hudWarn
	case autopilot.StateSearching:
		col = hudBad
	}
	rl.DrawText(d.State.String(), x+14, y+22, 20, col)
	rl.DrawText(d.Reason, x+14, y+46, 11, hudDim)

	rl.DrawText(fmt.Sprintf("target  %.0f mph", mathx.ToMPH(d.TargetSpeed)), x+14, y+64, 12, hudText)
	rl.DrawText(fmt.Sprintf("sign    %.0f mph", mathx.ToMPH(d.SeenLimit)), x+14, y+78, 12, hudText)
	rl.DrawText(fmt.Sprintf("offset  %+.2f m", d.LaneOffset), x+14, y+92, 12, hudText)

	// Throttle and brake bars.
	bar := func(bx, by int32, v float32, col rl.Color) {
		rl.DrawRectangle(bx, by, 60, 6, rl.NewColor(50, 54, 60, 255))
		rl.DrawRectangle(bx, by, int32(60*mathx.Clamp(v, 0, 1)), 6, col)
	}
	bar(x+176, y+66, g.lastCmd.Throttle, hudGood)
	bar(x+176, y+80, g.lastCmd.Brake, hudBad)
	rl.DrawRectangle(x+176+30+int32(28*mathx.Clamp(g.lastCmd.Steer, -1, 1)), y+94, 4, 6, hudAccent)
	rl.DrawRectangle(x+176, y+96, 60, 1, rl.NewColor(70, 74, 82, 255))
}

// controlBindings is the game's key list, laid out by crucible's keymap.
func controlBindings() []keymap.Binding {
	return []keymap.Binding{
		{Key: "W/S", Action: "throttle, brake"},
		{Key: "A/D", Action: "steer"},
		{Key: "Space", Action: "handbrake"},
		{Key: "Tab", Action: "autopilot"},
		{Key: "C", Action: "camera"},
		{Key: "V", Action: "AI camera"},
		{Key: "L", Action: "labels"},
		{Key: "N", Action: "back on road"},
		{Key: "R", Action: "restart"},
		{Key: "P", Action: "pause"},
		{Key: "H", Action: "help"},
	}
}

func (g *Game) drawHelp() {
	// keymap wraps the bindings to the available width; this game supplies the
	// text measurement and does the drawing itself.
	const fontSize = 12
	measure := func(str string) int { return int(rl.MeasureText(str, fontSize)) }
	rows := keymap.Rows(controlBindings(), g.opts.Width-80, measure)

	w := int32(0)
	for _, r := range rows {
		w = max(w, rl.MeasureText(r, fontSize)+32)
	}
	h := int32(len(rows)*20 + 42)
	x := int32(g.opts.Width)/2 - w/2
	y := int32(g.opts.Height)/2 - h/2
	panel(x, y, w, h)
	rl.DrawText("CONTROLS", x+16, y+12, 14, hudAccent)
	for i, r := range rows {
		rl.DrawText(r, x+16, y+36+int32(i)*20, fontSize, hudText)
	}
}

// drawNotice shows the current overlay line: the toast raised whenever the
// judge records an infraction.
func (g *Game) drawNotice() {
	text, ch, ok := g.notices.Active()
	if !ok {
		return
	}
	col := hudBad
	if ch == crucihud.Diagnostic {
		col = hudDim
	}
	w := rl.MeasureText(text, 20) + 36
	x := int32(g.opts.Width)/2 - w/2
	y := int32(g.opts.Height) - 168
	panel(x, y, w, 34)
	rl.DrawText(text, x+18, y+8, 20, col)
}

// drawMinimap draws a top-down plan of the surrounding streets. Everything is
// clipped to the panel, so the road lines cannot spill across the screen.
func (g *Game) drawMinimap(x, y, size int32) {
	const worldSpan float32 = 250
	half := size / 2
	cx, cy := x+half, y+half
	scale := float32(half) / worldSpan
	p := g.world.Player.Pos

	panel(x, y, size, size)
	rl.BeginScissorMode(x+1, y+1, size-2, size-2)
	for _, l := range g.world.City.Lanes {
		if l.Index != 0 || l.Dir != 1 {
			continue // one line per road, not per lane
		}
		if l.A.DistTo(p) > worldSpan*1.5 && l.B.DistTo(p) > worldSpan*1.5 {
			continue
		}
		col, thick := hudDim, float32(1)
		if l.Class == city.Arterial {
			col, thick = hudText, 2
		}
		rl.DrawLineEx(
			rl.NewVector2(float32(cx)+(l.A.X-p.X)*scale, float32(cy)+(l.A.Z-p.Z)*scale),
			rl.NewVector2(float32(cx)+(l.B.X-p.X)*scale, float32(cy)+(l.B.Z-p.Z)*scale),
			thick, col)
	}
	for _, a := range g.world.Agents {
		d := a.V.Pos.Sub(p)
		if d.Len() > worldSpan*1.4 {
			continue
		}
		rl.DrawCircle(cx+int32(d.X*scale), cy+int32(d.Z*scale), 1.6, hudWarn)
	}
	// The player, drawn as an arrow pointing along the heading.
	f := g.world.Player.Forward()
	r := f.Right()
	tip := rl.NewVector2(float32(cx)+f.X*7, float32(cy)+f.Z*7)
	l := rl.NewVector2(float32(cx)-f.X*4+r.X*4, float32(cy)-f.Z*4+r.Z*4)
	rr := rl.NewVector2(float32(cx)-f.X*4-r.X*4, float32(cy)-f.Z*4-r.Z*4)
	rl.DrawTriangle(tip, l, rr, hudAccent)
	rl.EndScissorMode()
}
