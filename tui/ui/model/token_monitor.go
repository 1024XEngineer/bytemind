package tui

import "time"

const tokenMonitorTickInterval = 20 * time.Millisecond

type tokenMonitorTickMsg time.Time

type tokenUsageComponent struct {
	used        int
	total       int
	displayUsed float64
	unavailable bool
	input       int
	output      int
	context     int
	inputPrice  float64
	outputPrice float64

	hover        bool
	popup        bool
	popupUntil   time.Time
	bounds       rect
	ringSegments int
	simpleRing   bool
	noBraille    bool
}

type rect struct {
	x int
	y int
	w int
	h int
}

type rgb struct {
	r int
	g int
	b int
}
