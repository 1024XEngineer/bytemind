package tools

type ApprovalRequest struct {
	ToolName string
	Command  string
	Reason   string
}

type ApprovalDisposition string

const (
	ApprovalApproveOnce            ApprovalDisposition = "approve_once"
	ApprovalApproveSameToolSession ApprovalDisposition = "approve_same_tool_session"
	ApprovalApproveAllSession      ApprovalDisposition = "approve_all_session"
	ApprovalDeny                   ApprovalDisposition = "deny"
)

type ApprovalDecision struct {
	Disposition ApprovalDisposition
}

func (d ApprovalDecision) Approved() bool {
	switch d.Disposition {
	case ApprovalApproveOnce, ApprovalApproveSameToolSession, ApprovalApproveAllSession:
		return true
	default:
		return false
	}
}

func (d ApprovalDecision) ReusableForSameTool() bool {
	return d.Disposition == ApprovalApproveSameToolSession || d.Disposition == ApprovalApproveAllSession
}

func (d ApprovalDecision) ReusableForAllTools() bool {
	return d.Disposition == ApprovalApproveAllSession
}

func NormalizeApprovalDecision(d ApprovalDecision) ApprovalDecision {
	switch d.Disposition {
	case ApprovalApproveOnce, ApprovalApproveSameToolSession, ApprovalApproveAllSession, ApprovalDeny:
		return d
	default:
		return ApprovalDecision{Disposition: ApprovalDeny}
	}
}

type ApprovalHandler func(ApprovalRequest) (ApprovalDecision, error)
