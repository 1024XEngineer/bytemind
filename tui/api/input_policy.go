package api

import "time"

type PastePolicyInput struct {
	Input              string
	Source             string
	Workspace          string
	LastInputAt        time.Time
	LastPasteAt        time.Time
	InputBurstSize     int
	PasteSubmitGuard   time.Duration
	PasteBurstWindow   time.Duration
	PasteQuickChars    int
	BurstImmediateMin  int
	BurstCharThreshold int
	ContinuationWindow time.Duration
}

type PastePolicyDecision struct {
	ShouldCompress bool
	Reason         string
}

type InputPolicy interface {
	Evaluate(req PastePolicyInput, hasPathLikeContent bool) Result[PastePolicyDecision]
}
