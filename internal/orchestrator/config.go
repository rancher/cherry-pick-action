package orchestrator

// Config captures the runtime controls the orchestrator needs.
type Config struct {
	LabelPrefix      string
	ConflictStrategy string
	DryRun           bool
	TargetBranches   []string
}
