package sim

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

func TestADriverOnSnowLeavesMoreRoomAndBrakesEarlier(t *testing.T) {
	dry := Normal()
	if got := dry.onSurface(1); got != dry {
		t.Error("a dry road changed how somebody drives")
	}
	if got := dry.onSurface(0); got != dry {
		t.Error("a surface with no figure for its grip changed how somebody drives")
	}
	snow := dry.onSurface(winterGrip)
	if snow.Decel >= dry.Decel {
		t.Error("a driver on snow counts on the brakes as much as in the dry")
	}
	if snow.Headway <= dry.Headway {
		t.Error("a driver on snow follows as closely as in the dry")
	}
	if snow.CornerBias >= dry.CornerBias {
		t.Error("a driver on snow takes a bend as fast as in the dry")
	}
	// What they plan on is no more than the car can do.
	car := TrafficSpec()
	if share := car.OnSurface(winterGrip).BrakeForce / car.BrakeForce; snow.Decel/dry.Decel > share {
		t.Errorf("a driver plans on %.2f of the dry braking and the car has %.2f", snow.Decel/dry.Decel, share)
	}
}

func TestEveryVehicleInAWinterTownIsOnSnow(t *testing.T) {
	var winter city.Map = -1
	for _, m := range city.Maps() {
		if m.Season() == city.Winter {
			winter = m
		}
	}
	if winter < 0 {
		t.Skip("no map is driven in winter")
	}
	w := NewWorld(Config{Seed: 7, Map: winter, Traffic: 40, Police: 4})
	if w.Player.Spec != w.PlayerBody.Spec.OnSurface(winterGrip) {
		t.Error("the player's car has its dry grip in winter")
	}
	for _, a := range w.Agents {
		if a.V.Spec != a.Body.Spec.OnSurface(winterGrip) {
			t.Fatalf("a %s has its dry grip in winter", a.Body.Kind.Name())
		}
		if a.style().Decel >= a.Style.Decel && !a.pursuing {
			t.Fatalf("the driver of a %s plans a dry stop in winter", a.Body.Kind.Name())
		}
	}
	// Changing the car keeps it on the same road.
	w.SetPlayerBody(PlayerBodyOf(Estate))
	if w.Player.Spec != PlayerBodyOf(Estate).Spec.OnSurface(winterGrip) {
		t.Error("a car chosen in the garage has its dry grip in winter")
	}
	for _, a := range w.Agents {
		if a.Role == RolePolice {
			a.SetUnit(UnitHeavy)
			if a.V.Spec != a.Body.Spec.OnSurface(winterGrip) {
				t.Fatal("a police unit changed to another car has its dry grip in winter")
			}
			break
		}
	}
}

func TestAFastRoadIsJudgedFromFurtherBack(t *testing.T) {
	st := Normal()
	a := &Agent{V: NewVehicle(TrafficSpec(), mathx.V(0, 0), 0)}

	a.lane = &city.Lane{SpeedLimit: mathx.MPH(40)}
	a.V.Vel = mathx.V(mathx.MPH(40), 0)
	if got := a.yieldFrom(st); got != yieldRange {
		t.Errorf("on a town street the gap is judged from %.1f m, want %.1f", got, yieldRange)
	}

	a.lane = &city.Lane{SpeedLimit: mathx.MPH(50)}
	a.V.Vel = mathx.V(mathx.MPH(50), 0)
	far := a.yieldFrom(st)
	stop := a.V.Speed() * a.V.Speed() / (2 * st.Decel)
	if far <= yieldRange || far < stop {
		t.Errorf("at fifty the gap is judged from %.1f m, and it takes %.1f to stop", far, stop)
	}
	// Crawling along a fast road, there is no need to look further than usual.
	a.V.Vel = mathx.V(2, 0)
	if got := a.yieldFrom(st); got != yieldRange {
		t.Errorf("crawling on a fast road the gap is judged from %.1f m", got)
	}
	a.lane = nil
	if got := a.yieldFrom(st); got != yieldRange {
		t.Errorf("off the road the gap is judged from %.1f m", got)
	}
}

// A driver slows for a bend by how sharp it is, and not at all for a road
// that runs straight.
func TestADriverSlowsForABend(t *testing.T) {
	st := Normal()
	straight := NewWorld(Config{Seed: 7, Traffic: 1, Police: 0}).City.Lanes[0]
	if got := bendSpeed(straight, 0, st); got < 1e5 {
		t.Errorf("a straight road is taken at %.1f m/s", got)
	}
	var winding city.Map = -1
	for _, m := range city.Maps() {
		if city.ParamsFor(m, 7).Wind > 0 {
			winding = m
		}
	}
	if winding < 0 {
		t.Skip("no map has a road that bends")
	}
	c := NewWorld(Config{Seed: 7, Map: winding, Traffic: 1, Police: 0}).City
	var gentle, sharp *city.Lane
	for _, l := range c.Lanes {
		b := l.Bend(0, l.Length)
		if b < 1e-4 || c.Roads[l.Road].Ring {
			continue
		}
		if gentle == nil || b < gentle.Bend(0, gentle.Length) {
			gentle = l
		}
		if sharp == nil || b > sharp.Bend(0, sharp.Length) {
			sharp = l
		}
	}
	if gentle == nil || sharp == gentle {
		t.Fatal("no two roads that bend differently")
	}
	fast, slow := bendSpeed(gentle, 0, st), bendSpeed(sharp, 0, st)
	for s := float32(0); s < sharp.Length; s += 5 {
		slow = min(slow, bendSpeed(sharp, s, st))
	}
	if slow >= fast || slow <= 1 {
		t.Errorf("the sharpest bend is taken at %.1f m/s and the gentlest at %.1f", slow, fast)
	}
	// Past the bend, it is no longer a reason to go slowly.
	if got := bendSpeed(sharp, sharp.Length+bendAhead, st); got < 1e5 {
		t.Errorf("a bend already driven still holds a driver to %.1f m/s", got)
	}
}
