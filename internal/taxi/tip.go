// Package taxi is the taxi pack's side of the jobs framework: fares picked up
// at the kerb, risky fares that pay for a rush, and couriers carrying a
// fragile load on a round of drops.
//
// It is pure. It sees the world only through the plain data of a [job.Frame]
// and the city it plans on, and imports nothing of the simulation. Everything
// it pays is earnings: a job never touches the wanted level, and bounty stays
// the police's. Pick-up places are a function of where they are on the plan, as
// roadworks and emergency sites are, so they come out the same whatever order
// the city was reached in.
package taxi

// Streak tuning: each faultless job in a row adds StreakStep percent to the
// pay of the next, up to StreakCap of them.
const (
	StreakStep = 10
	StreakCap  = 5
)

// Multiplier is the pay of a job as a percentage of its worth, for a streak of
// jobs passed without a fault before it. It is never below 100.
func Multiplier(streak int) int {
	return 100 + StreakStep*min(max(streak, 0), StreakCap)
}

// Pay is what a fare is worth: the base, and a tip that every penalty point
// eats into at costPerPoint until it is gone, all scaled by the streak. The
// base is never taken from, so it is never below zero however many points
// there are.
func Pay(base, tip, points, costPerPoint, streak int) int {
	base, tip = max(base, 0), max(tip, 0)
	left := max(tip-max(points, 0)*max(costPerPoint, 0), 0)
	return (base + left) * Multiplier(streak) / 100
}

// Reward is the pay a job type reports to the framework, which takes
// costPerPoint for each point from it. It is [Pay] with that cost added back
// where the tip was too small to bear it, so that what is paid in the end is
// exactly [Pay].
func Reward(base, tip, points, costPerPoint, streak int) int {
	return Pay(base, tip, points, costPerPoint, streak) + max(points, 0)*max(costPerPoint, 0)
}
