package governance

type ApprovalMode string

const (
	ApprovalAuto         ApprovalMode = "auto"
	ApprovalPrompt       ApprovalMode = "prompt"
	ApprovalDenyHighRisk ApprovalMode = "deny-high-risk"
)

type ApprovalDecision string

const (
	DecisionAllow  ApprovalDecision = "allow"
	DecisionPrompt ApprovalDecision = "prompt"
	DecisionDeny   ApprovalDecision = "deny"
)

func Decide(mode ApprovalMode, a Assessment) ApprovalDecision {
	if a.HardBlocked {
		return DecisionDeny
	}

	switch mode {
	case ApprovalPrompt:
		if a.Level == RiskLow {
			return DecisionAllow
		}
		return DecisionPrompt
	case ApprovalDenyHighRisk:
		if a.Level == RiskHigh {
			return DecisionDeny
		}
		return DecisionAllow
	default: // auto
		if a.Level == RiskHigh {
			return DecisionDeny
		}
		return DecisionAllow
	}
}
