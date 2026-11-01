// Package planted holds one unused function for the U1000 witness: dead is unused, dead2 is used by an
// exported function, and Used is reachable by being exported.
package planted

// Used is reachable: exported.
func Used() int { return dead2() }

func dead() int { return 1 }

func dead2() int { return 2 }
